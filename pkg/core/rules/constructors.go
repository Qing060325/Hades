// Package rules 规则构造函数
package rules

import (
	"net/netip"
	"strconv"
)

// NewDomainRule 创建域名规则
func NewDomainRule(domain string, adapterName string) *DomainRule {
	return &DomainRule{
		BaseRule: BaseRule{adapter: adapterName, payload: domain, shouldResolveIP: true},
		domain:   domain,
	}
}

// NewDomainSuffixRule 创建域名后缀规则
func NewDomainSuffixRule(suffix string, adapterName string) *DomainSuffixRule {
	return &DomainSuffixRule{
		BaseRule: BaseRule{adapter: adapterName, payload: suffix, shouldResolveIP: true},
		suffix:   suffix,
	}
}

// NewDomainKeywordRule 创建域名关键词规则
func NewDomainKeywordRule(keyword string, adapterName string) *DomainKeywordRule {
	return &DomainKeywordRule{
		BaseRule: BaseRule{adapter: adapterName, payload: keyword, shouldResolveIP: true},
		keyword:  keyword,
	}
}

// NewDomainWildcardRule 创建域名通配符规则
func NewDomainWildcardRule(pattern string, adapterName string) *DomainWildcardRule {
	return &DomainWildcardRule{
		BaseRule: BaseRule{adapter: adapterName, payload: pattern, shouldResolveIP: true},
	}
}

// NewIPCIDRRule 创建 IP CIDR 规则
func NewIPCIDRRule(cidr string, adapterName string, isSrc bool) *IPCIDRRule {
	prefix, _ := netip.ParsePrefix(cidr)
	return &IPCIDRRule{
		BaseRule: BaseRule{adapter: adapterName, payload: cidr, shouldResolveIP: !isSrc},
		prefix:   prefix,
		isSrc:    isSrc,
	}
}

// NewProcessNameRule 创建进程名规则
func NewProcessNameRule(name string, adapterName string) *ProcessNameRule {
	return &ProcessNameRule{
		BaseRule: BaseRule{adapter: adapterName, payload: name, shouldResolveIP: false},
		process:  name,
	}
}

// NewMatchRule 创建匹配规则
func NewMatchRule(adapterName string) *MatchRule {
	return &MatchRule{
		BaseRule: BaseRule{adapter: adapterName, payload: "", shouldResolveIP: true},
	}
}

// NewGeoSiteRule 创建 GeoSite 规则
func NewGeoSiteRuleConstructor(country string, adapterName string) *GeoSiteRule {
	return NewGeoSiteRule(country, adapterName)
}

// NewProcessPathRule 创建进程路径规则
func NewProcessPathRuleConstructor(path string, adapterName string) *ProcessPathRule {
	return NewProcessPathRule(path, adapterName)
}

// NewNetworkRule 创建网络类型规则
func NewNetworkRuleConstructor(network string, adapterName string) *NetworkRule {
	return NewNetworkRule(network, adapterName)
}

// NewPortRule 创建端口规则
func NewPortRuleConstructor(port uint16, adapterName string) *PortRule {
	return NewPortRule(port, adapterName)
}

// NewPortRuleFromString 从字符串创建端口规则
func NewPortRuleFromString(portStr string, adapterName string) (*PortRule, error) {
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	return NewPortRule(uint16(port), adapterName), nil
}

// NewSrcPortRule 创建源端口规则
func NewSrcPortRuleConstructor(port uint16, adapterName string) *SrcPortRule {
	return NewSrcPortRule(port, adapterName)
}

// NewSrcPortRuleFromString 从字符串创建源端口规则
func NewSrcPortRuleFromString(portStr string, adapterName string) (*SrcPortRule, error) {
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	return NewSrcPortRule(uint16(port), adapterName), nil
}

// NewInNameRule 创建入站名称规则
func NewInNameRuleConstructor(name string, adapterName string) *InNameRule {
	return NewInNameRule(name, adapterName)
}

// NewInTypeRule 创建入站类型规则
func NewInTypeRuleConstructor(t string, adapterName string) *InTypeRule {
	return NewInTypeRule(t, adapterName)
}

// NewDSCPRule 创建 DSCP 规则
func NewDSCPRuleConstructor(dscp int, adapterName string) *DSCPRule {
	return NewDSCPRule(dscp, adapterName)
}

// NewDSCPRuleFromString 从字符串创建 DSCP 规则
func NewDSCPRuleFromString(dscpStr string, adapterName string) (*DSCPRule, error) {
	dscp, err := strconv.Atoi(dscpStr)
	if err != nil {
		return nil, err
	}
	return NewDSCPRule(dscp, adapterName), nil
}

// NewASNRule 创建 ASN 规则
func NewASNRuleConstructor(asn uint, adapterName string) *ASNRule {
	return NewASNRule(asn, adapterName)
}

// NewASNRuleFromString 从字符串创建 ASN 规则
func NewASNRuleFromString(asnStr string, adapterName string) (*ASNRule, error) {
	asn, err := strconv.ParseUint(asnStr, 10, 32)
	if err != nil {
		return nil, err
	}
	return NewASNRule(uint(asn), adapterName), nil
}

// NewGeoIPRuleConstructor 创建 GeoIP 规则
func NewGeoIPRuleConstructor(country string, adapterName string, isSource bool) *GeoIPRule {
	return NewGeoIPRule(country, adapterName, isSource)
}
