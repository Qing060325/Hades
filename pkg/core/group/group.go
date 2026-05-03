// Package group 代理组模块
package group

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// GroupType 代理组类型
type GroupType string

const (
	GroupTypeSelect      GroupType = "select"
	GroupTypeURLTest     GroupType = "url-test"
	GroupTypeFallback    GroupType = "fallback"
	GroupTypeLoadBalance GroupType = "load-balance"
	GroupTypeRelay       GroupType = "relay"
)

// Group 代理组接口
type Group interface {
	Name() string
	Type() GroupType
	Select(metadata *adapter.Metadata) adapter.Adapter
	HealthCheck() error
	Proxies() []adapter.Adapter
}

// SelectGroup 手动选择组
type SelectGroup struct {
	name       string
	proxies    []adapter.Adapter
	proxyNames []string
	selected   int
	mu         sync.RWMutex
}

// NewSelectGroup 创建手动选择组
func NewSelectGroup(name string, proxies []adapter.Adapter) *SelectGroup {
	return &SelectGroup{
		name:     name,
		proxies:  proxies,
		selected: 0,
	}
}

// Name 返回名称
func (g *SelectGroup) Name() string {
	return g.name
}

// Type 返回类型
func (g *SelectGroup) Type() GroupType {
	return GroupTypeSelect
}

// Select 选择代理
func (g *SelectGroup) Select(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.selected < len(g.proxies) {
		return g.proxies[g.selected]
	}
	return g.proxies[0]
}

// SetSelected 设置选中代理
func (g *SelectGroup) SetSelected(index int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if index >= 0 && index < len(g.proxies) {
		g.selected = index
	}
}

// HealthCheck 健康检查
func (g *SelectGroup) HealthCheck() error {
	return nil
}

// Proxies 返回代理列表
func (g *SelectGroup) Proxies() []adapter.Adapter {
	return g.proxies
}

// URLTestGroup 自动测速组
type URLTestGroup struct {
	name       string
	proxies    []adapter.Adapter
	url        string
	interval   time.Duration
	tolerance  time.Duration
	timeout    time.Duration
	fastest    adapter.Adapter
	lastTest   time.Time
	mu         sync.RWMutex
}

// NewURLTestGroup 创建自动测速组
func NewURLTestGroup(name string, proxies []adapter.Adapter, url string, interval, tolerance, timeout time.Duration) *URLTestGroup {
	g := &URLTestGroup{
		name:      name,
		proxies:   proxies,
		url:       url,
		interval:  interval,
		tolerance: tolerance,
		timeout:   timeout,
	}

	// 初始测速
	g.HealthCheck()

	return g
}

// Name 返回名称
func (g *URLTestGroup) Name() string {
	return g.name
}

// Type 返回类型
func (g *URLTestGroup) Type() GroupType {
	return GroupTypeURLTest
}

// Select 选择最快代理
func (g *URLTestGroup) Select(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.fastest != nil {
		return g.fastest
	}

	if len(g.proxies) > 0 {
		return g.proxies[0]
	}
	return nil
}

// HealthCheck 健康检查
func (g *URLTestGroup) HealthCheck() error {
	if len(g.proxies) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	type result struct {
		adapter adapter.Adapter
		delay   time.Duration
		err     error
	}

	results := make(chan result, len(g.proxies))
	var wg sync.WaitGroup

	for _, adapt := range g.proxies {
		wg.Add(1)
		go func(a adapter.Adapter) {
			defer wg.Done()
			delay, err := a.URLTest(ctx, g.url)
			results <- result{adapter: a, delay: delay, err: err}
		}(adapt)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var fastest adapter.Adapter
	var minDelay time.Duration = time.Hour

	for res := range results {
		if res.err == nil && res.delay < minDelay {
			minDelay = res.delay
			fastest = res.adapter
		}
	}

	g.mu.Lock()
	if fastest != nil {
		g.fastest = fastest
	}
	g.lastTest = time.Now()
	g.mu.Unlock()

	return nil
}

// Proxies 返回代理列表
func (g *URLTestGroup) Proxies() []adapter.Adapter {
	return g.proxies
}

// FallbackGroup 故障转移组
type FallbackGroup struct {
	name       string
	proxies    []adapter.Adapter
	url        string
	interval   time.Duration
	timeout    time.Duration
	active     int
	mu         sync.RWMutex
}

// NewFallbackGroup 创建故障转移组
func NewFallbackGroup(name string, proxies []adapter.Adapter, url string, interval, timeout time.Duration) *FallbackGroup {
	g := &FallbackGroup{
		name:     name,
		proxies:  proxies,
		url:      url,
		interval: interval,
		timeout:  timeout,
		active:   0,
	}

	g.HealthCheck()
	return g
}

// Name 返回名称
func (g *FallbackGroup) Name() string {
	return g.name
}

// Type 返回类型
func (g *FallbackGroup) Type() GroupType {
	return GroupTypeFallback
}

// Select 选择可用代理
func (g *FallbackGroup) Select(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.active < len(g.proxies) {
		return g.proxies[g.active]
	}
	return nil
}

// HealthCheck 健康检查
func (g *FallbackGroup) HealthCheck() error {
	if len(g.proxies) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	for i, adapt := range g.proxies {
		_, err := adapt.URLTest(ctx, g.url)
		if err == nil {
			g.mu.Lock()
			g.active = i
			g.mu.Unlock()
			return nil
		}
	}

	return fmt.Errorf("所有代理不可用")
}

// Proxies 返回代理列表
func (g *FallbackGroup) Proxies() []adapter.Adapter {
	return g.proxies
}

// LoadBalanceGroup 负载均衡组
type LoadBalanceGroup struct {
	name      string
	proxies   []adapter.Adapter
	strategy  BalanceStrategy
	index     uint64
	mu        sync.Mutex
}

// BalanceStrategy 负载均衡策略
type BalanceStrategy string

const (
	BalanceRoundRobin     BalanceStrategy = "round-robin"
	BalanceConsistentHash BalanceStrategy = "consistent-hashing"
)

// NewLoadBalanceGroup 创建负载均衡组
func NewLoadBalanceGroup(name string, proxies []adapter.Adapter, strategy BalanceStrategy) *LoadBalanceGroup {
	return &LoadBalanceGroup{
		name:     name,
		proxies:  proxies,
		strategy: strategy,
	}
}

// Name 返回名称
func (g *LoadBalanceGroup) Name() string {
	return g.name
}

// Type 返回类型
func (g *LoadBalanceGroup) Type() GroupType {
	return GroupTypeLoadBalance
}

// Select 选择代理（根据策略）
func (g *LoadBalanceGroup) Select(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.proxies) == 0 {
		return nil
	}

	switch g.strategy {
	case BalanceConsistentHash:
		return g.selectConsistentHash(metadata)
	default: // round-robin
		idx := g.index % uint64(len(g.proxies))
		g.index++
		return g.proxies[idx]
	}
}

// selectConsistentHash 一致性哈希选择代理
func (g *LoadBalanceGroup) selectConsistentHash(metadata *adapter.Metadata) adapter.Adapter {
	key := fmt.Sprintf("%s:%d", metadata.Host, metadata.DstPort)
	hash := fnv32(key)
	idx := hash % uint32(len(g.proxies))
	return g.proxies[idx]
}

// HealthCheck 健康检查
func (g *LoadBalanceGroup) HealthCheck() error {
	return nil
}

// Proxies 返回代理列表
func (g *LoadBalanceGroup) Proxies() []adapter.Adapter {
	return g.proxies
}

// RelayGroup 中继组 - 按顺序链式连接代理
type RelayGroup struct {
	name    string
	proxies []adapter.Adapter
	mu      sync.RWMutex
}

// NewRelayGroup 创建中继组
func NewRelayGroup(name string, proxies []adapter.Adapter) *RelayGroup {
	return &RelayGroup{
		name:    name,
		proxies: proxies,
	}
}

// Name 返回名称
func (g *RelayGroup) Name() string {
	return g.name
}

// Type 返回类型
func (g *RelayGroup) Type() GroupType {
	return GroupTypeRelay
}

// Select 返回链式代理适配器
// 中继组的 Select 返回第一个代理，由调用方按顺序链式连接
func (g *RelayGroup) Select(metadata *adapter.Metadata) adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.proxies) == 0 {
		return nil
	}

	// 返回第一个代理；真正的链式连接由 RelayAdapter 或上层逻辑处理
	return g.proxies[0]
}

// Chain 返回按顺序排列的代理链
func (g *RelayGroup) Chain() []adapter.Adapter {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]adapter.Adapter, len(g.proxies))
	copy(result, g.proxies)
	return result
}

// HealthCheck 检查链中所有代理的健康状态
func (g *RelayGroup) HealthCheck() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.proxies) == 0 {
		return fmt.Errorf("中继组 %s 无可用代理", g.name)
	}

	for i, p := range g.proxies {
		if p == nil {
			return fmt.Errorf("中继组 %s 第 %d 个代理为 nil", g.name, i)
		}
	}

	return nil
}

// Proxies 返回代理列表
func (g *RelayGroup) Proxies() []adapter.Adapter {
	return g.proxies
}

// fnv32 计算 FNV-1a 32位哈希
func fnv32(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h
}

// Manager 代理组管理器
type Manager struct {
	groups map[string]Group
	mu     sync.RWMutex
}

// NewManager 创建代理组管理器
func NewManager(adapterMgr *adapter.Manager) *Manager {
	return &Manager{
		groups: make(map[string]Group),
	}
}

// Add 添加代理组
func (m *Manager) Add(g Group) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[g.Name()] = g
}

// Get 获取代理组
func (m *Manager) Get(name string) Group {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.groups[name]
}

// All 获取所有代理组
func (m *Manager) All() map[string]Group {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]Group)
	for k, v := range m.groups {
		result[k] = v
	}
	return result
}

// Remove 移除代理组
func (m *Manager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, name)
}

// Select 从指定组中选择代理
func (m *Manager) Select(groupName string, metadata *adapter.Metadata) adapter.Adapter {
	m.mu.RLock()
	g, ok := m.groups[groupName]
	m.mu.RUnlock()

	if !ok {
		return nil
	}
	return g.Select(metadata)
}

// HealthCheckAll 并发对所有代理组执行健康检查
func (m *Manager) HealthCheckAll() map[string]error {
	m.mu.RLock()
	groups := make(map[string]Group, len(m.groups))
	for k, v := range m.groups {
		groups[k] = v
	}
	m.mu.RUnlock()

	results := make(map[string]error, len(groups))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, g := range groups {
		wg.Add(1)
		go func(n string, gr Group) {
			defer wg.Done()
			err := gr.HealthCheck()
			mu.Lock()
			results[n] = err
			mu.Unlock()
		}(name, g)
	}

	wg.Wait()
	return results
}

// ParseGroupConfig 解析代理组配置
func ParseGroupConfig(cfg interface{}, adapterMgr *adapter.Manager) (Group, error) {
	// 简化实现
	return nil, fmt.Errorf("未实现")
}
