// Package gostplugin GOST Plugin 传输实现
// GOST Plugin 提供 WebSocket 和 gRPC 隧道传输
package gostplugin

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// TunnelType 隧道类型
type TunnelType string

const (
	TunnelWebSocket TunnelType = "websocket" // WebSocket 隧道
	TunnelGRPC      TunnelType = "grpc"      // gRPC 隧道
)

// Config GOST Plugin 配置
type Config struct {
	Type     TunnelType `yaml:"type"`          // 隧道类型 (websocket/grpc)
	Host     string     `yaml:"host"`          // 主机名
	Path     string     `yaml:"path"`          // 路径
	ServerName string   `yaml:"server-name"`   // TLS 服务器名称
	Insecure bool       `yaml:"insecure"`      // 跳过证书验证
}

// Conn GOST Plugin 连接封装
type Conn struct {
	net.Conn
	config    *Config
	handshook bool
}

// NewConn 创建 GOST Plugin 连接
func NewConn(conn net.Conn, cfg *Config) *Conn {
	if cfg.Path == "" {
		cfg.Path = "/gost"
	}
	if !strings.HasPrefix(cfg.Path, "/") {
		cfg.Path = "/" + cfg.Path
	}

	return &Conn{
		Conn:   conn,
		config: cfg,
	}
}

// Dial 建立 GOST Plugin 连接
func Dial(ctx context.Context, addr string, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		cfg = &Config{Type: TunnelWebSocket, Path: "/gost"}
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("GOST Plugin TCP 连接失败: %w", err)
	}

	gostConn := NewConn(conn, cfg)

	// 执行握手
	if err := gostConn.handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("GOST Plugin 握手失败: %w", err)
	}

	return gostConn, nil
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

// handshake 执行 GOST Plugin 握手
func (c *Conn) handshake() error {
	if c.handshook {
		return nil
	}

	switch c.config.Type {
	case TunnelWebSocket:
		return c.wsHandshake()
	case TunnelGRPC:
		return c.grpcHandshake()
	default:
		return fmt.Errorf("不支持的 GOST 隧道类型: %s", c.config.Type)
	}
}

// wsHandshake WebSocket 隧道握手
func (c *Conn) wsHandshake() error {
	host := c.config.Host
	if host == "" {
		host = c.Conn.RemoteAddr().String()
	}
	if c.config.ServerName != "" {
		host = c.config.ServerName
	}

	// 构建 WebSocket 握手请求
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", c.config.Path)
	req += fmt.Sprintf("Host: %s\r\n", host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"
	req += "Sec-WebSocket-Key: R29zdFBsdGdlbiBLZXk=\r\n"
	req += "Sec-WebSocket-Version: 13\r\n"
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

// grpcHandshake gRPC 隧道握手
func (c *Conn) grpcHandshake() error {
	host := c.config.Host
	if host == "" {
		host = c.Conn.RemoteAddr().String()
	}
	if c.config.ServerName != "" {
		host = c.config.ServerName
	}

	serviceName := strings.TrimPrefix(c.config.Path, "/")
	if serviceName == "" {
		serviceName = "gost"
	}

	// 构建 HTTP/2 PRI 请求 (gRPC 基于 HTTP/2)
	req := "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	if _, err := c.Conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("发送 gRPC 请求失败: %w", err)
	}

	// 发送 gRPC 请求头
	// 实际实现需要完整的 HTTP/2 帧处理
	headers := fmt.Sprintf(":method: POST\r\n:scheme: https\r\n:authority: %s\r\n:path: /%s\r\ncontent-type: application/grpc\r\n\r\n", host, serviceName)
	if _, err := c.Conn.Write([]byte(headers)); err != nil {
		return fmt.Errorf("发送 gRPC 头部失败: %w", err)
	}

	c.handshook = true
	return nil
}

// WrapListener 包装 Listener 为 GOST Plugin 监听器
func WrapListener(ln net.Listener, cfg *Config) net.Listener {
	if cfg == nil {
		cfg = &Config{Type: TunnelWebSocket, Path: "/gost"}
	}
	return &gostListener{
		Listener: ln,
		config:   cfg,
	}
}

// gostListener GOST Plugin 监听器
type gostListener struct {
	net.Listener
	config *Config
}

// Accept 接受连接
func (l *gostListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// 读取客户端请求
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		return nil, err
	}

	reqStr := string(buf[:n])

	switch l.config.Type {
	case TunnelWebSocket:
		if strings.Contains(reqStr, "Upgrade: websocket") {
			// WebSocket 升级响应
			resp := "HTTP/1.1 101 Switching Protocols\r\n"
			resp += "Upgrade: websocket\r\n"
			resp += "Connection: Upgrade\r\n"
			resp += "Sec-WebSocket-Accept: R29zdFBsdGdlbiBSZXNwb25zZQ==\r\n"
			resp += "\r\n"
			if _, err := conn.Write([]byte(resp)); err != nil {
				conn.Close()
				return nil, err
			}
		}
	case TunnelGRPC:
		// gRPC 隧道直接透传
		_ = reqStr
	}

	return NewConn(conn, l.config), nil
}
