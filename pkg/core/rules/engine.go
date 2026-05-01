// Package rules 规则引擎模块
package rules

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// RuleType 规则类型
type RuleType string

const (
	RuleTypeDomain        RuleType = "DOMAIN"
	RuleTypeDomainSuffix  RuleType = "DOMAIN-SUFFIX"
	RuleTypeDomainKeyword RuleType = "DOMAIN-KEYWORD"
	RuleTypeDomainWildcard RuleType = "DOMAIN-WILDCARD"
	RuleTypeGeoIP         RuleType = "GEOIP"
	RuleTypeGeoSite       RuleType = "GEOSITE"
	RuleTypeIPCIDR        RuleType = "IP-CIDR"
	RuleTypeIPCIDR6       RuleType = "IP-CIDR6"
	RuleTypeSrcIPCIDR     RuleType = "SRC-IP-CIDR"
	RuleTypeSrcGeoIP      RuleType = "SRC-GEOIP"
	RuleTypeProcessName   RuleType = "PROCESS-NAME"
	RuleTypeProcessPath   RuleType = "PROCESS-PATH"
	RuleTypeRuleSet       RuleType = "RULE-SET"
	RuleTypeMatch         RuleType = "MATCH"
	RuleTypeFinal         RuleType = "FINAL"
)

// Rule 规则接口
type Rule interface {
	Match(metadata *adapter.Metadata) bool
	Adapter() string
	Payload() string
	ShouldResolveIP() bool
	Type() RuleType
}

// BaseRule 基础规则
type BaseRule struct {
	adapter         string
	payload         string
	shouldResolveIP bool
}

// Adapter 返回适配器名称
func (r *BaseRule) Adapter() string {
	return r.adapter
}

// Payload 返回规则内容
func (r *BaseRule) Payload() string {
	return r.payload
}

// ShouldResolveIP 是否需要解析IP
func (r *BaseRule) ShouldResolveIP() bool {
	return r.shouldResolveIP
}

// DomainRule 域名规则
type DomainRule struct {
	BaseRule
	domain string
}

// Match 匹配
func (r *DomainRule) Match(metadata *adapter.Metadata) bool {
	return metadata.Host == r.domain
}

// Type 返回类型
func (r *DomainRule) Type() RuleType {
	return RuleTypeDomain
}

// DomainSuffixRule 域名后缀规则
type DomainSuffixRule struct {
	BaseRule
	suffix string
}

// Match 匹配
func (r *DomainSuffixRule) Match(metadata *adapter.Metadata) bool {
	return strings.HasSuffix(metadata.Host, r.suffix) || metadata.Host == r.suffix[1:]
}

// Type 返回类型
func (r *DomainSuffixRule) Type() RuleType {
	return RuleTypeDomainSuffix
}

// DomainKeywordRule 域名关键字规则
type DomainKeywordRule struct {
	BaseRule
	keyword string
}

// Match 匹配
func (r *DomainKeywordRule) Match(metadata *adapter.Metadata) bool {
	return strings.Contains(metadata.Host, r.keyword)
}

// Type 返回类型
func (r *DomainKeywordRule) Type() RuleType {
	return RuleTypeDomainKeyword
}

// DomainWildcardRule 域名通配符规则
type DomainWildcardRule struct {
	BaseRule
	pattern *regexp.Regexp
}

// Match 匹配
func (r *DomainWildcardRule) Match(metadata *adapter.Metadata) bool {
	return r.pattern.MatchString(metadata.Host)
}

// Type 返回类型
func (r *DomainWildcardRule) Type() RuleType {
	return RuleTypeDomainWildcard
}

// IPCIDRRule IP CIDR 规则
type IPCIDRRule struct {
	BaseRule
	prefix netip.Prefix
	isSrc  bool
}

// Match 匹配
func (r *IPCIDRRule) Match(metadata *adapter.Metadata) bool {
	var ip netip.Addr
	if r.isSrc {
		ip = metadata.SrcIP
	} else {
		ip = metadata.DstIP
	}

	if !ip.IsValid() {
		return false
	}

	return r.prefix.Contains(ip)
}

// Type 返回类型
func (r *IPCIDRRule) Type() RuleType {
	if r.isSrc {
		return RuleTypeSrcIPCIDR
	}
	if r.prefix.Addr().Is6() {
		return RuleTypeIPCIDR6
	}
	return RuleTypeIPCIDR
}

// ProcessNameRule 进程名规则
type ProcessNameRule struct {
	BaseRule
	process string
}

// Match 匹配
func (r *ProcessNameRule) Match(metadata *adapter.Metadata) bool {
	return metadata.ProcessName == r.process
}

// Type 返回类型
func (r *ProcessNameRule) Type() RuleType {
	return RuleTypeProcessName
}

// MatchRule 匹配所有规则
type MatchRule struct {
	BaseRule
}

// Match 匹配所有
func (r *MatchRule) Match(metadata *adapter.Metadata) bool {
	return true
}

// Type 返回类型
func (r *MatchRule) Type() RuleType {
	return RuleTypeMatch
}

// Engine 规则引擎
type Engine struct {
	rules     []Rule
	ruleSets  map[string]RuleSet
	providers []RuleSet // 规则集提供者
}

// RuleSet 规则集接口
type RuleSet interface {
	Name() string
	Rules() []Rule
	Match(metadata *adapter.Metadata) bool
}

// NewEngine 创建规则引擎
func NewEngine(ruleStrs []string) *Engine {
	engine := &Engine{
		rules:     make([]Rule, 0),
		ruleSets:  make(map[string]RuleSet),
		providers: make([]RuleSet, 0),
	}

	// 解析规则
	for _, ruleStr := range ruleStrs {
		rule, err := ParseRule(ruleStr)
		if err != nil {
			continue
		}
		engine.rules = append(engine.rules, rule)
	}

	return engine
}

// NewEngineWithProviders 创建包含 Provider 规则的引擎
func NewEngineWithProviders(ruleStrs []string, providerRules []Rule) *Engine {
	engine := NewEngine(ruleStrs)
	engine.rules = append(engine.rules, providerRules...)
	return engine
}

// Match 匹配规则
func (e *Engine) Match(metadata *adapter.Metadata) string {
	for _, rule := range e.rules {
		if rule.Match(metadata) {
			return rule.Adapter()
		}
	}

	// 默认返回直连
	return "DIRECT"
}

// ParseRule 解析规则字符串
func ParseRule(ruleStr string) (Rule, error) {
	parts := strings.Split(ruleStr, ",")
	if len(parts) < 2 {
		return nil, ErrInvalidRule
	}

	ruleType := RuleType(strings.ToUpper(parts[0]))
	adapterName := parts[len(parts)-1]
	payload := ""

	if len(parts) > 2 {
		payload = strings.Join(parts[1:len(parts)-1], ",")
	}

	base := BaseRule{
		adapter: adapterName,
		payload: payload,
	}

	switch ruleType {
	case RuleTypeDomain:
		return &DomainRule{
			BaseRule: base,
			domain:   payload,
		}, nil

	case RuleTypeDomainSuffix:
		return &DomainSuffixRule{
			BaseRule: base,
			suffix:   "." + payload,
		}, nil

	case RuleTypeDomainKeyword:
		return &DomainKeywordRule{
			BaseRule: base,
			keyword:  payload,
		}, nil

	case RuleTypeIPCIDR, RuleTypeIPCIDR6, RuleTypeSrcIPCIDR:
		prefix, err := netip.ParsePrefix(payload)
		if err != nil {
			return nil, err
		}
		return &IPCIDRRule{
			BaseRule: base,
			prefix:   prefix,
			isSrc:    ruleType == RuleTypeSrcIPCIDR,
		}, nil

	case RuleTypeProcessName:
		return &ProcessNameRule{
			BaseRule: base,
			process:  payload,
		}, nil

	case RuleTypeMatch, RuleTypeFinal:
		return &MatchRule{BaseRule: base}, nil

	default:
		return nil, ErrUnknownRuleType
	}
}

// Rules 返回所有规则列表
func (e *Engine) Rules() []Rule {
	return e.rules
}

// RuleProviderConfig 规则提供者配置 (占位)
type RuleProviderConfig struct{}

// 错误定义
var (
	ErrInvalidRule     = fmt.Errorf("invalid rule")
	ErrUnknownRuleType = fmt.Errorf("unknown rule type")
)
