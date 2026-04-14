// Package provider 规则集提供者管理器
package provider

import (
	"fmt"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/rs/zerolog/log"
)

// Manager 规则集提供者管理器
type Manager struct {
	providers map[string]*Provider
	mu        sync.RWMutex
}

// NewManager 创建管理器
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
	}
}

// CreateFromConfig 从配置创建所有提供者
func (m *Manager) CreateFromConfig(cfgs map[string]config.RuleProviderConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, cfg := range cfgs {
		p := New(name, cfg)
		m.providers[name] = p
	}

	log.Info().Int("count", len(m.providers)).Msg("[RuleProvider] 管理器已创建")
	return nil
}

// LoadAll 加载所有提供者的规则
func (m *Manager) LoadAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, p := range m.providers {
		if err := p.Load(); err != nil {
			log.Warn().Str("name", name).Err(err).Msg("[RuleProvider] 加载失败")
			// 继续加载其他提供者
		}
	}

	return nil
}

// StartAll 启动所有提供者的自动刷新
func (m *Manager) StartAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.providers {
		p.StartAutoRefresh()
	}
}

// StopAll 停止所有提供者
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.providers {
		p.Stop()
	}
}

// Get 获取指定提供者
func (m *Manager) Get(name string) *Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[name]
}

// All 获取所有提供者
func (m *Manager) All() map[string]*Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Provider, len(m.providers))
	for k, v := range m.providers {
		result[k] = v
	}
	return result
}

// Reload 重新加载指定提供者
func (m *Manager) Reload(name string) error {
	m.mu.RLock()
	p, ok := m.providers[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("提供者 %s 不存在", name)
	}

	return p.Load()
}

// ReloadAll 重新加载所有提供者
func (m *Manager) ReloadAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, p := range m.providers {
		if err := p.Load(); err != nil {
			log.Warn().Str("name", name).Err(err).Msg("[RuleProvider] 重载失败")
		}
	}

	return nil
}

// CollectRules 收集所有提供者的规则
func (m *Manager) CollectRules() []rules.Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allRules []rules.Rule
	for _, p := range m.providers {
		allRules = append(allRules, p.Rules()...)
	}
	return allRules
}

// MatchAll 在所有提供者中匹配规则
// 返回匹配的适配器名称，空字符串表示未匹配
func (m *Manager) MatchAll(metadata *adapter.Metadata) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.providers {
		if p.Match(metadata) {
			// 返回提供者名称作为适配器名
			// 在 classical 模式下，规则本身指定了适配器
			// 这里简化处理
			return p.name
		}
	}
	return ""
}

// Stats 返回所有提供者的统计信息
func (m *Manager) Stats() map[string]ProviderStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ProviderStats, len(m.providers))
	for name, p := range m.providers {
		p.mu.RLock()
		result[name] = ProviderStats{
			Type:      p.cfg.Type,
			Behavior:  p.cfg.Behavior,
			Count:     p.count,
			UpdatedAt: p.updatedAt,
		}
		p.mu.RUnlock()
	}
	return result
}

// ProviderStats 提供者统计信息
type ProviderStats struct {
	Type      string
	Behavior  string
	Count     int
	UpdatedAt time.Time
}
