// Package snell Snell 协议适配器实现
package snell

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// ObfsMode 混淆模式
type ObfsMode string

const (
	ObfsNone ObfsMode = ""
	ObfsTLS  ObfsMode = "tls"
	ObfsHTTP ObfsMode = "http"
)

// Adapter Snell 适配器
type Adapter struct {
	name     string
	server   string
	port     int
	psk      string // Pre-Shared Key
	version  int    // Snell 协议版本 (2/3/4)
	obfsMode ObfsMode
	obfsHost string // 混淆伪装域名
	udp      bool
	dialer   *net.Dialer
}

// Option 配置选项
type Option func(*Adapter)

// WithObfs 设置混淆模式
func WithObfs(mode, host string) Option {
	return func(a *Adapter) {
		a.obfsMode = ObfsMode(mode)
		a.obfsHost = host
	}
}

// WithVersion 设置协议版本
func WithVersion(version int) Option {
	return func(a *Adapter) {
		if version >= 2 && version <= 4 {
			a.version = version
		}
	}
}

// NewAdapter 创建 Snell 适配器
func NewAdapter(name, server string, port int, psk string, opts ...Option) (*Adapter, error) {
	if psk == "" {
		return nil, fmt.Errorf("snell: PSK 不能为空")
	}

	a := &Adapter{
		name:    name,
		server:  server,
		port:    port,
		psk:     psk,
		version: 3, // 默认 Snell v3
		udp:     false,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	// Snell v4 支持 UDP
	if a.version >= 4 {
		a.udp = true
	}

	return a, nil
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeSnell }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立 TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	target := metadata.DestinationAddress()

	log.Debug().
		Str("server", a.Addr()).
		Str("target", target).
		Int("version", a.version).
		Msg("[Snell] 建立连接")

	// 建立底层 TCP 连接
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("snell: 连接服务器失败: %w", err)
	}

	// 混淆层封装
	if a.obfsMode != ObfsNone {
		conn, err = a.wrapObfs(conn)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("snell: 混淆封装失败: %w", err)
		}
	}

	// Snell 握手
	if err := a.handshake(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("snell: 握手失败: %w", err)
	}

	// 发送连接请求
	if err := a.writeConnect(conn, target); err != nil {
		conn.Close()
		return nil, fmt.Errorf("snell: 发送连接请求失败: %w", err)
	}

	return &snellConnWrapper{
		Conn:   conn,
		psk:    a.psk,
		version: a.version,
	}, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	if !a.udp {
		return nil, fmt.Errorf("snell: 当前版本 (v%d) 不支持 UDP", a.version)
	}

	// Snell v4 UDP 通过 TCP 通道封装
	return nil, fmt.Errorf("snell: UDP 尚未实现")
}

// URLTest 健康检查
func (a *Adapter) URLTest(ctx context.Context, testURL string) (time.Duration, error) {
	start := time.Now()
	conn, err := a.DialContext(ctx, &adapter.Metadata{
		Host:    "www.gstatic.com",
		DstPort: 80,
	})
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}

// handshake Snell 握手协议
func (a *Adapter) handshake(conn net.Conn) error {
	// Snell 握手请求: version(1) + command(1) + padding_len(1) + padding
	hello := make([]byte, 6)
	hello[0] = byte(a.version)
	hello[1] = 0x00 // handshake command
	hello[2] = 0x00 // no padding
	hello[3] = 0x00
	hello[4] = 0x01
	hello[5] = 0x00

	if _, err := conn.Write(hello); err != nil {
		return fmt.Errorf("发送握手请求失败: %w", err)
	}

	// 读取握手响应
	resp := make([]byte, 6)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("读取握手响应失败: %w", err)
	}

	// 检查响应状态
	if resp[1] != 0x00 {
		return fmt.Errorf("握手被拒绝, 状态码: 0x%02x", resp[1])
	}

	log.Debug().Int("version", a.version).Msg("[Snell] 握手成功")
	return nil
}

// writeConnect 发送连接请求
func (a *Adapter) writeConnect(conn net.Conn, target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("解析目标地址失败: %w", err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("解析端口失败: %w", err)
	}

	// Snell 连接请求格式:
	// version(1) + command(1) + id(1) + payload_len(2) + payload
	// payload: addr_type(1) + host_len(1) + host + port(2)
	header := make([]byte, 5)
	header[0] = byte(a.version)
	header[1] = 0x01 // connect command
	header[2] = 0x00 // id

	// 构建目标地址
	targetBuf := make([]byte, 0, len(host)+4)
	targetBuf = append(targetBuf, 0x01) // 域名类型
	targetBuf = append(targetBuf, byte(len(host)))
	targetBuf = append(targetBuf, host...)
	targetBuf = append(targetBuf, byte(port>>8), byte(port))

	// 设置 payload 长度
	payloadLen := uint16(len(targetBuf))
	header[3] = byte(payloadLen >> 8)
	header[4] = byte(payloadLen)

	// 发送 header + payload
	full := make([]byte, len(header)+len(targetBuf))
	copy(full, header)
	copy(full[len(header):], targetBuf)

	if _, err := conn.Write(full); err != nil {
		return fmt.Errorf("发送连接请求失败: %w", err)
	}

	// 读取响应头
	respHeader := make([]byte, 5)
	if _, err := io.ReadFull(conn, respHeader); err != nil {
		return fmt.Errorf("读取连接响应失败: %w", err)
	}

	// 检查响应
	if respHeader[1] != 0x00 {
		return fmt.Errorf("连接被拒绝, 状态码: 0x%02x", respHeader[1])
	}

	log.Debug().
		Str("target", target).
		Msg("[Snell] 连接请求成功")

	return nil
}

// wrapObfs 混淆层封装
func (a *Adapter) wrapObfs(conn net.Conn) (net.Conn, error) {
	switch a.obfsMode {
	case ObfsTLS:
		// TLS 混淆: 建立 TLS 连接伪装成 HTTPS 流量
		serverName := a.obfsHost
		if serverName == "" {
			serverName = a.server
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.Handshake(); err != nil {
			return nil, fmt.Errorf("TLS 混淆握手失败: %w", err)
		}
		return tlsConn, nil

	case ObfsHTTP:
		// HTTP 混淆: 伪装成 HTTP 流量
		return newHTTPConn(conn, a.server, a.port, a.obfsHost), nil

	default:
		return conn, nil
	}
}

// snellConnWrapper Snell 连接包装器
type snellConnWrapper struct {
	net.Conn
	psk     string
	version int
}

// Read 读取数据
func (c *snellConnWrapper) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *snellConnWrapper) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

// httpConn HTTP 混淆连接
type httpConn struct {
	net.Conn
	host      string
	port      int
	obfsHost  string
	handshook bool
}

func newHTTPConn(conn net.Conn, host string, port int, obfsHost string) *httpConn {
	obfsName := obfsHost
	if obfsName == "" {
		obfsName = host
	}
	return &httpConn{
		Conn:     conn,
		host:     host,
		port:     port,
		obfsHost: obfsName,
	}
}

// Read 读取数据 (首次读取时完成 HTTP 握手)
func (c *httpConn) Read(b []byte) (int, error) {
	if !c.handshook {
		if err := c.serverHandshake(); err != nil {
			return 0, err
		}
		c.handshook = true
	}
	return c.Conn.Read(b)
}

// Write 写入数据 (首次写入时发送 HTTP 请求头)
func (c *httpConn) Write(b []byte) (int, error) {
	if !c.handshook {
		if err := c.clientHandshake(); err != nil {
			return 0, err
		}
		c.handshook = true
	}
	return c.Conn.Write(b)
}

// clientHandshake 客户端 HTTP 握手
func (c *httpConn) clientHandshake() error {
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36\r\n\r\n", c.obfsHost)
	_, err := c.Conn.Write([]byte(req))
	return err
}

// serverHandshake 服务端 HTTP 响应读取
func (c *httpConn) serverHandshake() error {
	// 读取 HTTP 响应直到遇到 \r\n\r\n
	buf := make([]byte, 4096)
	for {
		n, err := c.Conn.Read(buf)
		if err != nil {
			return err
		}
		// 查找 HTTP 头结束标记
		for i := 0; i < n-3; i++ {
			if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
				return nil
			}
		}
	}
}
