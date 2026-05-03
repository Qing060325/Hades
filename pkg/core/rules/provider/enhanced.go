// Package provider 增强型规则集提供者
package provider

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/rules"
)

// EnhancedProvider 增强型规则集提供者
type EnhancedProvider struct {
	name      string
	behavior  Behavior
	sourceURL string
	filePath  string

	rules     []rules.Rule
	updatedAt time.Time
	mu        sync.RWMutex
	stopCh    chan struct{}
	running   bool
}

// NewEnhancedProvider 创建增强型提供者
func NewEnhancedProvider(name string, behavior Behavior) *EnhancedProvider {
	return &EnhancedProvider{
		name:     name,
		behavior: behavior,
		rules:    make([]rules.Rule, 0),
		stopCh:   make(chan struct{}),
	}
}

// LoadFromURL 从远程 URL 加载规则
func (ep *EnhancedProvider) LoadFromURL(url string) error {
	ep.sourceURL = url

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download rules from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download rules from %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	return ep.parseAndStore(data)
}

// LoadFromFile 从本地文件加载规则
func (ep *EnhancedProvider) LoadFromFile(path string) error {
	ep.filePath = path

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read rules file %s: %w", path, err)
	}

	return ep.parseAndStore(data)
}

// LoadFromMRS 从 MRS 格式加载规则
// MRS 格式为每行一条规则，支持注释（# 或 // 开头）
func (ep *EnhancedProvider) LoadFromMRS(path string) error {
	ep.filePath = path

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open MRS file %s: %w", path, err)
	}
	defer file.Close()

	parsed := make([]rules.Rule, 0)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		rule := ep.parseMRSEntry(line)
		if rule != nil {
			parsed = append(parsed, rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan MRS file: %w", err)
	}

	ep.mu.Lock()
	ep.rules = parsed
	ep.updatedAt = time.Now()
	ep.mu.Unlock()

	return nil
}

// parseAndStore 解析规则数据并存储
func (ep *EnhancedProvider) parseAndStore(data []byte) error {
	lines := strings.Split(string(data), "\n")
	parsed := make([]rules.Rule, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		rule := ep.parseRule(line)
		if rule != nil {
			parsed = append(parsed, rule)
		}
	}

	ep.mu.Lock()
	ep.rules = parsed
	ep.updatedAt = time.Now()
	ep.mu.Unlock()

	return nil
}

// parseRule 根据 behavior 解析单条规则
func (ep *EnhancedProvider) parseRule(line string) rules.Rule {
	switch ep.behavior {
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
		rule, _ := rules.ParseRule(line)
		return rule
	}
	return nil
}

// parseMRSEntry 解析 MRS 格式条目
func (ep *EnhancedProvider) parseMRSEntry(line string) rules.Rule {
	// MRS 格式支持 TYPE,PAYLOAD,ADAPTER 或简写格式
	if strings.Contains(line, ",") {
		rule, _ := rules.ParseRule(line)
		return rule
	}

	// 简写格式，根据 behavior 解析
	return ep.parseRule(line)
}

// AutoRefresh 后台自动刷新规则
func (ep *EnhancedProvider) AutoRefresh(interval time.Duration) {
	ep.mu.Lock()
	if ep.running {
		ep.mu.Unlock()
		return
	}
	ep.running = true
	ep.stopCh = make(chan struct{})
	ep.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ep.refresh()
			case <-ep.stopCh:
				return
			}
		}
	}()
}

// refresh 根据配置的源刷新规则
func (ep *EnhancedProvider) refresh() {
	ep.mu.RLock()
	url := ep.sourceURL
	path := ep.filePath
	ep.mu.RUnlock()

	if url != "" {
		if err := ep.LoadFromURL(url); err != nil {
			fmt.Printf("[EnhancedProvider] refresh from URL failed: %v\n", err)
		}
	} else if path != "" {
		if err := ep.LoadFromFile(path); err != nil {
			fmt.Printf("[EnhancedProvider] refresh from file failed: %v\n", err)
		}
	}
}

// Stop 停止自动刷新
func (ep *EnhancedProvider) Stop() {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if ep.running {
		close(ep.stopCh)
		ep.running = false
	}
}

// Rules 返回当前规则列表
func (ep *EnhancedProvider) Rules() []rules.Rule {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.rules
}

// Name 返回提供者名称
func (ep *EnhancedProvider) Name() string {
	return ep.name
}

// Count 返回规则数量
func (ep *EnhancedProvider) Count() int {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return len(ep.rules)
}

// UpdatedAt 返回最后更新时间
func (ep *EnhancedProvider) UpdatedAt() time.Time {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.updatedAt
}

// Stats 返回提供者统计信息
func (ep *EnhancedProvider) Stats() ProviderStats {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ProviderStats{
		Type:      "enhanced",
		Behavior:  string(ep.behavior),
		Count:     len(ep.rules),
		UpdatedAt: ep.updatedAt.Format(time.RFC3339),
	}
}
