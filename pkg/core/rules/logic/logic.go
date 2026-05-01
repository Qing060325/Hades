// Package logic 逻辑规则 (AND/OR/NOT)
package logic

import (
	"strings"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/rules"
)

// LogicalRuleType 逻辑规则类型
type LogicalRuleType string

const (
	LogicalAND LogicalRuleType = "AND"
	LogicalOR  LogicalRuleType = "OR"
	LogicalNOT LogicalRuleType = "NOT"
)

// LogicalRule 逻辑规则
type LogicalRule struct {
	rules.BaseRule
	logicType LogicalRuleType
	subRules  []rules.Rule
}

// NewLogicalRule 创建逻辑规则
func NewLogicalRule(logicType LogicalRuleType, subRules []rules.Rule, adapterName string) *LogicalRule {
	return &LogicalRule{
		BaseRule:  rules.BaseRule{},
		logicType: logicType,
		subRules:  subRules,
	}
}

// Match 匹配
func (r *LogicalRule) Match(metadata *adapter.Metadata) bool {
	switch r.logicType {
	case LogicalAND:
		for _, rule := range r.subRules {
			if !rule.Match(metadata) {
				return false
			}
		}
		return true
	case LogicalOR:
		for _, rule := range r.subRules {
			if rule.Match(metadata) {
				return true
			}
		}
		return false
	case LogicalNOT:
		if len(r.subRules) > 0 {
			return !r.subRules[0].Match(metadata)
		}
		return false
	}
	return false
}

// Type 返回类型
func (r *LogicalRule) Type() rules.RuleType {
	return rules.RuleType("LOGICAL-" + string(r.logicType))
}

// Adapter 返回适配器
func (r *LogicalRule) Adapter() string {
	return r.BaseRule.Adapter()
}

// Payload 返回负载
func (r *LogicalRule) Payload() string {
	return string(r.logicType)
}

// ParseLogicalRule 解析逻辑规则
// 格式: AND,((RULE1),(RULE2)),ADAPTER
func ParseLogicalRule(line string, ruleParser func(string) (rules.Rule, error)) (rules.Rule, error) {
	parts := strings.SplitN(line, ",", 3)
	if len(parts) < 3 {
		return nil, nil
	}

	logicType := LogicalRuleType(strings.TrimSpace(parts[0]))
	if logicType != LogicalAND && logicType != LogicalOR && logicType != LogicalNOT {
		return nil, nil
	}

	// 解析子规则
	ruleBody := strings.TrimSpace(parts[1])
	adapterName := strings.TrimSpace(parts[2])

	// 去掉外层括号
	if strings.HasPrefix(ruleBody, "(") && strings.HasSuffix(ruleBody, ")") {
		ruleBody = ruleBody[1 : len(ruleBody)-1]
	}

	subRuleStrs := splitLogicalRules(ruleBody)
	subRules := make([]rules.Rule, 0, len(subRuleStrs))

	for _, subRuleStr := range subRuleStrs {
		subRuleStr = strings.TrimSpace(subRuleStr)
		if subRuleStr == "" {
			continue
		}
		// 去掉括号
		if strings.HasPrefix(subRuleStr, "(") && strings.HasSuffix(subRuleStr, ")") {
			subRuleStr = subRuleStr[1 : len(subRuleStr)-1]
		}

		rule, err := ruleParser(subRuleStr)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			subRules = append(subRules, rule)
		}
	}

	return NewLogicalRule(logicType, subRules, adapterName), nil
}

func splitLogicalRules(s string) []string {
	var result []string
	depth := 0
	start := 0

	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}

	return result
}
