package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Qing060325/Hades/internal/config"
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

	cfg := config.RuleProviderConfig{
		Type:     "file",
		Behavior: "classical",
		Path:     ruleFile,
	}

	p := New("test-provider", cfg)
	if err := p.Load(); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if p.Count() != 5 {
		t.Errorf("Count() = %d, want 5", p.Count())
	}

	if p.Name() != "test-provider" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test-provider")
	}

	if p.Type() != "file" {
		t.Errorf("Type() = %q, want %q", p.Type(), "file")
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

	cfg := config.RuleProviderConfig{
		Type:     "file",
		Behavior: "domain",
		Path:     ruleFile,
	}

	p := New("domain-provider", cfg)
	if err := p.Load(); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if p.Count() != 3 {
		t.Errorf("Count() = %d, want 3", p.Count())
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

	cfg := config.RuleProviderConfig{
		Type:     "file",
		Behavior: "ipcidr",
		Path:     ruleFile,
	}

	p := New("cidr-provider", cfg)
	if err := p.Load(); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if p.Count() != 4 {
		t.Errorf("Count() = %d, want 4", p.Count())
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

	cfg := config.RuleProviderConfig{
		Type:     "file",
		Behavior: "classical",
		Format:   "yaml",
		Path:     ruleFile,
	}

	p := New("yaml-provider", cfg)
	if err := p.Load(); err != nil {
		t.Fatalf("加载规则失败: %v", err)
	}

	if p.Count() != 3 {
		t.Errorf("Count() = %d, want 3", p.Count())
	}
}

// TestManager_CreateAndLoad 测试管理器创建和加载
func TestManager_CreateAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建多个规则文件
	ruleFile1 := filepath.Join(tmpDir, "rules1.txt")
	ruleFile2 := filepath.Join(tmpDir, "rules2.txt")

	os.WriteFile(ruleFile1, []byte("DOMAIN,test1.com,DIRECT\nDOMAIN,test2.com,proxy\n"), 0644)
	// ipcidr 行为模式：每行只是一个 CIDR，不是完整规则行
	os.WriteFile(ruleFile2, []byte("10.0.0.0/8\n172.16.0.0/12\n"), 0644)

	cfgs := map[string]config.RuleProviderConfig{
		"provider1": {Type: "file", Behavior: "classical", Path: ruleFile1},
		"provider2": {Type: "file", Behavior: "ipcidr", Path: ruleFile2},
	}

	mgr := NewManager()
	if err := mgr.CreateFromConfig(cfgs); err != nil {
		t.Fatalf("CreateFromConfig 失败: %v", err)
	}

	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll 失败: %v", err)
	}

	// 验证规则收集
	allRules := mgr.CollectRules()
	if len(allRules) != 4 {
		t.Errorf("CollectRules() count = %d, want 4", len(allRules))
	}

	// 验证统计
	stats := mgr.Stats()
	if len(stats) != 2 {
		t.Errorf("Stats() count = %d, want 2", len(stats))
	}

	// 验证重载
	if err := mgr.Reload("provider1"); err != nil {
		t.Errorf("Reload 失败: %v", err)
	}

	if err := mgr.ReloadAll(); err != nil {
		t.Errorf("ReloadAll 失败: %v", err)
	}

	// 验证不存在的提供者
	if err := mgr.Reload("nonexistent"); err == nil {
		t.Error("期望 Reload 不存在的提供者返回错误")
	}
}
