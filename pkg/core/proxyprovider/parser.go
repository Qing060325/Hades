// Package proxyprovider 订阅格式解析器
package proxyprovider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Qing060325/Hades/internal/config"
	"gopkg.in/yaml.v3"
)

// clashConfig Clash 格式配置结构
type clashConfig struct {
	Proxies []clashProxy `yaml:"proxies"`
}

// clashProxy Clash 格式代理节点
type clashProxy struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Server         string            `yaml:"server"`
	Port           int               `yaml:"port"`
	Password       string            `yaml:"password"`
	Cipher         string            `yaml:"cipher"`
	UUID           string            `yaml:"uuid"`
	AlterID        int               `yaml:"alter-id"`
	Network        string            `yaml:"network"`
	TLS            bool              `yaml:"tls"`
	SkipCertVerify bool              `yaml:"skip-cert-verify"`
	ServerName     string            `yaml:"servername"`
	WSPath         string            `yaml:"ws-path"`
	WSHeaders      map[string]string `yaml:"ws-headers"`
	UDP            bool              `yaml:"udp"`
	Obfs           string            `yaml:"obfs"`
	ObfsParam      string            `yaml:"obfs-param"`
	Protocol       string            `yaml:"protocol"`
	ProtocolParam  string            `yaml:"protocol-param"`
	ALPN           []string          `yaml:"alpn"`
	Fingerprint    string            `yaml:"fingerprint"`
	SNI            string            `yaml:"sni"`
	Flow           string            `yaml:"flow"`
	PSK            string            `yaml:"psk"`
	Up             string            `yaml:"up"`
	Down           string            `yaml:"down"`
	PrivateKey     string            `yaml:"private-key"`
	PublicKey      string            `yaml:"public-key"`
	PreSharedKey   string            `yaml:"pre-shared-key"`
	MTU            int               `yaml:"mtu"`
	Reserved       []int             `yaml:"reserved"`
	Password2      string            `yaml:"auth-str"`
	ObfsPassword   string            `yaml:"obfs-password"`
}

// jsonProxyConfig JSON 格式代理配置
type jsonProxyConfig struct {
	Proxies []jsonProxy `json:"proxies"`
}

// jsonProxy JSON 格式代理节点
type jsonProxy struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Cipher   string `json:"cipher"`
	Password string `json:"password"`
	UUID     string `json:"uuid"`
	AlterID  int    `json:"alter_id"`
	Network  string `json:"network"`
	TLS      bool   `json:"tls"`
	UDP      bool   `json:"udp"`
}

// ParseSubscription 自动检测格式并解析订阅数据
// 支持: YAML (Clash), Base64 编码, JSON
func ParseSubscription(data []byte) ([]config.ProxyConfig, error) {
	data = trimBOM(data)

	// 尝试 YAML (Clash 格式)
	if proxies, err := parseClashYAML(data); err == nil && len(proxies) > 0 {
		return proxies, nil
	}

	// 尝试 JSON
	if proxies, err := parseJSON(data); err == nil && len(proxies) > 0 {
		return proxies, nil
	}

	// 尝试 Base64 解码
	if proxies, err := parseBase64(data); err == nil && len(proxies) > 0 {
		return proxies, nil
	}

	return nil, fmt.Errorf("unsupported subscription format (tried YAML, JSON, Base64)")
}

// trimBOM 去除 UTF-8 BOM
func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// parseClashYAML 解析 Clash YAML 格式
func parseClashYAML(data []byte) ([]config.ProxyConfig, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if len(cfg.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies in clash config")
	}

	proxies := make([]config.ProxyConfig, 0, len(cfg.Proxies))
	for _, p := range cfg.Proxies {
		proxy := convertClashProxy(p)
		if proxy != nil {
			proxies = append(proxies, *proxy)
		}
	}

	return proxies, nil
}

// convertClashProxy 转换 Clash 代理到内部格式
func convertClashProxy(p clashProxy) *config.ProxyConfig {
	proxy := &config.ProxyConfig{
		Name:           p.Name,
		Type:           strings.ToLower(p.Type),
		Server:         p.Server,
		Port:           p.Port,
		Password:       p.Password,
		Cipher:         p.Cipher,
		UUID:           p.UUID,
		AlterID:        p.AlterID,
		Network:        p.Network,
		TLS:            p.TLS,
		SkipCertVerify: p.SkipCertVerify,
		ServerName:     p.ServerName,
		WSPath:         p.WSPath,
		WSHeaders:      p.WSHeaders,
		UDP:            p.UDP,
		Obfs:           p.Obfs,
		ObfsParam:      p.ObfsParam,
		Protocol:       p.Protocol,
		ProtocolParam:  p.ProtocolParam,
		ALPN:           p.ALPN,
		Fingerprint:    p.Fingerprint,
		SNI:            p.SNI,
		Flow:           p.Flow,
		PrivateKey:     p.PrivateKey,
		PublicKey:       p.PublicKey,
		PreSharedKey:   p.PreSharedKey,
		MTU:            p.MTU,
		Reserved:       p.Reserved,
	}

	// 处理类型映射
	switch proxy.Type {
	case "ss", "shadowsocks":
		proxy.Type = "ss"
	case "ssr", "shadowsocksr":
		proxy.Type = "ssr"
	case "vmess":
		proxy.Type = "vmess"
	case "vless":
		proxy.Type = "vless"
	case "trojan":
		proxy.Type = "trojan"
	case "hysteria":
		proxy.Type = "hysteria"
		proxy.HysteriaOpts = &config.HysteriaOpts{
			Up:      p.Up,
			Down:    p.Down,
			Obfs:    p.Obfs,
			AuthStr: p.Password2,
			ALPN:    p.ALPN,
		}
	case "hysteria2":
		proxy.Type = "hysteria2"
		proxy.Hysteria2Opts = &config.Hysteria2Opts{
			Up:           p.Up,
			Down:         p.Down,
			ObfsPassword: p.ObfsPassword,
			ALPN:         p.ALPN,
		}
	case "wireguard":
		proxy.Type = "wireguard"
	case "tuic":
		proxy.Type = "tuic"
	case "snell":
		proxy.Type = "snell"
		proxy.SnellOpts = &config.SnellOpts{
			PSK: p.PSK,
		}
	default:
		// 未知类型，跳过
		return nil
	}

	return proxy
}

// parseJSON 解析 JSON 格式
func parseJSON(data []byte) ([]config.ProxyConfig, error) {
	var cfg jsonProxyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if len(cfg.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies in JSON config")
	}

	proxies := make([]config.ProxyConfig, 0, len(cfg.Proxies))
	for _, p := range cfg.Proxies {
		proxy := config.ProxyConfig{
			Name:     p.Name,
			Type:     strings.ToLower(p.Type),
			Server:   p.Server,
			Port:     p.Port,
			Cipher:   p.Cipher,
			Password: p.Password,
			UUID:     p.UUID,
			AlterID:  p.AlterID,
			Network:  p.Network,
			TLS:      p.TLS,
			UDP:      p.UDP,
		}
		proxies = append(proxies, proxy)
	}

	return proxies, nil
}

// parseBase64 解析 Base64 编码的订阅
// 每行一个代理 URI: vmess://, ss://, trojan://, vless://, ssr:// 等
func parseBase64(data []byte) ([]config.ProxyConfig, error) {
	// 尝试标准 Base64 和 URL-safe Base64
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(string(data))
		if err != nil {
			// 尝试 RawStdEncoding (无 padding)
			decoded, err = base64.RawStdEncoding.DecodeString(string(data))
			if err != nil {
				decoded, err = base64.RawURLEncoding.DecodeString(string(data))
				if err != nil {
					return nil, fmt.Errorf("base64 decode failed: %w", err)
				}
			}
		}
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var proxies []config.ProxyConfig

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		proxy, err := parseProxyURI(line)
		if err != nil {
			continue // 跳过解析失败的行
		}
		if proxy != nil {
			proxies = append(proxies, *proxy)
		}
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("no valid proxies in base64 data")
	}

	return proxies, nil
}

// parseProxyURI 解析单个代理 URI
func parseProxyURI(uri string) (*config.ProxyConfig, error) {
	idx := strings.Index(uri, "://")
	if idx < 0 {
		return nil, fmt.Errorf("invalid uri format")
	}

	scheme := strings.ToLower(uri[:idx])
	body := uri[idx+3:]

	switch scheme {
	case "ss", "shadowsocks":
		return parseSSURI(body)
	case "ssr", "shadowsocksr":
		return parseSSRURI(body)
	case "vmess":
		return parseVMessURI(body)
	case "vless":
		return parseVLESSURI(body)
	case "trojan":
		return parseTrojanURI(body)
	case "hysteria2", "hy2":
		return parseHysteria2URI(body)
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", scheme)
	}
}

// parseSSURI 解析 ss:// URI
// 格式: ss://base64(method:password)@server:port#name
// 或:   ss://base64(method:password@server:port)#name
func parseSSURI(body string) (*config.ProxyConfig, error) {
	name := ""
	if idx := strings.LastIndex(body, "#"); idx >= 0 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	// 尝试 SIP002 格式: base64(method:password)@server:port
	if idx := strings.LastIndex(body, "@"); idx >= 0 {
		userInfo := body[:idx]
		hostPort := body[idx+1:]

		decoded, err := base64.RawURLEncoding.DecodeString(userInfo)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(userInfo)
			if err != nil {
				return nil, err
			}
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ss userinfo")
		}

		host, portStr, err := netSplitHostPort(hostPort)
		if err != nil {
			return nil, err
		}
		port, _ := strconv.Atoi(portStr)

		return &config.ProxyConfig{
			Name:     name,
			Type:     "ss",
			Server:   host,
			Port:     port,
			Cipher:   parts[0],
			Password: parts[1],
		}, nil
	}

	// 兼容格式: base64(method:password@server:port)
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, err
		}
	}

	parts := strings.SplitN(string(decoded), "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ss format")
	}

	methodPass := strings.SplitN(parts[0], ":", 2)
	if len(methodPass) != 2 {
		return nil, fmt.Errorf("invalid ss method:password")
	}

	host, portStr, err := netSplitHostPort(parts[1])
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)

	return &config.ProxyConfig{
		Name:     name,
		Type:     "ss",
		Server:   host,
		Port:     port,
		Cipher:   methodPass[0],
		Password: methodPass[1],
	}, nil
}

// parseSSRURI 解析 ssr:// URI
func parseSSRURI(body string) (*config.ProxyConfig, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(body)
		if err != nil {
			return nil, err
		}
	}

	// SSR 格式: server:port:protocol:method:obfs:base64pass/?params
	parts := strings.SplitN(string(decoded), ":", 6)
	if len(parts) < 6 {
		return nil, fmt.Errorf("invalid ssr format")
	}

	port, _ := strconv.Atoi(parts[1])

	// 解析密码 (base64)
	passParts := strings.SplitN(parts[5], "/?", 2)
	password, _ := base64.RawURLEncoding.DecodeString(passParts[0])

	return &config.ProxyConfig{
		Name:      "SSR",
		Type:      "ssr",
		Server:    parts[0],
		Port:      port,
		Protocol:  parts[2],
		Cipher:    parts[3],
		Obfs:      parts[4],
		Password:  string(password),
	}, nil
}

// parseVMessURI 解析 vmess:// URI
// vmess URI 是 base64 编码的 JSON
func parseVMessURI(body string) (*config.ProxyConfig, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(body)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(body)
			if err != nil {
				return nil, err
			}
		}
	}

	var vm struct {
		Name   string `json:"ps"`
		Server string `json:"add"`
		Port   int    `json:"port"`
		UUID   string `json:"id"`
		AlterID int   `json:"aid"`
		Net    string `json:"net"`
		Type   string `json:"type"`
		Host   string `json:"host"`
		Path   string `json:"path"`
		TLS    string `json:"tls"`
		SNI    string `json:"sni"`
		FP     string `json:"fp"`
		ALPN   string `json:"alpn"`
	}

	if err := json.Unmarshal(decoded, &vm); err != nil {
		return nil, err
	}

	proxy := &config.ProxyConfig{
		Name:    vm.Name,
		Type:    "vmess",
		Server:  vm.Server,
		Port:    vm.Port,
		UUID:    vm.UUID,
		AlterID: vm.AlterID,
		Network: vm.Net,
		TLS:     vm.TLS == "tls",
		SNI:     vm.SNI,
		Fingerprint: vm.FP,
	}

	if vm.Host != "" {
		proxy.WSHeaders = map[string]string{"Host": vm.Host}
	}
	if vm.Path != "" {
		proxy.WSPath = vm.Path
	}
	if vm.ALPN != "" {
		proxy.ALPN = strings.Split(vm.ALPN, ",")
	}

	return proxy, nil
}

// parseVLESSURI 解析 vless:// URI
// 格式: vless://uuid@server:port?params#name
func parseVLESSURI(body string) (*config.ProxyConfig, error) {
	name := ""
	if idx := strings.LastIndex(body, "#"); idx >= 0 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	idx := strings.Index(body, "@")
	if idx < 0 {
		return nil, fmt.Errorf("invalid vless uri")
	}

	uuid := body[:idx]
	hostPort := body[idx+1:]

	// 分离 query 参数
	queryStr := ""
	if qIdx := strings.Index(hostPort, "?"); qIdx >= 0 {
		queryStr = hostPort[qIdx+1:]
		hostPort = hostPort[:qIdx]
	}

	host, portStr, err := netSplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)

	proxy := &config.ProxyConfig{
		Name:   name,
		Type:   "vless",
		Server: host,
		Port:   port,
		UUID:   uuid,
	}

	// 解析 query 参数
	if queryStr != "" {
		params, _ := url.ParseQuery(queryStr)
		proxy.Network = params.Get("type")
		proxy.SNI = params.Get("sni")
		proxy.Flow = params.Get("flow")
		proxy.Fingerprint = params.Get("fp")

		if params.Get("security") == "tls" {
			proxy.TLS = true
		}
		if alpn := params.Get("alpn"); alpn != "" {
			proxy.ALPN = strings.Split(alpn, ",")
		}
		if host := params.Get("host"); host != "" {
			proxy.WSHeaders = map[string]string{"Host": host}
		}
		if path := params.Get("path"); path != "" {
			proxy.WSPath = path
		}
	}

	return proxy, nil
}

// parseTrojanURI 解析 trojan:// URI
// 格式: trojan://password@server:port?params#name
func parseTrojanURI(body string) (*config.ProxyConfig, error) {
	name := ""
	if idx := strings.LastIndex(body, "#"); idx >= 0 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	idx := strings.Index(body, "@")
	if idx < 0 {
		return nil, fmt.Errorf("invalid trojan uri")
	}

	password := body[:idx]
	hostPort := body[idx+1:]

	queryStr := ""
	if qIdx := strings.Index(hostPort, "?"); qIdx >= 0 {
		queryStr = hostPort[qIdx+1:]
		hostPort = hostPort[:qIdx]
	}

	host, portStr, err := netSplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)

	proxy := &config.ProxyConfig{
		Name:     name,
		Type:     "trojan",
		Server:   host,
		Port:     port,
		Password: password,
		TLS:      true,
	}

	if queryStr != "" {
		params, _ := url.ParseQuery(queryStr)
		proxy.SNI = params.Get("sni")
		proxy.Network = params.Get("type")
		if host := params.Get("host"); host != "" {
			proxy.WSHeaders = map[string]string{"Host": host}
		}
		if path := params.Get("path"); path != "" {
			proxy.WSPath = path
		}
	}

	return proxy, nil
}

// parseHysteria2URI 解析 hysteria2:// URI
// 格式: hysteria2://password@server:port?params#name
func parseHysteria2URI(body string) (*config.ProxyConfig, error) {
	name := ""
	if idx := strings.LastIndex(body, "#"); idx >= 0 {
		name, _ = url.QueryUnescape(body[idx+1:])
		body = body[:idx]
	}

	idx := strings.Index(body, "@")
	if idx < 0 {
		return nil, fmt.Errorf("invalid hy2 uri")
	}

	password := body[:idx]
	hostPort := body[idx+1:]

	queryStr := ""
	if qIdx := strings.Index(hostPort, "?"); qIdx >= 0 {
		queryStr = hostPort[qIdx+1:]
		hostPort = hostPort[:qIdx]
	}

	host, portStr, err := netSplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)

	proxy := &config.ProxyConfig{
		Name:     name,
		Type:     "hysteria2",
		Server:   host,
		Port:     port,
		Password: password,
	}

	if queryStr != "" {
		params, _ := url.ParseQuery(queryStr)
		proxy.SNI = params.Get("sni")
		if obfs := params.Get("obfs"); obfs != "" {
			proxy.Hysteria2Opts = &config.Hysteria2Opts{
				Obfs:         obfs,
				ObfsPassword: params.Get("obfs-password"),
			}
		}
		if insecure := params.Get("insecure"); insecure == "1" {
			proxy.SkipCertVerify = true
		}
	}

	return proxy, nil
}

// netSplitHostPort 分离 host:port (兼容 IPv6)
func netSplitHostPort(hostPort string) (string, string, error) {
	// 处理 IPv6 地址 [::1]:port
	if strings.HasPrefix(hostPort, "[") {
		idx := strings.LastIndex(hostPort, "]")
		if idx < 0 {
			return "", "", fmt.Errorf("invalid IPv6 address")
		}
		host := hostPort[1:idx]
		if idx+1 < len(hostPort) && hostPort[idx+1] == ':' {
			return host, hostPort[idx+2:], nil
		}
		return host, "", nil
	}

	// 普通 host:port
	parts := strings.SplitN(hostPort, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid host:port")
	}
	return parts[0], parts[1], nil
}
