// Package tunnel 流量调度中枢
// 所有入站连接统一经过 tunnel 进行：元数据提取 → DNS 解析 → 规则匹配 → 出站选择 → 流量转发
package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/Qing060325/Hades/pkg/perf/zerocopy"
	"github.com/rs/zerolog/log"
)

// DNSResolver DNS 解析器接口
type DNSResolver interface {
	ResolveIP(host string) (net.IP, error)
}

// Tunnel 流量调度中枢
type Tunnel struct {
	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager
	resolver   DNSResolver
	tracker    *Tracker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New 创建 Tunnel
func New(
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
	resolver DNSResolver,
) *Tunnel {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tunnel{
		adapterMgr: adapterMgr,
		ruleEngine: ruleEngine,
		groupMgr:   groupMgr,
		resolver:   resolver,
		tracker:    NewTracker(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// HandleTCP 处理 TCP 连接（核心调度流程）
// 所有 listener 的 TCP 连接都走这条路
func (t *Tunnel) HandleTCP(conn net.Conn, metadata *adapter.Metadata) {
	t.wg.Add(1)
	defer t.wg.Done()

	// 检查是否已关闭
	select {
	case <-t.ctx.Done():
		conn.Close()
		return
	default:
	}

	// 连接跟踪
	trackedConn := t.tracker.Track(conn)
	defer trackedConn.Close()

	// 1. DNS 解析（如果需要且规则要求）
	t.resolveIP(metadata)

	// 2. 规则匹配 → 选择适配器
	adapt, ruleName := t.matchAdapter(metadata)
	if adapt == nil {
		log.Warn().
			Str("target", metadata.DestinationAddress()).
			Msg("[Tunnel] 无可用适配器，丢弃连接")
		return
	}

	log.Debug().
		Str("adapter", adapt.Name()).
		Str("rule", ruleName).
		Str("target", metadata.DestinationAddress()).
		Str("network", metadata.NetWork).
		Msg("[Tunnel] 连接匹配")

	// 3. 建立后端连接
	backendConn, err := adapt.DialContext(t.ctx, metadata)
	if err != nil {
		log.Error().Err(err).
			Str("adapter", adapt.Name()).
			Str("target", metadata.DestinationAddress()).
			Msg("[Tunnel] 建立后端连接失败")
		return
	}
	defer backendConn.Close()

	// 4. 双向转发
	t.relay(trackedConn, backendConn)
}

// HandleUDP 处理 UDP 数据包转发
func (t *Tunnel) HandleUDP(metadata *adapter.Metadata) (net.PacketConn, error) {
	// DNS 解析
	t.resolveIP(metadata)

	// 规则匹配
	adapt, _ := t.matchAdapter(metadata)
	if adapt == nil {
		return nil, fmt.Errorf("no adapter available for %s", metadata.DestinationAddress())
	}

	if !adapt.SupportUDP() {
		return nil, fmt.Errorf("adapter %s does not support UDP", adapt.Name())
	}

	return adapt.DialUDPContext(t.ctx, metadata)
}

// matchAdapter 规则匹配 → 选择适配器
// 返回适配器和匹配到的规则名
func (t *Tunnel) matchAdapter(metadata *adapter.Metadata) (adapter.Adapter, string) {
	if t.ruleEngine != nil {
		adapterName, rule := t.ruleEngine.MatchWithRule(metadata)
		if adapterName != "" {
			// 尝试直接获取适配器
			if adapt := t.adapterMgr.Get(adapterName); adapt != nil {
				ruleName := ""
				if rule != nil {
					ruleName = string(rule.Type()) + "," + rule.Payload()
				}
				return adapt, ruleName
			}

			// 尝试从代理组获取
			if t.groupMgr != nil {
				if g := t.groupMgr.Get(adapterName); g != nil {
					adapt := g.Select(metadata)
					if adapt != nil {
						return adapt, "group:" + adapterName
					}
				}
			}
		}
	}

	// 兜底：尝试 proxy 组
	if t.groupMgr != nil {
		if g := t.groupMgr.Get("proxy"); g != nil {
			adapt := g.Select(metadata)
			if adapt != nil {
				return adapt, "group:proxy"
			}
		}
	}

	// 最终兜底：直连
	if adapt := t.adapterMgr.Get("DIRECT"); adapt != nil {
		return adapt, "DIRECT"
	}

	return nil, ""
}

// resolveIP DNS 解析
// 如果 metadata 中只有域名没有 IP，尝试解析
func (t *Tunnel) resolveIP(metadata *adapter.Metadata) {
	if t.resolver == nil {
		return
	}

	// 已有 IP 则跳过
	if metadata.DstIP.IsValid() {
		return
	}

	// 没有域名也不需要解析
	if metadata.Host == "" {
		return
	}

	ip, err := t.resolver.ResolveIP(metadata.Host)
	if err != nil {
		log.Debug().Err(err).
			Str("host", metadata.Host).
			Msg("[Tunnel] DNS 解析失败")
		return
	}

	addr, ok := netipFromIP(ip)
	if ok {
		metadata.DstIP = addr
	}
}

// relay 双向流量转发
func (t *Tunnel) relay(left, right net.Conn) {
	// 尝试 TCP 零拷贝
	if lc, lok := left.(*net.TCPConn); lok {
		if rc, rok := right.(*net.TCPConn); rok {
			zerocopy.TCPRelay(lc, rc)
			return
		}
	}

	// 回退到缓冲拷贝
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
	}()

	wg.Wait()
}

// Close 优雅关闭 Tunnel
func (t *Tunnel) Close() {
	t.cancel()

	// 等待所有活跃连接完成（带超时）
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("[Tunnel] 所有连接已关闭")
	case <-time.After(10 * time.Second):
		log.Warn().Msg("[Tunnel] 等待连接关闭超时")
	}

	t.tracker.Close()
}

// Stats 返回统计信息
func (t *Tunnel) Stats() Stats {
	return t.tracker.Stats()
}

// UpdateRuleEngine 热更新规则引擎
func (t *Tunnel) UpdateRuleEngine(engine *rules.Engine) {
	t.ruleEngine = engine
}

// UpdateAdapterManager 热更新适配器管理器
func (t *Tunnel) UpdateAdapterManager(mgr *adapter.Manager) {
	t.adapterMgr = mgr
}

// UpdateGroupManager 热更新代理组管理器
func (t *Tunnel) UpdateGroupManager(mgr *group.Manager) {
	t.groupMgr = mgr
}

// netipFromIP 将 net.IP 转换为 netip.Addr
func netipFromIP(ip net.IP) (netip.Addr, bool) {
	return netip.AddrFromSlice(ip)
}
