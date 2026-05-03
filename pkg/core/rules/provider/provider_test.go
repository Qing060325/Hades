package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProvider_FileLoad 测试从文件加载规则
func TestProvider_FileLoad(t *testing.T) {
	// 创建临时规则文件
	tmpDir := t.TempDir()
	ruleFile := filepath.Join(tmpDir, "test-rules.txt")

	rules := `DOMAIN,example.com,DIRECT
DOMAIN-SUFFIX,google.com,proxy
IP-CIDR,10.0.0.0/8,DIRECT
IP-CIDR6,fd00::/8,DIRECT
# 注释行应该被忽略

DOMAIN-KEYWORD,github,proxy
`

	if err := os.WriteFile(ruleFile, []byte(rules), 0644); err != nil {
		t.Fatalf("创建规则文件失败: %v", err)
	}

	p := New("test-provider", ProviderFile, BehaviorClassical, "", ruleFile, 0)
	if err := p.Load(""); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if len(p.Rules()) != 5 {
		t.Errorf("Rules() count = %d, want 5", len(p.Rules()))
	}

	if p.Name != "test-provider" {
		t.Errorf("Name = %q, want %q", p.Name, "test-provider")
	}

	if p.Type != ProviderFile {
		t.Errorf("Type = %q, want %q", p.Type, ProviderFile)
	}
}

// TestProvider_DomainBehavior 测试 domain 行为模式
func TestProvider_DomainBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	ruleFile := filepath.Join(tmpDir, "domains.txt")

	domains := `google.com
github.com
example.org
# 忽略注释
`

	if err := os.WriteFile(ruleFile, []byte(domains), 0644); err != nil {
		t.Fatalf("创建规则文件失败: %v", err)
	}

	p := New("domain-provider", ProviderFile, BehaviorDomain, "", ruleFile, 0)
	if err := p.Load(""); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if len(p.Rules()) != 3 {
		t.Errorf("Rules() count = %d, want 3", len(p.Rules()))
	}
}

// TestProvider_IPCIDRBehavior 测试 ipcidr 行为模式
func TestProvider_IPCIDRBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	ruleFile := filepath.Join(tmpDir, "cidrs.txt")

	cidrs := `10.0.0.0/8
172.16.0.0/12
192.168.0.0/16
::1/128
`

	if err := os.WriteFile(ruleFile, []byte(cidrs), 0644); err != nil {
		t.Fatalf("创建规则文件失败: %v", err)
	}

	p := New("cidr-provider", ProviderFile, BehaviorIPCIDR, "", ruleFile, 0)
	if err := p.Load(""); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if len(p.Rules()) != 4 {
		t.Errorf("Rules() count = %d, want 4", len(p.Rules()))
	}
}

// TestProvider_YAMLFormat 测试 YAML 格式解析
func TestProvider_YAMLFormat(t *testing.T) {
	tmpDir := t.TempDir()
	ruleFile := filepath.Join(tmpDir, "rules.yaml")

	yaml := `payload:
  - DOMAIN,example.com,DIRECT
  - DOMAIN-SUFFIX,google.com,proxy
  - IP-CIDR,10.0.0.0/8,DIRECT
`

	if err := os.WriteFile(ruleFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("创建规则文件失败: %v", err)
	}

	p := New("yaml-provider", ProviderFile, BehaviorClassical, "", ruleFile, 0)
	if err := p.Load(""); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if len(p.Rules()) != 3 {
		t.Errorf("Rules() count = %d, want 3", len(p.Rules()))
	}
}
