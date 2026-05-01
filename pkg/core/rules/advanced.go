// Package rules 规则引擎 — 扩展规则类型
package rules

import (
	"net/netip"

	"github.com/Qing060325/Hades/pkg/component/mmdb"
	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// GeoIPRule GeoIP 规则
type GeoIPRule struct {
	BaseRule
	country  string
	isSource bool
}

func NewGeoIPRule(country string, adapterName string, isSource bool) *GeoIPRule {
	return &GeoIPRule{
		BaseRule: BaseRule{adapter: adapterName, payload: country, shouldResolveIP: !isSource},
		country:  country,
		isSource: isSource,
	}
}

func (r *GeoIPRule) Match(metadata *adapter.Metadata) bool {
	var ip netip.Addr
	if r.isSource {
		ip = metadata.SrcIP
	} else {
		ip = metadata.DstIP
	}
	if !ip.IsValid() {
		return false
	}
	code := mmdb.LookupCountry(ip)
	return code == r.country
}

func (r *GeoIPRule) Type() RuleType {
	if r.isSource {
		return RuleTypeSrcGeoIP
	}
	return RuleTypeGeoIP
}

// GeoSiteRule GeoSite 规则
type GeoSiteRule struct {
	BaseRule
	country string
}

func NewGeoSiteRule(country string, adapterName string) *GeoSiteRule {
	return &GeoSiteRule{
		BaseRule: BaseRule{adapter: adapterName, payload: country, shouldResolveIP: true},
		country:  country,
	}
}

func (r *GeoSiteRule) Match(metadata *adapter.Metadata) bool {
	if geoSiteLookup != nil {
		return geoSiteLookup(metadata.Host, r.country)
	}
	return false
}

func (r *GeoSiteRule) Type() RuleType {
	return RuleTypeGeoSite
}

// ProcessPathRule 进程路径规则
type ProcessPathRule struct {
	BaseRule
	processPath string
}

func NewProcessPathRule(path string, adapterName string) *ProcessPathRule {
	return &ProcessPathRule{
		BaseRule:    BaseRule{adapter: adapterName, payload: path, shouldResolveIP: false},
		processPath: path,
	}
}

func (r *ProcessPathRule) Match(metadata *adapter.Metadata) bool {
	return metadata.ProcessPath == r.processPath
}

func (r *ProcessPathRule) Type() RuleType {
	return RuleTypeProcessPath
}

// NetworkRule 网络类型规则 (tcp/udp)
type NetworkRule struct {
	BaseRule
	network string
}

func NewNetworkRule(network string, adapterName string) *NetworkRule {
	return &NetworkRule{
		BaseRule: BaseRule{adapter: adapterName, payload: network, shouldResolveIP: false},
		network:  network,
	}
}

func (r *NetworkRule) Match(metadata *adapter.Metadata) bool {
	return metadata.NetWork == r.network
}

func (r *NetworkRule) Type() RuleType {
	return "NETWORK"
}

// DSCPRule DSCP 规则
type DSCPRule struct {
	BaseRule
	dscp int
}

func NewDSCPRule(dscp int, adapterName string) *DSCPRule {
	return &DSCPRule{
		BaseRule: BaseRule{adapter: adapterName, payload: "", shouldResolveIP: false},
		dscp:     dscp,
	}
}

func (r *DSCPRule) Match(metadata *adapter.Metadata) bool {
	return metadata.DSCP == r.dscp
}

func (r *DSCPRule) Type() RuleType {
	return "DSCP"
}

// ASNRule ASN 规则
type ASNRule struct {
	BaseRule
	asn uint
}

func NewASNRule(asn uint, adapterName string) *ASNRule {
	return &ASNRule{
		BaseRule: BaseRule{adapter: adapterName, payload: "", shouldResolveIP: false},
		asn:      asn,
	}
}

func (r *ASNRule) Match(metadata *adapter.Metadata) bool {
	return metadata.ASN == r.asn
}

func (r *ASNRule) Type() RuleType {
	return "ASN"
}

// InNameRule 入站名称规则
type InNameRule struct {
	BaseRule
	inName string
}

func NewInNameRule(name string, adapterName string) *InNameRule {
	return &InNameRule{
		BaseRule: BaseRule{adapter: adapterName, payload: name, shouldResolveIP: false},
		inName:   name,
	}
}

func (r *InNameRule) Match(metadata *adapter.Metadata) bool {
	return metadata.InName == r.inName
}

func (r *InNameRule) Type() RuleType {
	return "IN-NAME"
}

// InTypeRule 入站类型规则
type InTypeRule struct {
	BaseRule
	inType string
}

func NewInTypeRule(t string, adapterName string) *InTypeRule {
	return &InTypeRule{
		BaseRule: BaseRule{adapter: adapterName, payload: t, shouldResolveIP: false},
		inType:   t,
	}
}

func (r *InTypeRule) Match(metadata *adapter.Metadata) bool {
	return string(metadata.Type) == r.inType
}

func (r *InTypeRule) Type() RuleType {
	return "IN-TYPE"
}

// PortRule 端口规则
type PortRule struct {
	BaseRule
	port uint16
}

func NewPortRule(port uint16, adapterName string) *PortRule {
	return &PortRule{
		BaseRule: BaseRule{adapter: adapterName, payload: "", shouldResolveIP: false},
		port:     port,
	}
}

func (r *PortRule) Match(metadata *adapter.Metadata) bool {
	return metadata.DstPort == r.port
}

func (r *PortRule) Type() RuleType {
	return "PORT"
}

// SrcPortRule 源端口规则
type SrcPortRule struct {
	BaseRule
	port uint16
}

func NewSrcPortRule(port uint16, adapterName string) *SrcPortRule {
	return &SrcPortRule{
		BaseRule: BaseRule{adapter: adapterName, payload: "", shouldResolveIP: false},
		port:     port,
	}
}

func (r *SrcPortRule) Match(metadata *adapter.Metadata) bool {
	return metadata.SrcPort == r.port
}

func (r *SrcPortRule) Type() RuleType {
	return "SRC-PORT"
}

// GeoSite 查询函数变量
var geoSiteLookup func(string, string) bool

func SetGeoSiteLookup(fn func(string, string) bool) {
	geoSiteLookup = fn
}
