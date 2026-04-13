// Package dns DNS解析模块
package dns

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hades/hades/internal/config"
	"github.com/rs/zerolog/log"
)

// Client DNS客户端
type Client struct {
	cfg   *config.DNSConfig
	cache *LRUCache

	// Fake-IP
	fakeIPPool *FakeIPPool

	// DNS服务器客户端
	nameservers []Resolver

	mu sync.RWMutex
}

// Resolver DNS解析器接口
type Resolver interface {
	Resolve(ctx context.Context, host string) ([]net.IP, error)
}

// NewClient 创建DNS客户端
func NewClient(cfg *config.DNSConfig) (*Client, error) {
	c := &Client{
		cfg:         cfg,
		cache:       NewLRUCache(4096),
		nameservers: make([]Resolver, 0),
	}

	// 初始化Fake-IP池
	if cfg.EnhancedMode == "fake-ip" {
		c.fakeIPPool = NewFakeIPPool(cfg.FakeIPRange)
	}

	// 初始化DNS服务器
	for _, ns := range cfg.Nameserver {
		r, err := NewResolver(ns)
		if err != nil {
			log.Warn().Err(err).Str("nameserver", ns).Msg("初始化DNS服务器失败")
			continue
		}
		c.nameservers = append(c.nameservers, r)
	}

	return c, nil
}

// Listen 启动DNS服务
func (c *Client) Listen(ctx context.Context, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Info().Str("addr", addr).Msg("DNS服务已启动")

	buf := make([]byte, 512)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			go c.handleDNSQuery(conn, remoteAddr, buf[:n])
		}
	}
}

// handleDNSQuery 处理DNS查询
func (c *Client) handleDNSQuery(conn *net.UDPConn, remoteAddr *net.UDPAddr, query []byte) {
	// TODO: 实现完整的DNS查询处理
	// 1. 解析DNS查询
	// 2. 检查缓存
	// 3. 转发查询或返回Fake-IP
	// 4. 发送响应

	conn.WriteToUDP(query, remoteAddr)
}

// Resolve 解析域名
func (c *Client) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// 检查缓存
	if ips, ok := c.cache.Get(host); ok {
		return ips, nil
	}

	// 使用DNS服务器解析
	for _, r := range c.nameservers {
		ips, err := r.Resolve(ctx, host)
		if err == nil {
			c.cache.Set(host, ips, time.Minute*5)
			return ips, nil
		}
	}

	return nil, fmt.Errorf("解析失败: %s", host)
}

// FakeIP 获取Fake-IP
func (c *Client) FakeIP(host string) net.IP {
	if c.fakeIPPool != nil {
		return c.fakeIPPool.Get(host)
	}
	return nil
}

// LRUCache LRU缓存
type LRUCache struct {
	capacity int
	data     map[string]cacheEntry
	mu       sync.RWMutex
}

type cacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
}

// NewLRUCache 创建LRU缓存
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		data:     make(map[string]cacheEntry),
	}
}

// Get 获取缓存
func (c *LRUCache) Get(key string) ([]net.IP, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.ips, true
}

// Set 设置缓存
func (c *LRUCache) Set(key string, ips []net.IP, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheEntry{
		ips:       ips,
		expiresAt: time.Now().Add(ttl),
	}
}

// FakeIPPool Fake-IP池
type FakeIPPool struct {
	prefix string
	mu     sync.RWMutex
	hostToIP map[string]net.IP
	ipToHost map[string]string
	counter uint32
}

// NewFakeIPPool 创建Fake-IP池
func NewFakeIPPool(rangeStr string) *FakeIPPool {
	return &FakeIPPool{
		prefix:   "198.18.0.",
		hostToIP: make(map[string]net.IP),
		ipToHost: make(map[string]string),
		counter:  1,
	}
}

// Get 获取或分配Fake-IP
func (p *FakeIPPool) Get(host string) net.IP {
	p.mu.RLock()
	if ip, ok := p.hostToIP[host]; ok {
		p.mu.RUnlock()
		return ip
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// 再次检查
	if ip, ok := p.hostToIP[host]; ok {
		return ip
	}

	// 分配新IP
	ip := net.ParseIP(fmt.Sprintf("%s%d", p.prefix, p.counter))
	p.counter++
	p.hostToIP[host] = ip
	p.ipToHost[ip.String()] = host

	return ip
}

// Lookup 通过IP查找主机名
func (p *FakeIPPool) Lookup(ip string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ipToHost[ip]
}

// Resolver 实现 (占位)
type resolver struct {
	addr string
}

func NewResolver(addr string) (Resolver, error) {
	return &resolver{addr: addr}, nil
}

func (r *resolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// TODO: 实现完整的DNS解析
	return net.LookupIP(host)
}
