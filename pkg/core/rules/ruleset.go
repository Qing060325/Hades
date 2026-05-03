// Package rules 规则集实现
package rules

import (
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// ConcreteRuleSet 具体规则集实现
type ConcreteRuleSet struct {
	name      string
	rules     []Rule
	behavior  string // domain / ipcidr / classical
	updatedAt time.Time
	mu        sync.RWMutex
}

// NewConcreteRuleSet 创建规则集
func NewConcreteRuleSet(name string, behavior string) *ConcreteRuleSet {
	return &ConcreteRuleSet{
		name:      name,
		rules:     make([]Rule, 0),
		behavior:  behavior,
		updatedAt: time.Now(),
	}
}

// Name 返回规则集名称
func (rs *ConcreteRuleSet) Name() string {
	return rs.name
}

// Rules 返回规则列表
func (rs *ConcreteRuleSet) Rules() []Rule {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.rules
}

// Match 匹配规则集中的任意规则
func (rs *ConcreteRuleSet) Match(metadata *adapter.Metadata) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for _, rule := range rs.rules {
		if rule.Match(metadata) {
			return true
		}
	}
	return false
}

// Count 返回规则数量
func (rs *ConcreteRuleSet) Count() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.rules)
}

// UpdatedAt 返回最后更新时间
func (rs *ConcreteRuleSet) UpdatedAt() time.Time {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.updatedAt
}

// SetRules 设置规则列表
func (rs *ConcreteRuleSet) SetRules(rules []Rule) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.rules = rules
	rs.updatedAt = time.Now()
}

// AddRule 添加单条规则
func (rs *ConcreteRuleSet) AddRule(rule Rule) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.rules = append(rs.rules, rule)
	rs.updatedAt = time.Now()
}

// Behavior 返回规则集行为类型
func (rs *ConcreteRuleSet) Behavior() string {
	return rs.behavior
}
