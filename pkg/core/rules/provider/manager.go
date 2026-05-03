// Package provider 规则集提供者管理器
package provider

import (
	"fmt"
	"sync"

	"github.com/Qing060325/Hades/pkg/core/rules"
)

// Manager 规则集提供者管理器
type Manager struct {
	providers map[string]*Provider
	mu        sync.RWMutex
	dataDir   string
}

// NewManager 创建管理器
func NewManager(dataDir string) *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
		dataDir:   dataDir,
	}
}

// Add 添加 Provider
func (m *Manager) Add(p *Provider) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[p.Name]; exists {
		return fmt.Errorf("provider %s already exists", p.Name)
	}

	if err := p.Load(m.dataDir); err != nil {
		return err
	}

	m.providers[p.Name] = p

	if p.Type == ProviderHTTP && p.Interval > 0 {
		go p.AutoUpdate(m.dataDir)
	}

	return nil
}

// Get 获取 Provider
func (m *Manager) Get(name string) (*Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	return p, ok
}

// Reload 重新加载指定 Provider
func (m *Manager) Reload(name string) error {
	m.mu.RLock()
	p, ok := m.providers[name]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("provider %s not found", name)
	}
	return p.Load(m.dataDir)
}

// ReloadAll 重新加载所有 Provider
func (m *Manager) ReloadAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.providers {
		if err := p.Load(m.dataDir); err != nil {
			return err
		}
	}
	return nil
}

// Stats 获取所有 Provider 统计
func (m *Manager) Stats() map[string]ProviderStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ProviderStats)
	for name, p := range m.providers {
		result[name] = p.Stats()
	}
	return result
}

// Rules 获取指定 Provider 的规则
func (m *Manager) Rules(name string) []rules.Rule {
	m.mu.RLock()
	p, ok := m.providers[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return p.Rules()
}

// MatchAll 匹配所有 Provider
func (m *Manager) MatchAll(host string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.providers {
		for _, rule := range p.Rules() {
			if rule.Match(nil) {
				return rule.Adapter(), true
			}
		}
	}
	return "", false
}

// StopAll 停止所有 Provider
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.providers {
		p.Stop()
	}
}

// StartAll 启动所有 Provider 的自动更新
func (m *Manager) StartAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.providers {
		if p.Type == ProviderHTTP && p.Interval > 0 {
			go p.AutoUpdate(m.dataDir)
		}
	}
}
