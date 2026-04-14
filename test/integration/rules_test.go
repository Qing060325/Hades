package integration

import (
	"testing"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/rules"
)

// TestRuleEngine_IPCIDRMatch 测试 IP-CIDR 规则匹配
func TestRuleEngine_IPCIDRMatch(t *testing.T) {
	ruleStrs := []string{
		"IP-CIDR,192.168.0.0/16,DIRECT",
		"IP-CIDR,10.0.0.0/8,DIRECT",
		"IP-CIDR,172.16.0.0/12,REJECT",
		"DOMAIN-SUFFIX,google.com,proxy",
		"MATCH,DIRECT",
	}

	engine := rules.NewEngine(ruleStrs)

	tests := []struct {
		name     string
		metadata *adapter.Metadata
		expected string
	}{
		{
			name: "192.168.x.x 匹配 DIRECT",
			metadata: &adapter.Metadata{
				Host:   "192.168.1.1",
				DstIP:  mustParseAddr("192.168.1.1"),
				DstPort: 443,
			},
			expected: "DIRECT",
		},
		{
			name: "10.x.x.x 匹配 DIRECT",
			metadata: &adapter.Metadata{
				Host:   "10.0.0.1",
				DstIP:  mustParseAddr("10.0.0.1"),
				DstPort: 80,
			},
			expected: "DIRECT",
		},
		{
			name: "172.16.x.x 匹配 REJECT",
			metadata: &adapter.Metadata{
				Host:   "172.16.0.1",
				DstIP:  mustParseAddr("172.16.0.1"),
				DstPort: 8080,
			},
			expected: "REJECT",
		},
		{
			name: "google.com 匹配 proxy",
			metadata: &adapter.Metadata{
				Host:   "www.google.com",
				DstPort: 443,
			},
			expected: "proxy",
		},
		{
			name: "未匹配规则走 MATCH",
			metadata: &adapter.Metadata{
				Host:   "example.org",
				DstPort: 443,
			},
			expected: "DIRECT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.Match(tc.metadata)
			if result != tc.expected {
				t.Errorf("规则匹配失败: got %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestRuleEngine_DomainRules 测试域名规则匹配
func TestRuleEngine_DomainRules(t *testing.T) {
	ruleStrs := []string{
		"DOMAIN,example.com,direct",
		"DOMAIN-SUFFIX,google.com,proxy",
		"DOMAIN-KEYWORD,github,proxy",
		"MATCH,DIRECT",
	}

	engine := rules.NewEngine(ruleStrs)

	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{"精确域名匹配", "example.com", "direct"},
		{"域名后缀匹配", "www.google.com", "proxy"},
		{"域名后缀自身匹配", "google.com", "proxy"},
		{"域名关键字匹配", "github.com", "proxy"},
		{"域名关键字匹配2", "gist.github.com", "proxy"},
		{"不匹配走MATCH", "other.com", "DIRECT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &adapter.Metadata{Host: tc.host}
			result := engine.Match(metadata)
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestRuleEngine_ProcessName 测试进程名规则匹配
func TestRuleEngine_ProcessName(t *testing.T) {
	ruleStrs := []string{
		"PROCESS-NAME,curl,direct",
		"PROCESS-NAME,wget,direct",
		"MATCH,proxy",
	}

	engine := rules.NewEngine(ruleStrs)

	tests := []struct {
		name     string
		process  string
		expected string
	}{
		{"curl 进程", "curl", "direct"},
		{"wget 进程", "wget", "direct"},
		{"其他进程", "chrome", "proxy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &adapter.Metadata{Process: tc.process}
			result := engine.Match(metadata)
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}
