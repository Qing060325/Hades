// Package rules 规则引擎模块
package rules

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
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
	RuleTypeNetwork       RuleType = "NETWORK"
	RuleTypePort          RuleType = "PORT"
	RuleTypeSrcPort       RuleType = "SRC-PORT"
	RuleTypeInName        RuleType = "IN-NAME"
	RuleTypeInType        RuleType = "IN-TYPE"
	RuleTypeDSCP          RuleType = "DSCP"
	RuleTypeASN           RuleType = "ASN"
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
	case "AND", "OR", "NOT":
		return parseInlineLogicalRule(string(ruleType), payload, adapterName)

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

	case RuleTypeProcessPath:
		return &ProcessPathRule{
			BaseRule:    base,
			processPath: payload,
		}, nil

	case RuleTypeGeoSite:
		return &GeoSiteRule{
			BaseRule: base,
			country:  payload,
		}, nil

	case RuleTypeGeoIP, RuleTypeSrcGeoIP:
		isSource := ruleType == RuleTypeSrcGeoIP
		return &GeoIPRule{
			BaseRule: base,
			country:  payload,
			isSource: isSource,
		}, nil

	case RuleTypeNetwork:
		return &NetworkRule{
			BaseRule: base,
			network:  strings.ToLower(payload),
		}, nil

	case RuleTypePort:
		port, err := strconv.ParseUint(payload, 10, 16)
		if err != nil {
			return nil, err
		}
		return &PortRule{
			BaseRule: base,
			port:     uint16(port),
		}, nil

	case RuleTypeSrcPort:
		port, err := strconv.ParseUint(payload, 10, 16)
		if err != nil {
			return nil, err
		}
		return &SrcPortRule{
			BaseRule: base,
			port:     uint16(port),
		}, nil

	case RuleTypeInName:
		return &InNameRule{
			BaseRule: base,
			inName:   payload,
		}, nil

	case RuleTypeInType:
		return &InTypeRule{
			BaseRule: base,
			inType:   payload,
		}, nil

	case RuleTypeDSCP:
		dscp, err := strconv.Atoi(payload)
		if err != nil {
			return nil, err
		}
		return &DSCPRule{
			BaseRule: base,
			dscp:     dscp,
		}, nil

	case RuleTypeASN:
		asn, err := strconv.ParseUint(payload, 10, 32)
		if err != nil {
			return nil, err
		}
		return &ASNRule{
			BaseRule: base,
			asn:      uint(asn),
		}, nil

	case RuleTypeRuleSet:
		return &RuleSetRule{
			BaseRule: base,
			name:     payload,
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

// AddRule 动态添加规则
func (e *Engine) AddRule(rule Rule) {
	e.rules = append(e.rules, rule)
}

// RemoveRule 按索引移除规则
func (e *Engine) RemoveRule(index int) error {
	if index < 0 || index >= len(e.rules) {
		return fmt.Errorf("rule index %d out of range [0, %d)", index, len(e.rules))
	}
	e.rules = append(e.rules[:index], e.rules[index+1:]...)
	return nil
}

// RulesCount 返回规则数量
func (e *Engine) RulesCount() int {
	return len(e.rules)
}

// MatchWithRule 匹配规则并返回适配器名称和匹配的规则
func (e *Engine) MatchWithRule(metadata *adapter.Metadata) (string, Rule) {
	for _, rule := range e.rules {
		if rule.Match(metadata) {
			return rule.Adapter(), rule
		}
	}
	return "DIRECT", nil
}

// RuleSetRule 规则集规则
type RuleSetRule struct {
	BaseRule
	name string
}

// Match 匹配（委托给引擎的规则集）
func (r *RuleSetRule) Match(metadata *adapter.Metadata) bool {
	return false
}

// Type 返回类型
func (r *RuleSetRule) Type() RuleType {
	return RuleTypeRuleSet
}

// InlineLogicalRule 内联逻辑规则（AND/OR/NOT）
type InlineLogicalRule struct {
	BaseRule
	logicType string
	subRules  []Rule
}

// Match 匹配
func (r *InlineLogicalRule) Match(metadata *adapter.Metadata) bool {
	switch r.logicType {
	case "AND":
		for _, rule := range r.subRules {
			if !rule.Match(metadata) {
				return false
			}
		}
		return true
	case "OR":
		for _, rule := range r.subRules {
			if rule.Match(metadata) {
				return true
			}
		}
		return false
	case "NOT":
		if len(r.subRules) > 0 {
			return !r.subRules[0].Match(metadata)
		}
		return false
	}
	return false
}

// Type 返回类型
func (r *InlineLogicalRule) Type() RuleType {
	return RuleType("LOGICAL-" + r.logicType)
}

// parseInlineLogicalRule 解析内联逻辑规则
// 格式: AND,((RULE1),(RULE2)),ADAPTER
func parseInlineLogicalRule(logicType string, payload string, adapterName string) (Rule, error) {
	// 去掉外层括号
	body := payload
	if strings.HasPrefix(body, "(") && strings.HasSuffix(body, ")") {
		body = body[1 : len(body)-1]
	}

	subRuleStrs := splitInlineLogicalRules(body)
	subRules := make([]Rule, 0, len(subRuleStrs))

	for _, subRuleStr := range subRuleStrs {
		subRuleStr = strings.TrimSpace(subRuleStr)
		if subRuleStr == "" {
			continue
		}
		if strings.HasPrefix(subRuleStr, "(") && strings.HasSuffix(subRuleStr, ")") {
			subRuleStr = subRuleStr[1 : len(subRuleStr)-1]
		}
		rule, err := ParseRule(subRuleStr)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			subRules = append(subRules, rule)
		}
	}

	return &InlineLogicalRule{
		BaseRule:  BaseRule{adapter: adapterName, payload: logicType},
		logicType: logicType,
		subRules:  subRules,
	}, nil
}

// splitInlineLogicalRules 按逗号分割，但尊重括号嵌套
func splitInlineLogicalRules(s string) []string {
	var result []string
	depth := 0
	start := 0

	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}

	return result
}

// RuleProviderConfig 规则提供者配置 (占位)
type RuleProviderConfig struct{}

// 错误定义
var (
	ErrInvalidRule     = fmt.Errorf("invalid rule")
	ErrUnknownRuleType = fmt.Errorf("unknown rule type")
)
