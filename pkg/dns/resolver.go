// Package dns DNS 解析器增强
package dns

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/component/cidr"
	"github.com/Qing060325/Hades/pkg/component/fakeip"
	"github.com/Qing060325/Hades/pkg/component/geodata"
	"github.com/Qing060325/Hades/pkg/component/mmdb"
)

// Mode DNS 模式
type Mode string

const (
	ModeNormal  Mode = "normal"
	ModeFakeIP  Mode = "fake-ip"
	ModeMapping Mode = "mapping"
)

// Config DNS 配置
type Config struct {
	Enable       bool     `yaml:"enable"`
	Listen       string   `yaml:"listen"`
	EnhancedMode Mode     `yaml:"enhanced-mode"`
	FakeIPRange  string   `yaml:"fake-ip-range"`
	Nameserver   []string `yaml:"nameserver"`
	Fallback     []string `yaml:"fallback"`
	Hosts        map[string]string `yaml:"hosts"`
	FallbackFilter *FallbackFilter `yaml:"fallback-filter"`
	DefaultNameserver []string `yaml:"default-nameserver"`
}

// FallbackFilter 回退过滤器
type FallbackFilter struct {
	GeoIP     bool     `yaml:"geoip"`
	IPCIDR    []string `yaml:"ipcidr"`
	Domain    []string `yaml:"domain"`
	GeoIPCode string   `yaml:"geoip-code"`
}

// Resolver DNS 解析器
type Resolver struct {
	config     *Config
	fakeIPPool *fakeip.Pool
	resolvers  []*net.Resolver
	mu         sync.RWMutex
	cache      map[string]*CacheEntry

	// 回退解析器
	defaultResolvers  []*net.Resolver // default-nameserver (用于 IP 解析)
	fallbackResolvers []*net.Resolver // fallback (用于域名解析)

	// 回退过滤器
	fallbackFilter *FallbackFilter
	ipcidrPrefixes []netip.Prefix // CIDR 过滤规则
	geoIPCode      string         // GeoIP 国家代码
	geositeDomains []string       // GeoSite 域名列表
}

// CacheEntry 缓存条目
type CacheEntry struct {
	IPs       []net.IP
	ExpiresAt time.Time
}

// NewResolver 创建解析器
func NewResolver(cfg *Config) (*Resolver, error) {
	r := &Resolver{
		config:    cfg,
		cache:     make(map[string]*CacheEntry),
		resolvers: make([]*net.Resolver, 0),
	}

	// 创建 nameserver resolver (用于域名解析)
	for _, ns := range cfg.Nameserver {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", ns)
			},
		}
		r.resolvers = append(r.resolvers, resolver)
	}

	// 创建 default-nameserver resolver (用于 IP 地址解析，避免污染)
	if cfg.DefaultNameserver != nil {
		for _, ns := range cfg.DefaultNameserver {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: 5 * time.Second}
					return d.DialContext(ctx, "udp", ns)
				},
			}
			r.defaultResolvers = append(r.defaultResolvers, resolver)
		}
	}

	// 创建 fallback resolver (用于回退解析)
	if cfg.Fallback != nil {
		for _, ns := range cfg.Fallback {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: 5 * time.Second}
					return d.DialContext(ctx, "udp", ns)
				},
			}
			r.fallbackResolvers = append(r.fallbackResolvers, resolver)
		}
	}

	// 初始化回退过滤器
	if cfg.FallbackFilter != nil {
		r.fallbackFilter = cfg.FallbackFilter

		// 初始化 GeoIP 国家代码
		if cfg.FallbackFilter.GeoIP && cfg.FallbackFilter.GeoIPCode != "" {
			r.geoIPCode = cfg.FallbackFilter.GeoIPCode
		}

		// 初始化 CIDR 前缀列表
		if len(cfg.FallbackFilter.IPCIDR) > 0 {
			prefixes, err := cidr.ParsePrefixes(cfg.FallbackFilter.IPCIDR)
			if err == nil {
				r.ipcidrPrefixes = prefixes
			}
		}

		// 初始化 GeoSite 域名列表
		if len(cfg.FallbackFilter.Domain) > 0 {
			r.geositeDomains = cfg.FallbackFilter.Domain
		}
	}

	// 初始化 Fake-IP 池
	if cfg.EnhancedMode == ModeFakeIP && cfg.FakeIPRange != "" {
		pool, err := fakeip.NewPool(cfg.FakeIPRange)
		if err != nil {
			return nil, fmt.Errorf("init fakeip pool: %w", err)
		}
		r.fakeIPPool = pool
	}

	return r, nil
}

// Resolve 解析域名
func (r *Resolver) Resolve(ctx context.Context, domain string) ([]net.IP, error) {
	// 检查 hosts
	if r.config.Hosts != nil {
		if ip, ok := r.config.Hosts[domain]; ok {
			return []net.IP{net.ParseIP(ip)}, nil
		}
	}

	// Fake-IP 模式
	if r.config.EnhancedMode == ModeFakeIP && r.fakeIPPool != nil {
		fakeIP := r.fakeIPPool.Get(domain)
		return []net.IP{fakeIP}, nil
	}

	// 检查缓存
	r.mu.RLock()
	if entry, ok := r.cache[domain]; ok && time.Now().Before(entry.ExpiresAt) {
		r.mu.RUnlock()
		return entry.IPs, nil
	}
	r.mu.RUnlock()

	// 解析
	ips, err := r.lookup(ctx, domain)
	if err != nil {
		return nil, err
	}

	// 缓存
	r.mu.Lock()
	r.cache[domain] = &CacheEntry{
		IPs:       ips,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	r.mu.Unlock()

	return ips, nil
}

func (r *Resolver) lookup(ctx context.Context, domain string) ([]net.IP, error) {
	// 0. 域名级回退过滤检查
	if r.shouldFallbackByDomain(domain) {
		return r.lookupFallback(ctx, domain)
	}

	// 1. 先用 default-nameserver 解析（IP 解析，避免污染）
	if len(r.defaultResolvers) > 0 {
		for _, resolver := range r.defaultResolvers {
			ips, err := resolver.LookupIPAddr(ctx, domain)
			if err == nil {
				result := make([]net.IP, len(ips))
				for i, ip := range ips {
					result[i] = ip.IP
				}
				return result, nil
			}
		}
	}

	// 2. 使用 nameserver 解析（域名解析）
	for _, resolver := range r.resolvers {
		ips, err := resolver.LookupIPAddr(ctx, domain)
		if err == nil {
			result := make([]net.IP, len(ips))
			for i, ip := range ips {
				result[i] = ip.IP
			}

			// 3. 检查 IP 级回退过滤
			if r.shouldFallback(result) {
				return r.lookupFallback(ctx, domain)
			}

			return result, nil
		}
	}

	// 4. 全部失败，使用 fallback
	return r.lookupFallback(ctx, domain)
}

// shouldFallback 检查解析结果是否需要回退
func (r *Resolver) shouldFallback(ips []net.IP) bool {
	if r.fallbackFilter == nil {
		return false
	}

	for _, ip := range ips {
		// GeoIP 检查
		if r.fallbackFilter.GeoIP && r.geoIPCode != "" {
			addr, ok := netip.AddrFromSlice(ip)
			if ok {
				country := mmdb.LookupCountry(addr)
				if country == r.geoIPCode {
					return true
				}
			}
		}

		// CIDR 检查
		if len(r.ipcidrPrefixes) > 0 {
			addr, ok := netip.AddrFromSlice(ip)
			if ok && cidr.ContainsAny(r.ipcidrPrefixes, addr) {
				return true
			}
		}
	}

	return false
}

// shouldFallbackByDomain 检查域名是否需要回退
func (r *Resolver) shouldFallbackByDomain(domain string) bool {
	if r.fallbackFilter == nil || len(r.geositeDomains) == 0 {
		return false
	}
	for _, geoCountry := range r.geositeDomains {
		if geodata.LookupGeoSite(domain, geoCountry) {
			return true
		}
	}
	return false
}

// lookupFallback 使用 fallback 解析器解析
func (r *Resolver) lookupFallback(ctx context.Context, domain string) ([]net.IP, error) {
	if len(r.fallbackResolvers) == 0 {
		return nil, fmt.Errorf("no fallback resolvers configured")
	}

	for _, resolver := range r.fallbackResolvers {
		ips, err := resolver.LookupIPAddr(ctx, domain)
		if err == nil {
			result := make([]net.IP, len(ips))
			for i, ip := range ips {
				result[i] = ip.IP
			}
			return result, nil
		}
	}

	return nil, fmt.Errorf("all fallback resolvers failed for %s", domain)
}

// IsFakeIP 检查是否是 Fake-IP
func (r *Resolver) IsFakeIP(ip net.IP) bool {
	if r.fakeIPPool == nil {
		return false
	}
	return r.fakeIPPool.IsFakeIP(ip)
}

// LookupFakeIP 查找 Fake-IP 对应域名
func (r *Resolver) LookupFakeIP(ip net.IP) (string, bool) {
	if r.fakeIPPool == nil {
		return "", false
	}
	return r.fakeIPPool.Lookup(ip)
}
