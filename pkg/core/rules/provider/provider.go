// Package provider 规则集动态热更新
//
// Rule Provider 从外部来源（HTTP/文件）动态获取规则列表，
// 并按配置的间隔定期刷新，无需重启内核即可更新规则。
//
// 支持的来源类型：
//   - http: 从远程 URL 拉取规则文件
//   - file: 从本地文件读取规则
//
// 支持的行为模式：
//   - domain: 纯域名规则列表
//   - ipcidr: 纯 IP-CIDR 规则列表
//   - classical: 经典格式（完整规则行，如 DOMAIN,example.com,DIRECT）
//
// 支持的文件格式：
//   - yaml: Clash Meta 格式
//   - text: 纯文本（每行一条规则载荷）
package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/rs/zerolog/log"
)

// Provider 规则集提供者
type Provider struct {
	name     string
	cfg      config.RuleProviderConfig
	rules    []rules.Rule
	mu       sync.RWMutex
	updatedAt time.Time
	count    int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New 创建规则集提供者
func New(name string, cfg config.RuleProviderConfig) *Provider {
	ctx, cancel := context.WithCancel(context.Background())
	return &Provider{
		name:   name,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Name 返回提供者名称
func (p *Provider) Name() string {
	return p.name
}

// Rules 返回当前规则列表
func (p *Provider) Rules() []rules.Rule {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]rules.Rule, len(p.rules))
	copy(result, p.rules)
	return result
}

// Count 返回规则数量
func (p *Provider) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.count
}

// UpdatedAt 返回最后更新时间
func (p *Provider) UpdatedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.updatedAt
}

// Type 返回来源类型
func (p *Provider) Type() string {
	return p.cfg.Type
}

// Behavior 返回行为模式
func (p *Provider) Behavior() string {
	return p.cfg.Behavior
}

// Load 初始加载规则
func (p *Provider) Load() error {
	data, err := p.fetch()
	if err != nil {
		return fmt.Errorf("加载规则集 %s 失败: %w", p.name, err)
	}

	parsed, err := p.parse(data)
	if err != nil {
		return fmt.Errorf("解析规则集 %s 失败: %w", p.name, err)
	}

	p.mu.Lock()
	p.rules = parsed
	p.count = len(parsed)
	p.updatedAt = time.Now()
	p.mu.Unlock()

	log.Info().
		Str("name", p.name).
		Str("type", p.cfg.Type).
		Str("behavior", p.cfg.Behavior).
		Int("count", len(parsed)).
		Msg("[RuleProvider] 规则集已加载")

	return nil
}

// StartAutoRefresh 启动自动刷新
func (p *Provider) StartAutoRefresh() {
	interval := time.Duration(p.cfg.Interval) * time.Second
	if interval < 60*time.Second {
		interval = 60 * time.Second
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Info().
			Str("name", p.name).
			Str("interval", interval.String()).
			Msg("[RuleProvider] 自动刷新已启动")

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				if err := p.Load(); err != nil {
					log.Warn().
						Str("name", p.name).
						Err(err).
						Msg("[RuleProvider] 自动刷新失败")
				}
			}
		}
	}()
}

// Stop 停止提供者
func (p *Provider) Stop() {
	p.cancel()
	p.wg.Wait()
}

// Match 检查元数据是否匹配此提供者的任何规则
func (p *Provider) Match(metadata *adapter.Metadata) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, rule := range p.rules {
		if rule.Match(metadata) {
			return true
		}
	}
	return false
}

// fetch 从来源获取原始数据
func (p *Provider) fetch() ([]byte, error) {
	switch p.cfg.Type {
	case "http":
		return p.fetchHTTP()
	case "file":
		return p.fetchFile()
	default:
		return nil, fmt.Errorf("不支持的来源类型: %s", p.cfg.Type)
	}
}

// fetchHTTP 从远程 URL 拉取
func (p *Provider) fetchHTTP() ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(p.ctx, "GET", p.cfg.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 缓存到本地文件
	if p.cfg.Path != "" {
		if err := os.WriteFile(p.cfg.Path, data, 0644); err != nil {
			log.Warn().Err(err).Str("path", p.cfg.Path).Msg("[RuleProvider] 缓存写入失败")
		}
	}

	return data, nil
}

// fetchFile 从本地文件读取
func (p *Provider) fetchFile() ([]byte, error) {
	if p.cfg.Path == "" {
		return nil, fmt.Errorf("文件路径为空")
	}

	data, err := os.ReadFile(p.cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	return data, nil
}

// parse 解析原始数据为规则列表
func (p *Provider) parse(data []byte) ([]rules.Rule, error) {
	switch p.cfg.Format {
	case "yaml":
		return p.parseYAML(data)
	case "text", "":
		return p.parseText(data)
	default:
		return p.parseText(data) // 默认尝试文本格式
	}
}

// parseText 解析纯文本格式
func (p *Provider) parseText(data []byte) ([]rules.Rule, error) {
	lines := strings.Split(string(data), "\n")
	var result []rules.Rule

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule, err := p.parseLine(line)
		if err != nil {
			log.Debug().Err(err).Str("line", line).Msg("[RuleProvider] 解析规则行失败")
			continue
		}
		if rule != nil {
			result = append(result, rule)
		}
	}

	return result, nil
}

// parseLine 解析单行规则
func (p *Provider) parseLine(line string) (rules.Rule, error) {
	switch p.cfg.Behavior {
	case "domain":
		// 每行是一个域名，生成 DOMAIN 规则
		return rules.ParseRule(fmt.Sprintf("DOMAIN,%s,%s", line, p.name))
	case "ipcidr":
		// 每行是一个 CIDR，生成 IP-CIDR 规则
		if strings.Contains(line, ":") {
			return rules.ParseRule(fmt.Sprintf("IP-CIDR6,%s,%s", line, p.name))
		}
		return rules.ParseRule(fmt.Sprintf("IP-CIDR,%s,%s", line, p.name))
	case "classical", "":
		// 完整规则行格式
		return rules.ParseRule(line)
	default:
		return rules.ParseRule(line)
	}
}

// parseYAML 解析 YAML 格式
func (p *Provider) parseYAML(data []byte) ([]rules.Rule, error) {
	// YAML 格式解析（简化版）
	// 预期格式：
	// payload:
	//   - DOMAIN,example.com,DIRECT
	//   - IP-CIDR,10.0.0.0/8,DIRECT
	content := string(data)
	var inPayload bool
	var result []rules.Rule

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "payload:") {
			inPayload = true
			continue
		}

		if inPayload {
			// 去除 YAML 列表前缀 "- "
			ruleStr := strings.TrimPrefix(trimmed, "- ")
			ruleStr = strings.TrimSpace(ruleStr)
			if ruleStr == "" {
				continue
			}

			rule, err := p.parseLine(ruleStr)
			if err != nil {
				log.Debug().Err(err).Str("line", ruleStr).Msg("[RuleProvider] 解析规则行失败")
				continue
			}
			if rule != nil {
				result = append(result, rule)
			}
		}
	}

	return result, nil
}
