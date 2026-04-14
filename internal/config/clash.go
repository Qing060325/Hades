// Package config 配置管理模块 - Clash YAML 兼容层
package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashConfig Clash/Mihomo 兼容配置结构
// 支持所有 Clash 配置字段，自动转换到 Hades 配置
type ClashConfig struct {
	// 基础配置
	Port           int    `yaml:"port"`
	SocksPort      int    `yaml:"socks-port"`
	MixedPort      int    `yaml:"mixed-port"`
	RedirPort      int    `yaml:"redir-port"`
	TProxyPort     int    `yaml:"tproxy-port"`
	AllowLan       bool   `yaml:"allow-lan"`
	BindAddress    string `yaml:"bind-address"`
	Mode           string `yaml:"mode"`
	LogLevel       string `yaml:"log-level"`
	IPv6           bool   `yaml:"ipv6"`

	// 外部控制
	ExternalController string `yaml:"external-controller"`
	ExternalUI         string `yaml:"external-ui"`
	Secret             string `yaml:"secret"`

	// TUN 模式
	Tun TunConfig `yaml:"tun"`

	// DNS 配置
	DNS DNSConfig `yaml:"dns"`

	// 嗅探配置
	Sniffer SnifferConfig `yaml:"sniffer"`

	// 代理节点
	Proxies []ClashProxyConfig `yaml:"proxies"`

	// 代理组
	ProxyGroups []ProxyGroupConfig `yaml:"proxy-groups"`

	// 规则
	Rules []string `yaml:"rules"`

	// 规则集
	RuleProviders map[string]RuleProviderConfig `yaml:"rule-providers"`

	// 代理提供者
	ProxyProviders map[string]ProxyProviderConfig `yaml:"proxy-providers"`

	// GeoData
	GeoXURL    map[string]string `yaml:"geox-url"`
	GeodataMode bool             `yaml:"geodata-mode"`

	// Clash 特有字段 (兼容)
	UnifiedDelay    bool   `yaml:"unified-delay"`
	TcpConcurrent   bool   `yaml:"tcp-concurrent"`
	FindProcessMode string `yaml:"find-process-mode"`
}

// ClashProxyConfig Clash 代理配置
type ClashProxyConfig struct {
	// 基础字段
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Server     string `yaml:"server"`
	Port       int    `yaml:"port"`
	Cipher     string `yaml:"cipher"`
	Password   string `yaml:"password"`
	UUID       string `yaml:"uuid"`
	AlterID    int    `yaml:"alterId"`
	Network    string `yaml:"network"`
	TLS        bool   `yaml:"tls"`
	SkipCertVerify bool `yaml:"skip-cert-verify"`
	ServerName string `yaml:"servername"`

	// WebSocket
	WSPath    string            `yaml:"ws-path"`
	WSHeaders map[string]string `yaml:"ws-headers"`

	// gRPC
	GRPCServiceName string `yaml:"grpc-service-name"`

	// VLESS
	Flow string `yaml:"flow"`

	// SNI
	SNI string `yaml:"sni"`

	// UDP
	UDP bool `yaml:"udp"`

	// Shadowsocks 插件
	Plugin     string                 `yaml:"plugin"`
	PluginOpts map[string]interface{} `yaml:"plugin-opts"`

	// Obfs
	Obfs      string `yaml:"obfs"`
	ObfsParam string `yaml:"obfs-param"`

	// Protocol
	Protocol     string `yaml:"protocol"`
	ProtocolParam string `yaml:"protocol-param"`

	// ALPN
	ALPN []string `yaml:"alpn"`

	// TLS 指纹
	Fingerprint         string `yaml:"fingerprint"`
	ClientFingerprint   string `yaml:"client-fingerprint"`

	// Reality
	Reality *RealityConfig `yaml:"reality"`

	// Hysteria2
	Up           string `yaml:"up"`
	Down         string `yaml:"down"`
	ObfsPassword string `yaml:"obfs-password"`

	// TUIC
	CongestionController string `yaml:"congestion-controller"`
	UDPRelayMode         string `yaml:"udp-relay-mode"`
	HeartbeatInterval    int    `yaml:"heartbeat-interval"`

	// WireGuard
	PrivateKey   string   `yaml:"private-key"`
	PublicKey    string   `yaml:"public-key"`
	PreSharedKey string   `yaml:"pre-shared-key"`
	MTU          int      `yaml:"mtu"`
	AllowedIPs   []string `yaml:"allowed-ips"`
	Reserved     []int    `yaml:"reserved"`

	// Smux (多路复用)
	Smux *SmuxConfig `yaml:"smux"`

	// 其他可选字段
	IP      string `yaml:"ip"`
	Ports   string `yaml:"ports"`
	User    string `yaml:"user"`
	Pass    string `yaml:"pass"`
	Timeout int    `yaml:"timeout"`
}

// SmuxConfig 多路复用配置
type SmuxConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Protocol    string `yaml:"protocol"`
	MaxStreams  int    `yaml:"max-streams"`
	MaxConnWait int    `yaml:"max-conn-wait"`
}

// ParseClashConfig 解析 Clash 配置文件
func ParseClashConfig(data []byte) (*Config, error) {
	var clashCfg ClashConfig
	if err := yaml.Unmarshal(data, &clashCfg); err != nil {
		return nil, fmt.Errorf("解析 Clash 配置失败: %w", err)
	}

	// 转换为 Hades 配置
	return convertClashToHades(&clashCfg), nil
}

// convertClashToHades 将 Clash 配置转换为 Hades 配置
func convertClashToHades(clash *ClashConfig) *Config {
	cfg := Default()

	// 基础配置
	cfg.Port = clash.Port
	cfg.SocksPort = clash.SocksPort
	cfg.MixedPort = clash.MixedPort
	cfg.RedirPort = clash.RedirPort
	cfg.TProxyPort = clash.TProxyPort
	cfg.AllowLan = clash.AllowLan
	cfg.BindAddress = clash.BindAddress
	cfg.Mode = clash.Mode
	cfg.LogLevel = clash.LogLevel
	cfg.IPv6 = clash.IPv6

	// 如果 mixed-port 未设置但 port 设置了，使用 port
	if cfg.MixedPort == 0 && clash.Port > 0 {
		cfg.MixedPort = clash.Port
	}

	// 外部控制
	cfg.ExternalController = clash.ExternalController
	cfg.ExternalUI = clash.ExternalUI
	cfg.Secret = clash.Secret

	// TUN
	cfg.Tun = clash.Tun

	// DNS
	cfg.DNS = clash.DNS

	// Sniffer
	cfg.Sniffer = clash.Sniffer

	// 代理节点
	cfg.Proxies = make([]ProxyConfig, 0, len(clash.Proxies))
	for _, p := range clash.Proxies {
		cfg.Proxies = append(cfg.Proxies, convertClashProxy(p))
	}

	// 代理组
	cfg.ProxyGroups = clash.ProxyGroups

	// 规则
	cfg.Rules = clash.Rules

	// 规则集
	cfg.RuleProviders = clash.RuleProviders

	// 代理提供者
	cfg.ProxyProviders = clash.ProxyProviders

	// GeoData
	cfg.GeoXURL = clash.GeoXURL
	cfg.GeoDataMode = clash.GeodataMode

	return cfg
}

// convertClashProxy 转换 Clash 代理配置
func convertClashProxy(p ClashProxyConfig) ProxyConfig {
	cfg := ProxyConfig{
		Name:              p.Name,
		Type:              normalizeProxyType(p.Type),
		Server:            p.Server,
		Port:              p.Port,
		Cipher:            p.Cipher,
		Password:          p.Password,
		UUID:              p.UUID,
		AlterID:           p.AlterID,
		Network:           p.Network,
		TLS:               p.TLS,
		SkipCertVerify:    p.SkipCertVerify,
		ServerName:        p.ServerName,
		WSPath:            p.WSPath,
		WSHeaders:         p.WSHeaders,
		GRPCServiceName:   p.GRPCServiceName,
		Flow:              p.Flow,
		SNI:               p.SNI,
		UDP:               p.UDP,
		Obfs:              p.Obfs,
		ObfsParam:         p.ObfsParam,
		Protocol:          p.Protocol,
		ProtocolParam:     p.ProtocolParam,
		ALPN:              p.ALPN,
		Fingerprint:       p.Fingerprint,
		ClientFingerprint: p.ClientFingerprint,
		Reality:           p.Reality,
	}

	// 处理插件配置
	if p.Plugin != "" {
		cfg.Obfs = p.Plugin
		if p.PluginOpts != nil {
			if mode, ok := p.PluginOpts["mode"].(string); ok {
				cfg.ObfsParam = mode
			}
			if host, ok := p.PluginOpts["host"].(string); ok {
				if cfg.WSHeaders == nil {
					cfg.WSHeaders = make(map[string]string)
				}
				cfg.WSHeaders["Host"] = host
			}
		}
	}

	// Hysteria2 特有字段
	if p.Type == "hysteria2" {
		cfg.HysteriaOpts = &HysteriaOpts{
			Up:     p.Up,
			Down:   p.Down,
			Obfs:   p.Obfs,
			Auth:   p.Password,
			AuthStr: p.Password,
			ALPN:   p.ALPN,
		}
		if p.ObfsPassword != "" {
			cfg.HysteriaOpts.Obfs = "salamander"
		}
	}

	// WireGuard 特有字段
	if p.Type == "wireguard" {
		cfg.WireGuardOpts = &WireGuardOpts{
			PrivateKey: p.PrivateKey,
			MTU:        p.MTU,
			AllowedIPs: p.AllowedIPs,
		}
		if len(p.Reserved) >= 2 {
			cfg.WireGuardOpts.Peers = []WgPeer{{
				PublicKey:    p.PublicKey,
				PreSharedKey: p.PreSharedKey,
				AllowedIPs:   p.AllowedIPs,
			}}
		}
	}

	// TUIC 特有字段
	if p.Type == "tuic" {
		// TUIC 配置通过主配置字段处理
		cfg.ALPN = p.ALPN
	}

	return cfg
}

// normalizeProxyType 标准化代理类型名称
func normalizeProxyType(t string) string {
	// 统一类型名称
	switch strings.ToLower(t) {
	case "ss", "shadowsocks":
		return "ss"
	case "ssr", "shadowsocksr":
		return "ssr"
	case "vmess":
		return "vmess"
	case "vless":
		return "vless"
	case "trojan":
		return "trojan"
	case "hysteria", "hysteria1":
		return "hysteria"
	case "hysteria2", "hy2":
		return "hysteria2"
	case "tuic":
		return "tuic"
	case "wireguard", "wg":
		return "wireguard"
	case "http", "https":
		return "http"
	case "socks5", "socks":
		return "socks5"
	case "snell":
		return "snell"
	case "ssh":
		return "ssh"
	default:
		return t
	}
}

// DetectConfigFormat 检测配置格式
func DetectConfigFormat(data []byte) string {
	// 尝试解析为 Clash 配置
	var test map[string]interface{}
	if err := yaml.Unmarshal(data, &test); err != nil {
		return "unknown"
	}

	// 检测 Clash 特有字段
	if _, ok := test["proxies"]; ok {
		if proxies, ok := test["proxies"].([]interface{}); ok && len(proxies) > 0 {
			if p, ok := proxies[0].(map[string]interface{}); ok {
				if _, hasType := p["type"]; hasType {
					return "clash"
				}
			}
		}
	}

	return "hades"
}

// AutoParseConfig 自动检测配置格式并解析
func AutoParseConfig(data []byte) (*Config, error) {
	format := DetectConfigFormat(data)

	switch format {
	case "clash":
		return ParseClashConfig(data)
	default:
		return ParseBytes(data)
	}
}
