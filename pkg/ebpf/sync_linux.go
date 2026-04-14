//go:build linux

package ebpf

import (
	"net/netip"

	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/rs/zerolog/log"
)

// RuleSyncer 将 Hades 规则引擎的 IP-CIDR 规则同步到 eBPF maps
type RuleSyncer struct {
	mgr *Manager
}

// NewRuleSyncer 创建规则同步器
func NewRuleSyncer(mgr *Manager) *RuleSyncer {
	return &RuleSyncer{mgr: mgr}
}

// SyncFromEngine 从规则引擎同步 IP-CIDR 规则到 eBPF
//
// 只同步可以内核加速的规则：
//   - IP-CIDR → DIRECT 规则 → ActionDirect（内核直接放行）
//   - IP-CIDR → REJECT 规则 → ActionReject（内核直接丢弃）
//   - IP-CIDR6 规则同理
//
// 域名规则无法在内核层处理（需要 DNS 解析），仍走用户态代理
func (s *RuleSyncer) SyncFromEngine(engine *rules.Engine) error {
	if s.mgr == nil || !s.mgr.IsAttached() {
		log.Debug().Msg("[eBPF] 管理器未附加，跳过规则同步")
		return nil
	}

	var v4Rules []CIDR4Rule
	var v6Rules []CIDR6Rule

	for _, rule := range engine.Rules() {
		action := s.mapAdapterToAction(rule.Adapter())
		if action == ActionProxy {
			continue // 无法加速的规则跳过
		}

		switch rule.Type() {
		case rules.RuleTypeIPCIDR:
			if cidr, ok := parseCIDR4(rule.Payload()); ok {
				v4Rules = append(v4Rules, CIDR4Rule{
					Prefix: cidr,
					Action: action,
				})
			}

		case rules.RuleTypeIPCIDR6:
			if cidr, ok := parseCIDR6(rule.Payload()); ok {
				v6Rules = append(v6Rules, CIDR6Rule{
					Prefix: cidr,
					Action: action,
				})
			}

		case rules.RuleTypeSrcIPCIDR:
			// SRC-IP-CIDR 目前不支持内核加速
			// 因为 TC ingress 看到的是目的地址匹配
		}
	}

	// 更新 eBPF maps
	if len(v4Rules) > 0 {
		if err := s.mgr.UpdateCIDR4Rules(v4Rules); err != nil {
			log.Error().Err(err).Msg("[eBPF] 同步 IPv4 规则失败")
		}
	}

	if len(v6Rules) > 0 {
		if err := s.mgr.UpdateCIDR6Rules(v6Rules); err != nil {
			log.Error().Err(err).Msg("[eBPF] 同步 IPv6 规则失败")
		}
	}

	log.Info().
		Int("v4", len(v4Rules)).
		Int("v6", len(v6Rules)).
		Msg("[eBPF] 规则同步完成")

	return nil
}

// mapAdapterToAction 将适配器名称映射到 eBPF 动作
func (s *RuleSyncer) mapAdapterToAction(adapterName string) Action {
	switch adapterName {
	case "DIRECT":
		return ActionDirect
	case "REJECT", "REJECT-DROP":
		return ActionReject
	default:
		return ActionProxy
	}
}

// parseCIDR4 解析 IPv4 CIDR 字符串
// 输入格式：如 "192.168.0.0/16" 或 "10.0.0.0/8,no-resolve"
func parseCIDR4(payload string) (netip.Prefix, bool) {
	// 移除可能的 no-resolve 后缀
	clean := splitNoResolve(payload)
	prefix, err := netip.ParsePrefix(clean)
	if err != nil {
		return netip.Prefix{}, false
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, false
	}
	return prefix, true
}

// parseCIDR6 解析 IPv6 CIDR 字符串
func parseCIDR6(payload string) (netip.Prefix, bool) {
	clean := splitNoResolve(payload)
	prefix, err := netip.ParsePrefix(clean)
	if err != nil {
		return netip.Prefix{}, false
	}
	if !prefix.Addr().Is6() {
		return netip.Prefix{}, false
	}
	return prefix, true
}

// splitNoResolve 移除 CIDR 规则的 no-resolve 后缀
func splitNoResolve(payload string) string {
	for i := len(payload) - 1; i >= 0; i-- {
		if payload[i] == ',' {
			return payload[:i]
		}
		if payload[i] == '/' {
			break
		}
	}
	return payload
}
