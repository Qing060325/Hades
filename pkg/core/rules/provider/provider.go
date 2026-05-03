// Package provider 规则集提供者
package provider

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/rules"
)

// ProviderType Provider 类型
type ProviderType string

const (
	ProviderHTTP ProviderType = "http"
	ProviderFile ProviderType = "file"
)

// Behavior 规则行为
type Behavior string

const (
	BehaviorDomain    Behavior = "domain"
	BehaviorIPCIDR    Behavior = "ipcidr"
	BehaviorClassical Behavior = "classical"
)

// Provider 规则集 Provider
type Provider struct {
	Name     string       `yaml:"name"`
	Type     ProviderType `yaml:"type"`
	Behavior Behavior     `yaml:"behavior"`
	URL      string       `yaml:"url,omitempty"`
	Path     string       `yaml:"path,omitempty"`
	Interval int          `yaml:"interval,omitempty"`

	rules     []rules.Rule
	updatedAt time.Time
	mu        sync.RWMutex
	stopCh    chan struct{}
	started   bool
}

// New 创建 Provider
func New(name string, pType ProviderType, behavior Behavior, url, path string, interval int) *Provider {
	return &Provider{
		Name:     name,
		Type:     pType,
		Behavior: behavior,
		URL:      url,
		Path:     path,
		Interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Load 加载规则
func (p *Provider) Load(dataDir string) error {
	var data []byte
	var err error

	switch p.Type {
	case ProviderHTTP:
		data, err = p.download(dataDir)
	case ProviderFile:
		data, err = os.ReadFile(p.Path)
	default:
		return fmt.Errorf("unknown provider type: %s", p.Type)
	}

	if err != nil {
		return err
	}

	parsed, err := p.parse(data)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.rules = parsed
	p.updatedAt = time.Now()
	p.mu.Unlock()

	return nil
}

// Rules 获取规则列表
func (p *Provider) Rules() []rules.Rule {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rules
}

// Stats 获取统计
func (p *Provider) Stats() ProviderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return ProviderStats{
		Type:      string(p.Type),
		Behavior:  string(p.Behavior),
		Count:     len(p.rules),
		UpdatedAt: p.updatedAt.Format(time.RFC3339),
	}
}

// ProviderStats Provider 统计
type ProviderStats struct {
	Type      string `json:"type"`
	Behavior  string `json:"behavior"`
	Count     int    `json:"count"`
	UpdatedAt string `json:"updatedAt"`
}

func (p *Provider) download(dataDir string) ([]byte, error) {
	resp, err := http.Get(p.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download provider %s: HTTP %d", p.Name, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (p *Provider) parse(data []byte) ([]rules.Rule, error) {
	lines := strings.Split(string(data), "\n")
	result := make([]rules.Rule, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		rule := p.parseRule(line)
		if rule != nil {
			result = append(result, rule)
		}
	}

	return result, nil
}

func (p *Provider) parseRule(line string) rules.Rule {
	switch p.Behavior {
	case BehaviorDomain:
		if strings.HasPrefix(line, "full:") {
			return rules.NewDomainRule(strings.TrimPrefix(line, "full:"), "")
		}
		if strings.HasPrefix(line, ".") {
			return rules.NewDomainSuffixRule(line[1:], "")
		}
		return rules.NewDomainSuffixRule(line, "")
	case BehaviorIPCIDR:
		return rules.NewIPCIDRRule(line, "", false)
	case BehaviorClassical:
		parts := strings.SplitN(line, ",", 3)
		if len(parts) >= 3 {
			rule, _ := rules.ParseRule(line)
			return rule
		}
	}
	return nil
}

// AutoUpdate 自动更新
func (p *Provider) AutoUpdate(dataDir string) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()

	ticker := time.NewTicker(time.Duration(p.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.Load(dataDir)
		case <-p.stopCh:
			return
		}
	}
}

// Stop 停止
func (p *Provider) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		p.started = false
		close(p.stopCh)
	}
}
