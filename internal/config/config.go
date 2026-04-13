// Package config 配置管理模块
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Parser 配置解析器
type Parser struct {
	path string
}

// NewParser 创建配置解析器
func NewParser(path string) *Parser {
	return &Parser{path: path}
}

// Parse 解析配置文件
func (p *Parser) Parse() (*Config, error) {
	// 读取配置文件
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 应用默认值
	p.applyDefaults(&cfg)

	// 验证配置
	if err := p.validate(&cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &cfg, nil
}

// applyDefaults 应用默认值
func (p *Parser) applyDefaults(cfg *Config) {
	defaults := Default()

	if cfg.MixedPort == 0 && cfg.Port == 0 && cfg.SocksPort == 0 {
		cfg.MixedPort = defaults.MixedPort
	}

	if cfg.Mode == "" {
		cfg.Mode = defaults.Mode
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = defaults.LogLevel
	}

	if cfg.BindAddress == "" {
		cfg.BindAddress = defaults.BindAddress
	}

	// TUN 默认值
	if cfg.Tun.MTU == 0 {
		cfg.Tun.MTU = defaults.Tun.MTU
	}
	if cfg.Tun.Stack == "" {
		cfg.Tun.Stack = defaults.Tun.Stack
	}

	// DNS 默认值
	if cfg.DNS.FakeIPRange == "" {
		cfg.DNS.FakeIPRange = defaults.DNS.FakeIPRange
	}
	if cfg.DNS.EnhancedMode == "" {
		cfg.DNS.EnhancedMode = defaults.DNS.EnhancedMode
	}
}

// validate 验证配置
func (p *Parser) validate(cfg *Config) error {
	// 验证端口
	if cfg.MixedPort < 0 || cfg.MixedPort > 65535 {
		return fmt.Errorf("无效的混合端口: %d", cfg.MixedPort)
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("无效的HTTP端口: %d", cfg.Port)
	}
	if cfg.SocksPort < 0 || cfg.SocksPort > 65535 {
		return fmt.Errorf("无效的SOCKS端口: %d", cfg.SocksPort)
	}

	// 验证模式
	validModes := map[string]bool{
		"rule":   true,
		"global": true,
		"direct": true,
	}
	if !validModes[cfg.Mode] {
		return fmt.Errorf("无效的模式: %s", cfg.Mode)
	}

	// 验证日志级别
	validLevels := map[string]bool{
		"silent":  true,
		"error":   true,
		"warning": true,
		"info":    true,
		"debug":   true,
	}
	if !validLevels[cfg.LogLevel] {
		return fmt.Errorf("无效的日志级别: %s", cfg.LogLevel)
	}

	return nil
}

// ParseFile 从文件解析配置
func ParseFile(path string) (*Config, error) {
	parser := NewParser(path)
	return parser.Parse()
}

// ParseBytes 从字节解析配置
func ParseBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	parser := NewParser("")
	parser.applyDefaults(&cfg)

	if err := parser.validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// WriteFile 写入配置文件
func WriteFile(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
