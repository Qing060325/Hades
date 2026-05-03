// Package v2rayplugin V2Ray Plugin 传输实现
// V2Ray Plugin 通过 WebSocket 和 TLS 进行流量混淆
package v2rayplugin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Mode 传输模式
type Mode string

const (
	ModeWebSocket Mode = "websocket" // WebSocket 模式
)

// Config V2Ray Plugin 配置
type Config struct {
	Mode         Mode   `yaml:"mode"`           // 传输模式 (websocket)
	Host         string `yaml:"host"`           // TLS SNI / Host
	Path         string `yaml:"path"`           // WebSocket 路径
	TLS          bool   `yaml:"tls"`            // 是否启用 TLS
	Mux          int    `yaml:"mux"`            // 多路复用连接数 (0=禁用)
	SkipCertVerify bool `yaml:"skip-cert-verify"` // 跳过证书验证
}

// Conn V2Ray Plugin 连接封装
type Conn struct {
	net.Conn
	config    *Config
	handshook bool
}

// NewConn 创建 V2Ray Plugin 连接
func NewConn(conn net.Conn, cfg *Config) *Conn {
	// 设置默认值
	if cfg.Mode == "" {
		cfg.Mode = ModeWebSocket
	}
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	// 确保路径以 / 开头
	if !strings.HasPrefix(cfg.Path, "/") {
		cfg.Path = "/" + cfg.Path
	}

	return &Conn{
		Conn:   conn,
		config: cfg,
	}
}

// Dial 建立 V2Ray Plugin 连接
func Dial(ctx context.Context, addr string, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		cfg = &Config{Mode: ModeWebSocket, Path: "/"}
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("V2Ray Plugin TCP 连接失败: %w", err)
	}

	pluginConn := NewConn(conn, cfg)

	// 执行握手
	if err := pluginConn.handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("V2Ray Plugin 握手失败: %w", err)
	}

	return pluginConn, nil
}

// Read 读取数据
func (c *Conn) Read(b []byte) (int, error) {
	if !c.handshook {
		if err := c.handshake(); err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *Conn) Write(b []byte) (int, error) {
	if !c.handshook {
		if err := c.handshake(); err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(b)
}

// handshake 执行 V2Ray Plugin 握手
func (c *Conn) handshake() error {
	if c.handshook {
		return nil
	}

	switch c.config.Mode {
	case ModeWebSocket:
		return c.wsHandshake()
	default:
		return fmt.Errorf("不支持的 V2Ray Plugin 模式: %s", c.config.Mode)
	}
}

// wsHandshake WebSocket 模式握手
func (c *Conn) wsHandshake() error {
	host := c.config.Host
	if host == "" {
		host = c.Conn.RemoteAddr().String()
	}

	// 构建 WebSocket 握手请求
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", c.config.Path)
	req += fmt.Sprintf("Host: %s\r\n", host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"

	// 添加 V2Ray Plugin 特有的头部
	req += "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"
	req += "Sec-WebSocket-Version: 13\r\n"

	// 自定义 headers（用于伪装）
	if c.config.TLS {
		req += fmt.Sprintf("Origin: https://%s\r\n", host)
	}

	req += "\r\n"

	if _, err := c.Conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("发送 WebSocket 请求失败: %w", err)
	}

	// 读取服务器响应
	buf := make([]byte, 4096)
	n, err := c.Conn.Read(buf)
	if err != nil {
		return fmt.Errorf("读取 WebSocket 响应失败: %w", err)
	}

	resp := string(buf[:n])
	if !strings.Contains(resp, "101") {
		return fmt.Errorf("WebSocket 握手失败: %s", resp)
	}

	c.handshook = true
	return nil
}

// WrapListener 包装 Listener 为 V2Ray Plugin 监听器
func WrapListener(ln net.Listener, cfg *Config) net.Listener {
	if cfg == nil {
		cfg = &Config{Mode: ModeWebSocket, Path: "/"}
	}
	return &pluginListener{
		Listener: ln,
		config:   cfg,
	}
}

// pluginListener V2Ray Plugin 监听器
type pluginListener struct {
	net.Listener
	config *Config
}

// Accept 接受连接
func (l *pluginListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// 服务端需要先读取 HTTP 请求
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		return nil, err
	}

	reqStr := string(buf[:n])

	// 解析 HTTP 请求获取路径
	if strings.Contains(reqStr, "Upgrade: websocket") {
		// 发送 WebSocket 升级响应
		resp := "HTTP/1.1 101 Switching Protocols\r\n"
		resp += "Upgrade: websocket\r\n"
		resp += "Connection: Upgrade\r\n"
		resp += "Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n"
		resp += "\r\n"

		if _, err := conn.Write([]byte(resp)); err != nil {
			conn.Close()
			return nil, err
		}
	} else {
		// 普通 HTTP 请求，返回 200
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
		conn.Write([]byte(resp))
	}

	return NewConn(conn, l.config), nil
}

// wrapTransport 包装 http.Transport 以支持 TLS 跳过验证
func wrapTransport(cfg *Config) *http.Transport {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	// TODO: 如果 TLS 启用且 SkipCertVerify 为 true，配置 TLS 跳过验证
	// transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.SkipCertVerify}

	return transport
}
