// cmd/sub2config/main.go - 订阅链接转配置文件工具
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: sub2config <订阅URL或文件> <输出config.yaml路径>")
		fmt.Fprintln(os.Stderr, "  也可用 stdin: sub2config - output.yaml")
		os.Exit(1)
	}

	subURL := os.Args[1]
	outPath := os.Args[2]

	var content string

	switch {
	case subURL == "-":
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		content = strings.Join(lines, "\n")

	case strings.HasPrefix(subURL, "http://") || strings.HasPrefix(subURL, "https://"):
		resp, err := httpGet(subURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取订阅失败: %v\n", err)
			os.Exit(1)
		}
		content = resp

	default:
		data, err := os.ReadFile(subURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
			os.Exit(1)
		}
		content = string(data)
	}

	// 尝试 base64 解码
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decoded = []byte(content)
	}

	text := strings.TrimSpace(string(decoded))
	nodes := parseSubscription(text)

	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, "未解析到任何节点")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "解析到 %d 个节点\n", len(nodes))
	generateConfig(nodes, outPath)
	fmt.Fprintf(os.Stderr, "配置已写入 %s\n", outPath)
}

// ProxyNode 代理节点
type ProxyNode struct {
	Name            string
	Type            string
	Server          string
	Port            int
	UUID            string
	AlterID         int
	Password        string
	Network         string
	TLS             bool
	SNI             string
	WSPath          string
	WSHost          string
	Alpn            []string
	Insecure        bool
	Hy2Obfs         string
	Hy2ObfsPassword string
}

func parseSubscription(text string) []ProxyNode {
	lines := strings.Split(text, "\n")
	var nodes []ProxyNode

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var node *ProxyNode
		switch {
		case strings.HasPrefix(line, "vmess://"):
			node = parseVMess(line)
		case strings.HasPrefix(line, "trojan://"):
			node = parseTrojan(line)
		case strings.HasPrefix(line, "hysteria2://") || strings.HasPrefix(line, "hy2://"):
			node = parseHysteria2(line)
		}

		if node != nil {
			nodes = append(nodes, *node)
		}
	}

	return nodes
}

func parseVMess(raw string) *ProxyNode {
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(addBase64Padding(encoded))
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解码 vmess 失败: %v\n", err)
			return nil
		}
	}

	var vmess struct {
		V    string `json:"v"`
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		AID  any    `json:"aid"`
		Net  string `json:"net"`
		Type string `json:"type"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
	}

	if err := json.Unmarshal(decoded, &vmess); err != nil {
		fmt.Fprintf(os.Stderr, "解析 vmess JSON 失败: %v\n", err)
		return nil
	}

	name := vmess.PS
	if name == "" {
		name = vmess.Add
	}

	var port int
	switch v := vmess.Port.(type) {
	case float64:
		port = int(v)
	case string:
		fmt.Sscanf(v, "%d", &port)
	case int:
		port = v
	}

	var alterID int
	switch v := vmess.AID.(type) {
	case float64:
		alterID = int(v)
	case string:
		fmt.Sscanf(v, "%d", &alterID)
	case int:
		alterID = v
	}

	return &ProxyNode{
		Name:     name,
		Type:     "vmess",
		Server:   vmess.Add,
		Port:     port,
		UUID:     vmess.ID,
		AlterID:  alterID,
		Network:  vmess.Net,
		TLS:      vmess.TLS == "tls",
		SNI:      vmess.SNI,
		WSPath:   vmess.Path,
		WSHost:   vmess.Host,
		Insecure: true,
	}
}

func parseTrojan(raw string) *ProxyNode {
	u, err := url.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 trojan URL 失败: %v\n", err)
		return nil
	}

	password := u.User.Username()
	host, portStr, err := netSplitHostPort(u.Host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 trojan 地址失败: %v\n", err)
		return nil
	}
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = host
	}

	allowInsecure := q.Get("allowInsecure") == "1"
	sni := q.Get("sni")
	if sni == "" {
		sni = host
	}

	return &ProxyNode{
		Name:     name,
		Type:     "trojan",
		Server:   host,
		Port:     port,
		Password: password,
		TLS:      true,
		SNI:      sni,
		Insecure: allowInsecure,
		Network:  "tcp",
	}
}

func parseHysteria2(raw string) *ProxyNode {
	u, err := url.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 hysteria2 URL 失败: %v\n", err)
		return nil
	}

	password := u.User.Username()
	host, portStr, err := netSplitHostPort(u.Host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 hysteria2 地址失败: %v\n", err)
		return nil
	}
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	q := u.Query()
	name := u.Fragment
	if name == "" {
		name = host
	}

	return &ProxyNode{
		Name:            name,
		Type:            "hysteria2",
		Server:          host,
		Port:            port,
		Password:        password,
		TLS:             true,
		SNI:             q.Get("sni"),
		Insecure:        q.Get("insecure") == "1",
		Network:         "udp",
		Hy2Obfs:         q.Get("obfs"),
		Hy2ObfsPassword: q.Get("obfs-password"),
	}
}

func generateConfig(nodes []ProxyNode, outPath string) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	var proxyYAML strings.Builder
	proxyYAML.WriteString("proxies:\n")

	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		name := sanitizeName(n.Name)
		names = append(names, name)

		switch n.Type {
		case "vmess":
			proxyYAML.WriteString(fmt.Sprintf("  - name: \"%s\"\n", name))
			proxyYAML.WriteString("    type: vmess\n")
			proxyYAML.WriteString(fmt.Sprintf("    server: %s\n", n.Server))
			proxyYAML.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			proxyYAML.WriteString(fmt.Sprintf("    uuid: %s\n", n.UUID))
			proxyYAML.WriteString(fmt.Sprintf("    alter-id: %d\n", n.AlterID))
			proxyYAML.WriteString("    cipher: auto\n")
			proxyYAML.WriteString(fmt.Sprintf("    tls: %v\n", n.TLS))
			proxyYAML.WriteString(fmt.Sprintf("    skip-cert-verify: %v\n", n.Insecure))
			proxyYAML.WriteString(fmt.Sprintf("    network: %s\n", n.Network))
			if n.WSPath != "" {
				proxyYAML.WriteString(fmt.Sprintf("    ws-path: %s\n", n.WSPath))
				proxyYAML.WriteString("    ws-headers:\n")
				proxyYAML.WriteString(fmt.Sprintf("      Host: %s\n", n.WSHost))
			}
			if n.SNI != "" {
				proxyYAML.WriteString(fmt.Sprintf("    servername: %s\n", n.SNI))
			}

		case "trojan", "hysteria2":
			proxyYAML.WriteString(fmt.Sprintf("  - name: \"%s\"\n", name))
			if n.Type == "hysteria2" {
				proxyYAML.WriteString("    type: trojan  # hysteria2 fallback\n")
			} else {
				proxyYAML.WriteString("    type: trojan\n")
			}
			proxyYAML.WriteString(fmt.Sprintf("    server: %s\n", n.Server))
			proxyYAML.WriteString(fmt.Sprintf("    port: %d\n", n.Port))
			proxyYAML.WriteString(fmt.Sprintf("    password: \"%s\"\n", n.Password))
			proxyYAML.WriteString("    udp: true\n")
			proxyYAML.WriteString(fmt.Sprintf("    sni: %s\n", n.SNI))
			proxyYAML.WriteString(fmt.Sprintf("    skip-cert-verify: %v\n", n.Insecure))
		}
	}

	var config strings.Builder

	config.WriteString("# 由 sub2config 自动生成\n\n")
	config.WriteString("mixed-port: 7890\n")
	config.WriteString("allow-lan: true\n")
	config.WriteString("bind-address: \"*\"\n")
	config.WriteString("mode: rule\n")
	config.WriteString("log-level: info\n\n")

	config.WriteString("external-controller: 127.0.0.1:9090\n")
	config.WriteString("secret: \"\"\n\n")

	config.WriteString("dns:\n")
	config.WriteString("  enable: true\n")
	config.WriteString("  listen: 0.0.0.0:1053\n")
	config.WriteString("  enhanced-mode: fake-ip\n")
	config.WriteString("  fake-ip-range: 198.18.0.1/16\n")
	config.WriteString("  nameserver:\n")
	config.WriteString("    - https://dns.alidns.com/dns-query\n")
	config.WriteString("    - https://doh.pub/dns-query\n\n")

	config.WriteString(proxyYAML.String())
	config.WriteString("\n")

	config.WriteString("proxy-groups:\n")
	config.WriteString("  - name: \"proxy\"\n")
	config.WriteString("    type: select\n")
	config.WriteString("    proxies:\n")
	config.WriteString("      - auto\n")
	config.WriteString("      - DIRECT\n\n")

	config.WriteString("  - name: \"auto\"\n")
	config.WriteString("    type: url-test\n")
	config.WriteString("    url: \"https://www.gstatic.com/generate_204\"\n")
	config.WriteString("    interval: 300\n")
	config.WriteString("    tolerance: 50\n")
	config.WriteString("    timeout: 5000\n")
	config.WriteString("    proxies:\n")

	maxNodes := len(names)
	if maxNodes > 25 {
		maxNodes = 25
	}
	for _, name := range names[:maxNodes] {
		config.WriteString(fmt.Sprintf("      - \"%s\"\n", name))
	}
	config.WriteString("\n")

	config.WriteString("rules:\n")
	config.WriteString("  - DOMAIN-SUFFIX,cn,DIRECT\n")
	config.WriteString("  - DOMAIN-KEYWORD,baidu,DIRECT\n")
	config.WriteString("  - DOMAIN-KEYWORD,taobao,DIRECT\n")
	config.WriteString("  - DOMAIN-KEYWORD,alipay,DIRECT\n")
	config.WriteString("  - DOMAIN-KEYWORD,bilibili,DIRECT\n")
	config.WriteString("  - DOMAIN-SUFFIX,qq.com,DIRECT\n")
	config.WriteString("  - DOMAIN-SUFFIX,zhihu.com,DIRECT\n")
	config.WriteString("  - GEOIP,CN,DIRECT\n")
	config.WriteString("  - GEOIP,LAN,DIRECT\n")
	config.WriteString("  - DOMAIN-SUFFIX,google.com,proxy\n")
	config.WriteString("  - DOMAIN-SUFFIX,youtube.com,proxy\n")
	config.WriteString("  - DOMAIN-SUFFIX,github.com,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,google,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,facebook,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,twitter,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,telegram,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,openai,proxy\n")
	config.WriteString("  - DOMAIN-KEYWORD,chatgpt,proxy\n")
	config.WriteString("  - DOMAIN-SUFFIX,openai.com,proxy\n")
	config.WriteString("  - GEOIP,US,proxy\n")
	config.WriteString("  - GEOIP,JP,proxy\n")
	config.WriteString("  - GEOIP,HK,proxy\n")
	config.WriteString("  - GEOIP,SG,proxy\n")
	config.WriteString("  - MATCH,proxy\n")

	os.WriteFile(outPath, []byte(config.String()), 0644)
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	r := strings.NewReplacer("\"", "'", ":", "_", "\\", "_", "\n", " ")
	return r.Replace(name)
}

func addBase64Padding(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

func netSplitHostPort(hostPort string) (string, string, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort, "0", nil
	}
	return host, port, nil
}

func httpGet(rawURL string) (string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
