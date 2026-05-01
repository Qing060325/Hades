// Package outboundgroup 代理组实现
package outboundgroup

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// GroupType 代理组类型
type GroupType string

const (
	GroupSelect    GroupType = "select"
	GroupURLTest   GroupType = "url-test"
	GroupFallback  GroupType = "fallback"
	GroupLoadBalance GroupType = "load-balance"
)

// Group 代理组接口
type Group interface {
	adapter.Adapter
	Proxies() []adapter.Adapter
	Now() string
	Set(name string) error
}

// BaseGroup 基础代理组
type BaseGroup struct {
	name     string
	groupType GroupType
	proxies  []adapter.Adapter
	selected string
	mu       sync.RWMutex
}

// Name 名称
func (g *BaseGroup) Name() string { return g.name }

// Type 类型
func (g *BaseGroup) Type() adapter.AdapterType { return adapter.AdapterType(g.groupType) }

// Addr 地址
func (g *BaseGroup) Addr() string { return "" }

// SupportUDP 支持 UDP
func (g *BaseGroup) SupportUDP() bool { return true }

// SupportWithDialer 支持自定义拨号器
func (g *BaseGroup) SupportWithDialer() bool { return false }

// Now 当前选中的代理
func (g *BaseGroup) Now() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.selected
}

// Proxies 获取代理列表
func (g *BaseGroup) Proxies() []adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.proxies
}

// Set 设置选中的代理
func (g *BaseGroup) Set(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range g.proxies {
		if p.Name() == name {
			g.selected = name
			return nil
		}
	}
	return fmt.Errorf("proxy %s not found in group %s", name, g.name)
}

// DialContext 拨号
func (g *BaseGroup) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	proxy := g.selectedProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no available proxy in group %s", g.name)
	}
	return proxy.DialContext(ctx, metadata)
}

// DialUDPContext UDP 拨号
func (g *BaseGroup) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	proxy := g.selectedProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no available proxy in group %s", g.name)
	}
	return proxy.DialUDPContext(ctx, metadata)
}

// URLTest 健康检查
func (g *BaseGroup) URLTest(ctx context.Context, url string) (time.Duration, error) {
	proxy := g.selectedProxy()
	if proxy == nil {
		return 0, fmt.Errorf("no available proxy")
	}
	return proxy.URLTest(ctx, url)
}

func (g *BaseGroup) selectedProxy() adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.selected != "" {
		for _, p := range g.proxies {
			if p.Name() == g.selected {
				return p
			}
		}
	}

	if len(g.proxies) > 0 {
		return g.proxies[0]
	}
	return nil
}

// SelectGroup 手动选择代理组
type SelectGroup struct {
	BaseGroup
}

// NewSelectGroup 创建手动选择组
func NewSelectGroup(name string, proxies []adapter.Adapter) *SelectGroup {
	g := &SelectGroup{
		BaseGroup: BaseGroup{
			name:      name,
			groupType: GroupSelect,
			proxies:   proxies,
		},
	}
	if len(proxies) > 0 {
		g.selected = proxies[0].Name()
	}
	return g
}

// URLTestGroup 自动测速代理组
type URLTestGroup struct {
	BaseGroup
	testURL  string
	interval time.Duration
	tolerance time.Duration
	lastTest time.Time
}

// NewURLTestGroup 创建自动测速组
func NewURLTestGroup(name string, proxies []adapter.Adapter, testURL string, interval time.Duration) *URLTestGroup {
	return &URLTestGroup{
		BaseGroup: BaseGroup{
			name:      name,
			groupType: GroupURLTest,
			proxies:   proxies,
		},
		testURL:  testURL,
		interval: interval,
	}
}

// StartTest 启动定时测速
func (g *URLTestGroup) StartTest() {
	go func() {
		ticker := time.NewTicker(g.interval)
		defer ticker.Stop()
		for range ticker.C {
			g.testAll()
		}
	}()
}

func (g *URLTestGroup) testAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type result struct {
		name string
		d    time.Duration
	}

	ch := make(chan result, len(g.proxies))
	for _, p := range g.proxies {
		go func(proxy adapter.Adapter) {
			d, err := proxy.URLTest(ctx, g.testURL)
			if err == nil {
				ch <- result{proxy.Name(), d}
			} else {
				ch <- result{proxy.Name(), time.Hour}
			}
		}(p)
	}

	var best result
	best.d = time.Hour
	for i := 0; i < len(g.proxies); i++ {
		r := <-ch
		if r.d < best.d {
			best = r
		}
	}

	if best.d < time.Hour {
		g.mu.Lock()
		g.selected = best.name
		g.lastTest = time.Now()
		g.mu.Unlock()
	}
}

// FallbackGroup 故障转移代理组
type FallbackGroup struct {
	BaseGroup
	testURL  string
	interval time.Duration
}

// NewFallbackGroup 创建故障转移组
func NewFallbackGroup(name string, proxies []adapter.Adapter, testURL string) *FallbackGroup {
	g := &FallbackGroup{
		BaseGroup: BaseGroup{
			name:      name,
			groupType: GroupFallback,
			proxies:   proxies,
		},
		testURL: testURL,
	}
	if len(proxies) > 0 {
		g.selected = proxies[0].Name()
	}
	return g
}

// StartTest 启动健康检查
func (g *FallbackGroup) StartTest() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			g.check()
		}
	}()
}

func (g *FallbackGroup) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, p := range g.proxies {
		_, err := p.URLTest(ctx, g.testURL)
		if err == nil {
			g.mu.Lock()
			g.selected = p.Name()
			g.mu.Unlock()
			return
		}
	}
}

// LoadBalanceGroup 负载均衡代理组
type LoadBalanceGroup struct {
	BaseGroup
	strategy string // round-robin, consistent-hashing
	counter  uint64
}

// NewLoadBalanceGroup 创建负载均衡组
func NewLoadBalanceGroup(name string, proxies []adapter.Adapter, strategy string) *LoadBalanceGroup {
	return &LoadBalanceGroup{
		BaseGroup: BaseGroup{
			name:      name,
			groupType: GroupLoadBalance,
			proxies:   proxies,
		},
		strategy: strategy,
	}
}

// DialContext 负载均衡拨号
func (g *LoadBalanceGroup) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	proxy := g.selectProxy(metadata)
	if proxy == nil {
		return nil, fmt.Errorf("no available proxy")
	}
	return proxy.DialContext(ctx, metadata)
}

func (g *LoadBalanceGroup) selectProxy(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.proxies) == 0 {
		return nil
	}

	switch g.strategy {
	case "consistent-hashing":
		// 基于目标地址的一致性哈希
		idx := hashString(metadata.Host+fmt.Sprintf("%d", metadata.DstPort)) % uint32(len(g.proxies))
		return g.proxies[idx]
	default: // round-robin
		g.counter++
		return g.proxies[g.counter%uint64(len(g.proxies))]
	}
}

func hashString(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// 随机选择
func randomProxy(proxies []adapter.Adapter) adapter.Adapter {
	if len(proxies) == 0 {
		return nil
	}
	return proxies[rand.Intn(len(proxies))]
}
