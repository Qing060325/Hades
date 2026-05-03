// pkg/subscription/subscription.go - 订阅管理
package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Subscription 订阅配置
type Subscription struct {
	Name         string        `yaml:"name"`
	URL          string        `yaml:"url"`
	Interval     time.Duration `yaml:"interval"`
	AutoUpdate   bool          `yaml:"auto-update"`
	LastUpdate   time.Time     `yaml:"-"`
	Nodes        []Node        `yaml:"-"`
	// 流量信息
	Upload       int64         `yaml:"-"` // 已上传字节
	Download     int64         `yaml:"-"` // 已下载字节
	Total        int64         `yaml:"-"` // 总流量限制
	Expire       int64         `yaml:"-"` // 到期时间戳
	mu           sync.RWMutex
}

// Node 代理节点
type Node struct {
	Name       string            `yaml:"name" json:"name"`
	Type       string            `yaml:"type" json:"type"`
	Server     string            `yaml:"server" json:"server"`
	Port       int               `yaml:"port" json:"port"`
	Password   string            `yaml:"password,omitempty" json:"password,omitempty"`
	UUID       string            `yaml:"uuid,omitempty" json:"uuid,omitempty"`
	AlterID    int               `yaml:"alter-id,omitempty" json:"alterId,omitempty"`
	Cipher     string            `yaml:"cipher,omitempty" json:"cipher,omitempty"`
	TLS        bool              `yaml:"tls,omitempty" json:"tls,omitempty"`
	SkipCert   bool              `yaml:"skip-cert-verify,omitempty" json:"skip-cert-verify,omitempty"`
	ServerName string            `yaml:"servername,omitempty" json:"servername,omitempty"`
	Network    string            `yaml:"network,omitempty" json:"network,omitempty"`
	WSPath     string            `yaml:"ws-path,omitempty" json:"ws-path,omitempty"`
	WSHeaders  map[string]string `yaml:"ws-headers,omitempty" json:"ws-headers,omitempty"`
	UDP        bool              `yaml:"udp,omitempty" json:"udp,omitempty"`
	Opts       map[string]string `yaml:"-" json:"-"`
}

// Manager 订阅管理器
type Manager struct {
	subscriptions map[string]*Subscription
	client        *http.Client
	mu            sync.RWMutex
	stopCh        chan struct{}
}

// NewManager 创建订阅管理器
func NewManager() *Manager {
	return &Manager{
		subscriptions: make(map[string]*Subscription),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Add 添加订阅
func (m *Manager) Add(sub *Subscription) error {
	if sub.Name == "" || sub.URL == "" {
		return fmt.Errorf("subscription name and URL are required")
	}

	if sub.Interval == 0 {
		sub.Interval = 3600 * time.Second // 默认1小时
	}

	m.mu.Lock()
	m.subscriptions[sub.Name] = sub
	m.mu.Unlock()

	// 立即更新一次
	if err := m.Update(sub.Name); err != nil {
		log.Warn().Err(err).Str("name", sub.Name).Msg("初始订阅更新失败")
	}

	log.Info().Str("name", sub.Name).Str("url", sub.URL).Msg("订阅已添加")
	return nil
}

// Update 手动更新订阅
func (m *Manager) Update(name string) error {
	m.mu.RLock()
	sub, exists := m.subscriptions[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("subscription %s not found", name)
	}

	nodes, upload, download, total, expire, err := m.fetchWithInfo(sub.URL)
	if err != nil {
		return fmt.Errorf("fetch subscription failed: %w", err)
	}

	sub.mu.Lock()
	sub.Nodes = nodes
	sub.LastUpdate = time.Now()
	sub.Upload = upload
	sub.Download = download
	sub.Total = total
	sub.Expire = expire
	sub.mu.Unlock()

	log.Info().
		Str("name", name).
		Int("nodes", len(nodes)).
		Int64("upload", upload).
		Int64("download", download).
		Int64("total", total).
		Msg("订阅更新成功")
	return nil
}

// UpdateAll 更新所有订阅
func (m *Manager) UpdateAll() error {
	m.mu.RLock()
	names := make([]string, 0, len(m.subscriptions))
	for name := range m.subscriptions {
		names = append(names, name)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, name := range names {
		if err := m.Update(name); err != nil {
			log.Error().Err(err).Str("name", name).Msg("订阅更新失败")
			lastErr = err
		}
	}

	return lastErr
}

// GetNodes 获取所有订阅节点
func (m *Manager) GetNodes() []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allNodes []Node
	for _, sub := range m.subscriptions {
		sub.mu.RLock()
		allNodes = append(allNodes, sub.Nodes...)
		sub.mu.RUnlock()
	}

	return allNodes
}

// GetSubscription 获取指定订阅
func (m *Manager) GetSubscription(name string) (*Subscription, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, exists := m.subscriptions[name]
	return sub, exists
}

// List 列出所有订阅
func (m *Manager) List() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		list = append(list, sub)
	}

	return list
}

// Remove 删除订阅
func (m *Manager) Remove(name string) {
	m.mu.Lock()
	delete(m.subscriptions, name)
	m.mu.Unlock()

	log.Info().Str("name", name).Msg("订阅已删除")
}

// Start 启动自动更新
func (m *Manager) Start() {
	go m.autoUpdateLoop()
	log.Info().Msg("订阅管理器已启动")
}

// Stop 停止自动更新
func (m *Manager) Stop() {
	close(m.stopCh)
	log.Info().Msg("订阅管理器已停止")
}

// autoUpdateLoop 自动更新循环
func (m *Manager) autoUpdateLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndUpdate()
		}
	}
}

// checkAndUpdate 检查并更新需要更新的订阅
func (m *Manager) checkAndUpdate() {
	m.mu.RLock()
	subs := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		if sub.AutoUpdate {
			subs = append(subs, sub)
		}
	}
	m.mu.RUnlock()

	now := time.Now()
	for _, sub := range subs {
		sub.mu.RLock()
		lastUpdate := sub.LastUpdate
		interval := sub.Interval
		sub.mu.RUnlock()

		if now.Sub(lastUpdate) >= interval {
			if err := m.Update(sub.Name); err != nil {
				log.Error().Err(err).Str("name", sub.Name).Msg("自动更新订阅失败")
			}
		}
	}
}

// fetch 拉取订阅内容
func (m *Manager) fetch(url string) ([]Node, error) {
	nodes, _, _, _, _, err := m.fetchWithInfo(url)
	return nodes, err
}

// fetchWithInfo 拉取订阅内容并解析流量信息
func (m *Manager) fetchWithInfo(url string) ([]Node, int64, int64, int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	// 设置常见 User-Agent
	req.Header.Set("User-Agent", "ClashForWindows/0.20.39")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, 0, 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	// 解析流量信息（从响应头）
	upload, download, total, expire := parseSubscriptionInfo(resp.Header)

	content := string(body)

	// 尝试 base64 解码
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err == nil {
		content = string(decoded)
	} else {
		// 尝试 RawStdEncoding
		decoded, err = base64.RawStdEncoding.DecodeString(content)
		if err == nil {
			content = string(decoded)
		}
	}

	nodes, err := parseSubscription(strings.TrimSpace(content))
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	return nodes, upload, download, total, expire, nil
}

// parseSubscriptionInfo 解析订阅流量信息
func parseSubscriptionInfo(header http.Header) (upload, download, total, expire int64) {
	// 尝试从 Subscription-UserInfo 头解析
	// 格式: upload=123; download=456; total=789; expire=1699123456
	info := header.Get("Subscription-UserInfo")
	if info == "" {
		// 尝试其他常见的头
		info = header.Get("X-Subscription-UserInfo")
	}
	if info == "" {
		info = header.Get("subscription-userinfo")
	}

	if info != "" {
		parts := strings.Split(info, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(strings.ToLower(kv[0]))
			value := strings.TrimSpace(kv[1])

			switch key {
			case "upload":
				fmt.Sscanf(value, "%d", &upload)
			case "download":
				fmt.Sscanf(value, "%d", &download)
			case "total":
				fmt.Sscanf(value, "%d", &total)
			case "expire":
				fmt.Sscanf(value, "%d", &expire)
			}
		}
	}

	return
}

// parseSubscription 解析订阅内容
func parseSubscription(content string) ([]Node, error) {
	lines := strings.Split(content, "\n")
	var nodes []Node

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var node *Node
		switch {
		case strings.HasPrefix(line, "vmess://"):
			node = parseVMess(line)
		case strings.HasPrefix(line, "vless://"):
			node = parseVLESS(line)
		case strings.HasPrefix(line, "trojan://"):
			node = parseTrojan(line)
		case strings.HasPrefix(line, "hysteria2://"), strings.HasPrefix(line, "hy2://"):
			node = parseHysteria2(line)
		case strings.HasPrefix(line, "ss://"):
			node = parseShadowsocks(line)
		case strings.HasPrefix(line, "anytls://"):
			node = parseAnyTLS(line)
		case strings.HasPrefix(line, "masque://"):
			node = parseMASQUE(line)
		case strings.HasPrefix(line, "trust-tunnel://"), strings.HasPrefix(line, "trusttunnel://"):
			node = parseTrustTunnel(line)
		case strings.HasPrefix(line, "sudoku://"):
			node = parseSudoku(line)
		}

		if node != nil {
			nodes = append(nodes, *node)
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid nodes found")
	}

	return nodes, nil
}

// parseVMess 解析 VMess 链接
func parseVMess(raw string) *Node {
	encoded := strings.TrimPrefix(raw, "vmess://")

	// 尝试解码
	var decoded []byte
	var err error

	// 先尝试标准 base64
	decoded, err = base64.StdEncoding.DecodeString(addBase64Padding(encoded))
	if err != nil {
		// 尝试 URL safe base64
		decoded, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(addBase64Padding(encoded))
			if err != nil {
				log.Debug().Err(err).Msg("vmess decode failed")
				return nil
			}
		}
	}

	var vmess struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		AID  any    `json:"aid"`
		Net  string `json:"net"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
		Alpn string `json:"alpn"`
	}

	if err := json.Unmarshal(decoded, &vmess); err != nil {
		log.Debug().Err(err).Msg("vmess json parse failed")
		return nil
	}

	port := parsePort(vmess.Port)
	if port == 0 {
		port = 443
	}

	node := &Node{
		Name:       vmess.PS,
		Type:       "vmess",
		Server:     vmess.Add,
		Port:       port,
		UUID:       vmess.ID,
		AlterID:    parseIntAny(vmess.AID),
		Cipher:     "auto",
		TLS:        vmess.TLS == "tls",
		ServerName: vmess.SNI,
		Network:    vmess.Net,
		UDP:        true,
		WSHeaders:  make(map[string]string),
	}

	if node.Name == "" {
		node.Name = vmess.Add
	}

	if node.Network == "" {
		node.Network = "tcp"
	}

	if node.Network == "ws" {
		node.WSPath = vmess.Path
		if vmess.Host != "" {
			node.WSHeaders["Host"] = vmess.Host
		}
	}

	if node.ServerName == "" && node.TLS {
		node.ServerName = vmess.Add
	}

	return node
}

// parseVLESS 解析 VLESS 链接
func parseVLESS(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	uuid := u.User.Username()
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		portStr = "443"
	}
	port := parsePort(portStr)

	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = host
	}

	node := &Node{
		Name:       name,
		Type:       "vless",
		Server:     host,
		Port:       port,
		UUID:       uuid,
		TLS:        q.Get("security") == "tls" || q.Get("security") == "xtls",
		ServerName: q.Get("sni"),
		Network:    q.Get("type"),
		UDP:        true,
		WSHeaders:  make(map[string]string),
	}

	if node.Network == "" {
		node.Network = "tcp"
	}

	if node.Network == "ws" {
		node.WSPath = q.Get("path")
		if hostHeader := q.Get("host"); hostHeader != "" {
			node.WSHeaders["Host"] = hostHeader
		}
	}

	if node.ServerName == "" && node.TLS {
		node.ServerName = host
	}

	return node
}

// parseTrojan 解析 Trojan 链接
func parseTrojan(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := u.User.Username()
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		portStr = "443"
	}
	port := parsePort(portStr)

	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = host
	}

	node := &Node{
		Name:       name,
		Type:       "trojan",
		Server:     host,
		Port:       port,
		Password:   password,
		TLS:        true,
		ServerName: q.Get("sni"),
		Network:    q.Get("type"),
		SkipCert:   q.Get("allowInsecure") == "1",
		UDP:        true,
		WSHeaders:  make(map[string]string),
	}

	if node.Network == "" {
		node.Network = "tcp"
	}

	if node.Network == "ws" {
		node.WSPath = q.Get("path")
		if hostHeader := q.Get("host"); hostHeader != "" {
			node.WSHeaders["Host"] = hostHeader
		}
	}

	if node.ServerName == "" {
		node.ServerName = host
	}

	return node
}

// parseHysteria2 解析 Hysteria2 链接
func parseHysteria2(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := u.User.Username()
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		portStr = "443"
	}
	port := parsePort(portStr)

	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = host
	}

	return &Node{
		Name:     name,
		Type:     "hysteria2",
		Server:   host,
		Port:     port,
		Password: password,
		TLS:      true,
		ServerName: q.Get("sni"),
		SkipCert: q.Get("insecure") == "1",
		UDP:      true,
		Opts: map[string]string{
			"obfs":          q.Get("obfs"),
			"obfs-password": q.Get("obfs-password"),
		},
	}
}

// parseShadowsocks 解析 Shadowsocks 链接
func parseShadowsocks(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		// 尝试 base64 解码
		encoded := strings.TrimPrefix(raw, "ss://")
		decoded, err := base64.URLEncoding.DecodeString(addBase64Padding(encoded))
		if err != nil {
			return nil
		}
		u, err = url.Parse("ss://" + string(decoded))
		if err != nil {
			return nil
		}
	}

	// 解析用户信息 (method:password)
	userInfo := u.User.String()
	var method, password string
	if idx := strings.Index(userInfo, ":"); idx > 0 {
		method = userInfo[:idx]
		password = userInfo[idx+1:]
	} else {
		// 可能是 base64 编码的
		decoded, err := base64.URLEncoding.DecodeString(addBase64Padding(userInfo))
		if err != nil {
			return nil
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return nil
		}
		method = parts[0]
		password = parts[1]
	}

	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		portStr = "8388"
	}
	port := parsePort(portStr)

	name := u.Fragment
	if name == "" {
		name = host
	}

	return &Node{
		Name:     name,
		Type:     "ss",
		Server:   host,
		Port:     port,
		Password: password,
		Cipher:   method,
		UDP:      true,
	}
}

// parseAnyTLS 解析 AnyTLS 链接
// 格式: anytls://password@server:port#name?sni=example.com
func parseAnyTLS(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port := parsePort(portStr)

	name := u.Fragment
	if name == "" {
		name = host
	}

	sni := u.Query().Get("sni")
	if sni == "" {
		sni = host
	}

	return &Node{
		Name:       name,
		Type:       "anytls",
		Server:     host,
		Port:       port,
		Password:   password,
		TLS:        true,
		ServerName: sni,
	}
}

// parseMASQUE 解析 MASQUE 链接
// 格式: masque://password@server:port#name?sni=example.com
func parseMASQUE(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port := parsePort(portStr)

	name := u.Fragment
	if name == "" {
		name = host
	}

	sni := u.Query().Get("sni")
	if sni == "" {
		sni = host
	}

	return &Node{
		Name:       name,
		Type:       "masque",
		Server:     host,
		Port:       port,
		Password:   password,
		TLS:        true,
		ServerName: sni,
		UDP:        true,
	}
}

// parseTrustTunnel 解析 TrustTunnel 链接
// 格式: trust-tunnel://password@server:port#name?sni=example.com&mode=ws&path=/tunnel
func parseTrustTunnel(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port := parsePort(portStr)

	name := u.Fragment
	if name == "" {
		name = host
	}

	sni := u.Query().Get("sni")
	if sni == "" {
		sni = host
	}

	return &Node{
		Name:       name,
		Type:       "trust-tunnel",
		Server:     host,
		Port:       port,
		Password:   password,
		TLS:        true,
		ServerName: sni,
	}
}

// parseSudoku 解析 Sudoku 链接
// 格式: sudoku://password@server:port#name
func parseSudoku(raw string) *Node {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	password := ""
	if u.User != nil {
		password = u.User.Username()
	}

	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port := parsePort(portStr)

	name := u.Fragment
	if name == "" {
		name = host
	}

	return &Node{
		Name:     name,
		Type:     "sudoku",
		Server:   host,
		Port:     port,
		Password: password,
	}
}

// parsePort 解析端口
func parsePort(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case string:
		var port int
		fmt.Sscanf(val, "%d", &port)
		return port
	case int:
		return val
	default:
		return 0
	}
}

// parseIntAny 解析整数
func parseIntAny(v any) int {
	return parsePort(v)
}

// addBase64Padding 添加 base64 padding
func addBase64Padding(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

// ToConfigProxies 将节点转换为配置格式
func (n *Node) ToConfigProxies() map[string]any {
	proxy := map[string]any{
		"name":   n.Name,
		"type":   n.Type,
		"server": n.Server,
		"port":   n.Port,
	}

	switch n.Type {
	case "vmess":
		proxy["uuid"] = n.UUID
		proxy["alter-id"] = n.AlterID
		proxy["cipher"] = n.Cipher
		proxy["tls"] = n.TLS
		proxy["skip-cert-verify"] = n.SkipCert
		proxy["servername"] = n.ServerName
		proxy["network"] = n.Network
		if n.Network == "ws" && n.WSPath != "" {
			proxy["ws-path"] = n.WSPath
			if len(n.WSHeaders) > 0 {
				proxy["ws-headers"] = n.WSHeaders
			}
		}

	case "vless":
		proxy["uuid"] = n.UUID
		proxy["tls"] = n.TLS
		proxy["skip-cert-verify"] = n.SkipCert
		proxy["servername"] = n.ServerName
		proxy["network"] = n.Network
		if n.Network == "ws" && n.WSPath != "" {
			proxy["ws-path"] = n.WSPath
			if len(n.WSHeaders) > 0 {
				proxy["ws-headers"] = n.WSHeaders
			}
		}

	case "trojan":
		proxy["password"] = n.Password
		proxy["tls"] = true
		proxy["skip-cert-verify"] = n.SkipCert
		proxy["sni"] = n.ServerName
		proxy["network"] = n.Network
		if n.Network == "ws" && n.WSPath != "" {
			proxy["ws-path"] = n.WSPath
			if len(n.WSHeaders) > 0 {
				proxy["ws-headers"] = n.WSHeaders
			}
		}

	case "hysteria2":
		proxy["password"] = n.Password
		proxy["tls"] = true
		proxy["skip-cert-verify"] = n.SkipCert
		proxy["sni"] = n.ServerName

	case "ss":
		proxy["password"] = n.Password
		proxy["cipher"] = n.Cipher

	case "anytls":
		proxy["password"] = n.Password
		proxy["sni"] = n.ServerName
		proxy["skip-cert-verify"] = n.SkipCert

	case "masque":
		proxy["password"] = n.Password
		proxy["sni"] = n.ServerName
		proxy["skip-cert-verify"] = n.SkipCert

	case "trust-tunnel":
		proxy["password"] = n.Password
		proxy["sni"] = n.ServerName
		proxy["skip-cert-verify"] = n.SkipCert

	case "sudoku":
		proxy["password"] = n.Password
	}

	if n.UDP {
		proxy["udp"] = true
	}

	return proxy
}

// SubscriptionInfo 订阅信息响应
type SubscriptionInfo struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Interval   int       `json:"interval"`
	AutoUpdate bool      `json:"auto_update"`
	LastUpdate time.Time `json:"last_update"`
	NodeCount  int       `json:"node_count"`
	// 流量信息
	Upload     int64     `json:"upload"`     // 已上传（字节）
	Download   int64     `json:"download"`   // 已下载（字节）
	Total      int64     `json:"total"`      // 总流量（字节）
	Expire     int64     `json:"expire"`     // 到期时间戳
	// 计算字段
	Used       int64     `json:"used"`       // 已用流量
	Remaining  int64     `json:"remaining"`  // 剩余流量
	UsagePercent float64 `json:"usage_percent"` // 使用百分比
}

// GetInfo 获取订阅信息
func (s *Subscription) GetInfo() SubscriptionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	used := s.Upload + s.Download
	remaining := s.Total - used
	usagePercent := float64(0)
	if s.Total > 0 {
		usagePercent = float64(used) / float64(s.Total) * 100
	}

	return SubscriptionInfo{
		Name:         s.Name,
		URL:          s.URL,
		Interval:     int(s.Interval.Seconds()),
		AutoUpdate:   s.AutoUpdate,
		LastUpdate:   s.LastUpdate,
		NodeCount:    len(s.Nodes),
		Upload:       s.Upload,
		Download:     s.Download,
		Total:        s.Total,
		Expire:       s.Expire,
		Used:         used,
		Remaining:    remaining,
		UsagePercent: usagePercent,
	}
}
