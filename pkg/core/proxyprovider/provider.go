// Package proxyprovider 远程代理提供者模块
package proxyprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/rs/zerolog/log"
)

// HealthCheckConfig 健康检查配置
type HealthCheckConfig struct {
	Enable   bool
	URL      string
	Interval time.Duration
	Timeout  time.Duration
}

// Provider 远程代理提供者
type Provider struct {
	name        string
	url         string
	path        string
	interval    time.Duration
	healthCheck HealthCheckConfig
	proxies     []config.ProxyConfig
	adapters    []adapter.Adapter
	lastUpdate  time.Time
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	client      *http.Client
	header      map[string]string
}

// NewProvider 创建代理提供者
func NewProvider(name, url, path string, interval time.Duration, hc HealthCheckConfig) *Provider {
	ctx, cancel := context.WithCancel(context.Background())
	return &Provider{
		name:        name,
		url:         url,
		path:        path,
		interval:    interval,
		healthCheck: hc,
		ctx:         ctx,
		cancel:      cancel,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		header: make(map[string]string),
	}
}

// NewProviderWithHeader 创建带自定义 Header 的代理提供者
func NewProviderWithHeader(name, url, path string, interval time.Duration, hc HealthCheckConfig, header map[string]string) *Provider {
	p := NewProvider(name, url, path, interval, hc)
	p.header = header
	return p
}

// Start 启动提供者 (首次拉取 + 定时刷新)
func (p *Provider) Start() error {
	// 首次拉取
	if err := p.Update(); err != nil {
		log.Warn().Err(err).Str("provider", p.name).Msg("首次拉取代理列表失败")
		// 不阻塞启动，后续定时重试
	}

	// 启动定时刷新
	go p.refreshLoop()

	// 启动健康检查
	if p.healthCheck.Enable {
		go p.healthCheckLoop()
	}

	return nil
}

// Stop 停止提供者
func (p *Provider) Stop() {
	p.cancel()
}

// refreshLoop 定时刷新代理列表
func (p *Provider) refreshLoop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if err := p.Update(); err != nil {
				log.Warn().Err(err).Str("provider", p.name).Msg("定时刷新代理列表失败")
			}
		}
	}
}

// healthCheckLoop 定时健康检查
func (p *Provider) healthCheckLoop() {
	ticker := time.NewTicker(p.healthCheck.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if err := p.HealthCheck(); err != nil {
				log.Warn().Err(err).Str("provider", p.name).Msg("健康检查失败")
			}
		}
	}
}

// Update 立即拉取并更新代理列表
func (p *Provider) Update() error {
	if p.url == "" {
		return fmt.Errorf("provider %s: url is empty", p.name)
	}

	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// 设置自定义 Header
	for k, v := range p.header {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "ClashForAndroid/2.5.12")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", p.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", p.url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	proxies, err := ParseSubscription(body)
	if err != nil {
		return fmt.Errorf("parse subscription: %w", err)
	}

	p.mu.Lock()
	p.proxies = proxies
	p.lastUpdate = time.Now()
	p.mu.Unlock()

	log.Info().
		Str("provider", p.name).
		Int("count", len(proxies)).
		Msg("代理列表已更新")

	return nil
}

// Proxies 获取当前代理列表
func (p *Provider) Proxies() []config.ProxyConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]config.ProxyConfig, len(p.proxies))
	copy(result, p.proxies)
	return result
}

// HealthCheck 执行健康检查
func (p *Provider) HealthCheck() error {
	if !p.healthCheck.Enable || p.healthCheck.URL == "" {
		return nil
	}

	timeout := p.healthCheck.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.healthCheck.URL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// LastUpdate 获取最后更新时间
func (p *Provider) LastUpdate() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastUpdate
}

// Name 获取提供者名称
func (p *Provider) Name() string {
	return p.name
}

// IsEmpty 代理列表是否为空
func (p *Provider) IsEmpty() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.proxies) == 0
}
