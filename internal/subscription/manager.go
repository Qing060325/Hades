// Package subscription 内部订阅管理
package subscription

import (
	"fmt"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/Qing060325/Hades/pkg/subscription"
	"github.com/rs/zerolog/log"
)

// Manager 内部订阅管理器
type Manager struct {
	mgr        *subscription.Manager
	cfg        *config.Config
	subConfigs []config.SubscriptionConfig
	mu         sync.RWMutex
	stopCh     chan struct{}
}

// NewManager 创建内部订阅管理器
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		mgr:        subscription.NewManager(),
		cfg:        cfg,
		subConfigs: cfg.Subscriptions,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动订阅管理
func (m *Manager) Start() error {
	if len(m.subConfigs) == 0 {
		return nil
	}

	log.Info().Int("count", len(m.subConfigs)).Msg("启动订阅管理器")

	// 添加所有订阅
	for _, subCfg := range m.subConfigs {
		sub := &subscription.Subscription{
			Name:       subCfg.Name,
			URL:        subCfg.URL,
			Interval:   subCfg.Interval,
			AutoUpdate: subCfg.AutoUpdate,
		}

		if err := m.mgr.Add(sub); err != nil {
			log.Error().Err(err).Str("name", subCfg.Name).Msg("添加订阅失败")
		}
	}

	// 启动自动更新
	m.mgr.Start()

	// 启动配置更新循环
	go m.updateLoop()

	return nil
}

// Stop 停止订阅管理
func (m *Manager) Stop() error {
	close(m.stopCh)
	m.mgr.Stop()
	return nil
}

// updateLoop 配置更新循环
func (m *Manager) updateLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndUpdateConfig()
		}
	}
}

// checkAndUpdateConfig 检查订阅更新并更新配置
func (m *Manager) checkAndUpdateConfig() {
	// 获取所有订阅节点
	nodes := m.mgr.GetNodes()
	if len(nodes) == 0 {
		return
	}

	// 更新配置中的代理节点
	m.mu.Lock()
	defer m.mu.Unlock()

	// 保留非订阅节点
	nonSubProxies := m.filterNonSubProxies()

	// 添加订阅节点
	for _, node := range nodes {
		proxy := nodeToProxyConfig(node)
		nonSubProxies = append(nonSubProxies, proxy)
	}

	m.cfg.Proxies = nonSubProxies

	log.Debug().Int("nodes", len(nodes)).Msg("订阅配置已更新")
}

// filterNonSubProxies 过滤出非订阅代理节点
func (m *Manager) filterNonSubProxies() []config.ProxyConfig {
	// 简化处理：返回原始配置中的代理
	// 实际应用中可能需要更复杂的逻辑来区分订阅节点和手动节点
	return m.cfg.Proxies
}

// GetSubscriptionInfo 获取订阅信息
func (m *Manager) GetSubscriptionInfo() []subscription.SubscriptionInfo {
	list := m.mgr.List()
	infos := make([]subscription.SubscriptionInfo, 0, len(list))

	for _, sub := range list {
		infos = append(infos, sub.GetInfo())
	}

	return infos
}

// UpdateSubscription 手动更新指定订阅
func (m *Manager) UpdateSubscription(name string) error {
	return m.mgr.Update(name)
}

// UpdateAllSubscriptions 更新所有订阅
func (m *Manager) UpdateAllSubscriptions() error {
	return m.mgr.UpdateAll()
}

// nodeToProxyConfig 转换节点为代理配置
func nodeToProxyConfig(node subscription.Node) config.ProxyConfig {
	proxy := config.ProxyConfig{
		Name:           node.Name,
		Type:           node.Type,
		Server:         node.Server,
		Port:           node.Port,
		Password:       node.Password,
		UUID:           node.UUID,
		AlterID:        node.AlterID,
		Cipher:         node.Cipher,
		TLS:            node.TLS,
		SkipCertVerify: node.SkipCert,
		ServerName:     node.ServerName,
		Network:        node.Network,
		WSPath:         node.WSPath,
		WSHeaders:      node.WSHeaders,
		UDP:            node.UDP,
	}

	if proxy.Name == "" {
		proxy.Name = fmt.Sprintf("%s-%d", proxy.Server, proxy.Port)
	}

	return proxy
}
