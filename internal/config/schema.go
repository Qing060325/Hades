// Package config 配置管理模块
package config

import (
	"net/netip"
	"time"
)

// Config 主配置结构
type Config struct {
	// 配置文件路径（非序列化字段）
	ConfigPath string `yaml:"-"`

	// 基础配置
	MixedPort    int    `yaml:"mixed-port"`
	Port         int    `yaml:"port"`
	SocksPort    int    `yaml:"socks-port"`
	RedirPort    int    `yaml:"redir-port"`
	TProxyPort   int    `yaml:"tproxy-port"`
	AllowLan     bool   `yaml:"allow-lan"`
	BindAddress  string `yaml:"bind-address"`
	Mode         string `yaml:"mode"` // rule / global / direct
	LogLevel     string `yaml:"log-level"`
	IPv6         bool   `yaml:"ipv6"`

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
	Proxies []ProxyConfig `yaml:"proxies"`

	// 代理组
	ProxyGroups []ProxyGroupConfig `yaml:"proxy-groups"`

	// 规则
	Rules []string `yaml:"rules"`

	// 规则集
	RuleProviders map[string]RuleProviderConfig `yaml:"rule-providers"`

	// 代理提供者
	ProxyProviders map[string]ProxyProviderConfig `yaml:"proxy-providers"`

	// 订阅配置
	Subscriptions []SubscriptionConfig `yaml:"subscriptions"`

	// GeoData
	GeoXURL    map[string]string `yaml:"geox-url"`
	GeoDataMode bool             `yaml:"geodata-mode"`
}

// TunConfig TUN 模式配置
type TunConfig struct {
	Enable              bool     `yaml:"enable"`
	Stack               string   `yaml:"stack"` // system / gvisor / mixed
	DNSHijack           []string `yaml:"dns-hijack"`
	AutoRoute           bool     `yaml:"auto-route"`
	AutoDetectInterface bool     `yaml:"auto-detect-interface"`
	StrictRoute         bool     `yaml:"strict-route"`
	MTU                 int      `yaml:"mtu"`
}

// DNSConfig DNS 配置
type DNSConfig struct {
	Enable            bool              `yaml:"enable"`
	Listen            string            `yaml:"listen"`
	EnhancedMode      string            `yaml:"enhanced-mode"` // fake-ip / redir-host
	FakeIPRange       string            `yaml:"fake-ip-range"`
	FakeIPFilter      []string          `yaml:"fake-ip-filter"`
	FakeIPFilterMode  string            `yaml:"fake-ip-filter-mode"`
	Nameserver        []string          `yaml:"nameserver"`
	Fallback          []string          `yaml:"fallback"`
	FallbackFilter    FallbackFilter    `yaml:"fallback-filter"`
	NameserverPolicy  map[string]string `yaml:"nameserver-policy"`
	DefaultNameserver []string          `yaml:"default-nameserver"`
	ProxyServerNameserver []string      `yaml:"proxy-server-nameserver"`
}

// FallbackFilter 回退过滤器
type FallbackFilter struct {
	GeoIP     bool     `yaml:"geoip"`
	GeoIPCode string   `yaml:"geoip-code"`
	GeoSite   []string `yaml:"geosite"`
	IPCIDR    []string `yaml:"ipcidr"`
	Domain    []string `yaml:"domain"`
}

// SnifferConfig 嗅探配置
type SnifferConfig struct {
	Enable              bool              `yaml:"enable"`
	OverrideDestination bool              `yaml:"override-destination"`
	ForceDNSMapping     bool              `yaml:"force-dns-mapping"`
	ParsePureIP         bool              `yaml:"parse-pure-ip"`
	Sniff               map[string]Ports  `yaml:"sniff"`
	ForceDomain         []string          `yaml:"force-domain"`
	SkipDomain          []string          `yaml:"skip-domain"`
}

// Ports 端口范围
type Ports struct {
	Ports []string `yaml:"ports"`
}

// ProxyConfig 代理节点配置
type ProxyConfig struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"`
	Server     string            `yaml:"server"`
	Port       int               `yaml:"port"`
	Cipher     string            `yaml:"cipher"`
	Password   string            `yaml:"password"`
	UUID       string            `yaml:"uuid"`
	AlterID    int               `yaml:"alter-id"`
	Network    string            `yaml:"network"`
	TLS        bool              `yaml:"tls"`
	SkipCertVerify bool          `yaml:"skip-cert-verify"`
	ServerName string            `yaml:"servername"`
	WSPath     string            `yaml:"ws-path"`
	WSHeaders  map[string]string `yaml:"ws-headers"`
	GRPCServiceName string       `yaml:"grpc-service-name"`
	Flow       string            `yaml:"flow"` // VLESS XTLS
	SNI        string            `yaml:"sni"`
	UDP        bool              `yaml:"udp"`
	Obfs       string            `yaml:"obfs"`
	ObfsParam  string            `yaml:"obfs-param"`
	Protocol   string            `yaml:"protocol"`
	ProtocolParam string         `yaml:"protocol-param"`
	ALPN       []string          `yaml:"alpn"`
	Fingerprint string           `yaml:"fingerprint"`
	ClientFingerprint string     `yaml:"client-fingerprint"`
	Reality    *RealityConfig    `yaml:"reality"`
	SSOpts     *ShadowsocksOpts  `yaml:"ss-opts"`
	HysteriaOpts *HysteriaOpts   `yaml:"hysteria-opts"`
	Hysteria2Opts *Hysteria2Opts `yaml:"hysteria2-opts"`
	TUICOpts     *TUICOpts       `yaml:"tuic-opts"`
	WireGuardOpts *WireGuardOpts `yaml:"wireguard-opts"`
	// WireGuard 直接字段
	PrivateKey   string   `yaml:"private-key"`
	PublicKey    string   `yaml:"public-key"`
	PreSharedKey string   `yaml:"pre-shared-key"`
	MTU          int      `yaml:"mtu"`
	Reserved     []int    `yaml:"reserved"`
}

// RealityConfig Reality 配置
type RealityConfig struct {
	PublicKey string `yaml:"public-key"`
	ShortID  string `yaml:"short-id"`
}

// ShadowsocksOpts Shadowsocks 高级选项
type ShadowsocksOpts struct {
	UDPOverTCP bool `yaml:"udp-over-tcp"`
}

// HysteriaOpts Hysteria 高级选项
type HysteriaOpts struct {
	Up        string   `yaml:"up"`
	Down      string   `yaml:"down"`
	Obfs      string   `yaml:"obfs"`
	Auth      string   `yaml:"auth"`
	AuthStr   string   `yaml:"auth-str"`
	ALPN      []string `yaml:"alpn"`
}

// Hysteria2Opts Hysteria2 配置
type Hysteria2Opts struct {
	Up           string   `yaml:"up"`
	Down         string   `yaml:"down"`
	Obfs         string   `yaml:"obfs"`
	ObfsPassword string   `yaml:"obfs-password"`
	ALPN         []string `yaml:"alpn"`
}

// TUICOpts TUIC 配置
type TUICOpts struct {
	CongestionController string   `yaml:"congestion-controller"` // bbr / cubic / new_reno
	UDPRelayMode         string   `yaml:"udp-relay-mode"`        // native / quic
	HeartbeatInterval    int      `yaml:"heartbeat-interval"`
	ALPN                 []string `yaml:"alpn"`
	DisableSNI           bool     `yaml:"disable-sni"`
	ReduceRTT            bool     `yaml:"reduce-rtt"`
	RequestTimeout       int      `yaml:"request-timeout"`
	UDPOverStream        bool     `yaml:"udp-over-stream"`
	ZeroRTTHandshake     bool     `yaml:"zero-rtt-handshake"`
}

// WireGuardOpts WireGuard 高级选项
type WireGuardOpts struct {
	PrivateKey  string   `yaml:"private-key"`
	Peers       []WgPeer `yaml:"peers"`
	MTU         int      `yaml:"mtu"`
	AllowedIPs  []string `yaml:"allowed-ips"`
}

// WgPeer WireGuard Peer 配置
type WgPeer struct {
	PublicKey    string   `yaml:"public-key"`
	PreSharedKey string   `yaml:"pre-shared-key"`
	Endpoint     string   `yaml:"endpoint"`
	AllowedIPs   []string `yaml:"allowed-ips"`
	KeepAlive    int      `yaml:"keepalive"`
}

// ProxyGroupConfig 代理组配置
type ProxyGroupConfig struct {
	Name             string   `yaml:"name"`
	Type             string   `yaml:"type"` // select / url-test / fallback / load-balance
	Proxies          []string `yaml:"proxies"`
	Use              []string `yaml:"use"`
	URL              string   `yaml:"url"`
	Interval         int      `yaml:"interval"`
	Tolerance        int      `yaml:"tolerance"`
	Lazy             bool     `yaml:"lazy"`
	Timeout          int      `yaml:"timeout"`
	MaxFailedTimes   int      `yaml:"max-failed-times"`
	DisableUDP       bool     `yaml:"disable-udp"`
	IncludeAll       bool     `yaml:"include-all"`
	IncludeAllProxies bool    `yaml:"include-all-proxies"`
	IncludeAllProviders bool  `yaml:"include-all-providers"`
	Filter           string   `yaml:"filter"`
	ExcludeFilter    string   `yaml:"exclude-filter"`
	ExcludeType      string   `yaml:"exclude-type"`
	ExpectedStatus   int      `yaml:"expected-status"`
	Hidden           bool     `yaml:"hidden"`
	Icon             string   `yaml:"icon"`
}

// RuleProviderConfig 规则集配置
type RuleProviderConfig struct {
	Type     string `yaml:"type"`     // http / file
	Behavior string `yaml:"behavior"` // domain / ipcidr / classical
	Format   string `yaml:"format"`   // yaml / text / mrs
	URL      string `yaml:"url"`
	Path     string `yaml:"path"`
	Interval int    `yaml:"interval"`
	Proxy    string `yaml:"proxy"`
}

// ProxyProviderConfig 代理提供者配置
type ProxyProviderConfig struct {
	Type       string        `yaml:"type"` // http / file
	URL        string        `yaml:"url"`
	Path       string        `yaml:"path"`
	Interval   time.Duration `yaml:"interval"`
	Proxy      string        `yaml:"proxy"`
	Header     map[string]string `yaml:"header"`
	HealthCheck HealthCheckConfig `yaml:"health-check"`
}

// HealthCheckConfig 健康检查配置
type HealthCheckConfig struct {
	Enable   bool          `yaml:"enable"`
	URL      string        `yaml:"url"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Lazy     bool          `yaml:"lazy"`
}

// SubscriptionConfig 订阅配置
type SubscriptionConfig struct {
	Name       string        `yaml:"name"`
	URL        string        `yaml:"url"`
	Interval   time.Duration `yaml:"interval"`
	AutoUpdate bool          `yaml:"auto-update"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		MixedPort:   7890,
		AllowLan:    false,
		BindAddress: "*",
		Mode:        "rule",
		LogLevel:    "info",
		IPv6:        false,
		Tun: TunConfig{
			Enable:    false,
			Stack:     "mixed",
			MTU:       9000,
			AutoRoute: true,
		},
		DNS: DNSConfig{
			Enable:       true,
			Listen:       "0.0.0.0:1053",
			EnhancedMode: "fake-ip",
			FakeIPRange:  "198.18.0.1/16",
			DefaultNameserver: []string{
				"223.5.5.5",
				"119.29.29.29",
			},
			Nameserver: []string{
				"https://dns.alidns.com/dns-query",
				"https://doh.pub/dns-query",
			},
			Fallback: []string{
				"tls://8.8.4.4",
				"tls://1.1.1.1",
			},
		},
		Sniffer: SnifferConfig{
			Enable:              true,
			OverrideDestination: true,
			ForceDNSMapping:     true,
			ParsePureIP:         true,
			Sniff: map[string]Ports{
				"HTTP": {Ports: []string{"80", "8080-8880"}},
				"TLS":  {Ports: []string{"443", "8443"}},
				"QUIC": {Ports: []string{"443", "8443"}},
			},
		},
		GeoDataMode: true,
	}
}

// ParseIP 解析IP地址
func ParseIP(s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

// ParsePrefix 解析IP前缀
func ParsePrefix(s string) *netip.Prefix {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		return nil
	}
	return &prefix
}
