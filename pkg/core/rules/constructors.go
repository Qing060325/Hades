// Package rules 规则构造函数
package rules

import "net/netip"

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
