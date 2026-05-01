// Package dns DNS 解析器增强
package dns

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/component/fakeip"
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

	// 创建 nameserver resolver
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
	for _, resolver := range r.resolvers {
		ips, err := resolver.LookupIPAddr(ctx, domain)
		if err == nil {
			result := make([]net.IP, len(ips))
			for i, ip := range ips {
				result[i] = ip.IP
			}
			return result, nil
		}
	}

	// 回退到系统 DNS
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}

	result := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.IP
	}
	return result, nil
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
