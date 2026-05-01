// Package cidr CIDR 工具
package cidr

import (
	"net/netip"
)

// Contains 检查 IP 是否在 CIDR 范围内
func Contains(prefix netip.Prefix, addr netip.Addr) bool {
	return prefix.Contains(addr)
}

// ContainsAny 检查 IP 是否在任一 CIDR 范围内
func ContainsAny(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParsePrefix 解析 CIDR 字符串
func ParsePrefix(s string) (netip.Prefix, error) {
	return netip.ParsePrefix(s)
}

// ParsePrefixes 批量解析 CIDR
func ParsePrefixes(ss []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(ss))
	for _, s := range ss {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, p)
	}
	return prefixes, nil
}
