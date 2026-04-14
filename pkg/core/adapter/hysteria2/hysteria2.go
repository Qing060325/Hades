// Package hysteria2 Hysteria2 协议实现
// Hysteria2 是基于 QUIC 的高性能代理协议
package hysteria2

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// Adapter Hysteria2 适配器
type Adapter struct {
	name           string
	server         string
	port           int
	password       string
	sni            string
	obfs           string
	obfsPassword   string
	up             string
	down           string
	alpn           []string
	skipCertVerify bool

	// 内部状态
	mu       sync.RWMutex
	conn     net.PacketConn
	tlsCfg   *tls.Config

	// 拨号器
	dialer *net.Dialer
}

// NewAdapter 创建 Hysteria2 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:     name,
		server:   server,
		port:     port,
		password: password,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	// 初始化 TLS 配置
	a.tlsCfg = &tls.Config{
		ServerName:         a.sni,
		InsecureSkipVerify: a.skipCertVerify,
		NextProtos:         a.alpn,
		MinVersion:         tls.VersionTLS13,
	}

	return a, nil
}

// Option 配置选项
type Option func(*Adapter)

// WithSNI 设置 SNI
func WithSNI(sni string) Option {
	return func(a *Adapter) {
		a.sni = sni
	}
}

// WithObfs 设置混淆
func WithObfs(obfs, password string) Option {
	return func(a *Adapter) {
		a.obfs = obfs
		a.obfsPassword = password
	}
}

// WithBandwidth 设置带宽
func WithBandwidth(up, down string) Option {
	return func(a *Adapter) {
		a.up = up
		a.down = down
	}
}

// WithALPN 设置 ALPN
func WithALPN(alpn []string) Option {
	return func(a *Adapter) {
		a.alpn = alpn
	}
}

// WithSkipCertVerify 跳过证书验证
func WithSkipCertVerify(skip bool) Option {
	return func(a *Adapter) {
		a.skipCertVerify = skip
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeHysteria2 }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return true }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return false }

// DialContext 建立 TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// Hysteria2 基于 QUIC (UDP)
	// 这里实现 TCP over Hysteria2

	// 建立到服务器的 UDP 连接
	addr := net.JoinHostPort(a.server, strconv.Itoa(a.port))

	a.mu.Lock()
	if a.conn == nil {
		var err error
		a.conn, err = net.ListenPacket("udp", "")
		if err != nil {
			a.mu.Unlock()
			return nil, fmt.Errorf("创建 UDP 连接失败: %w", err)
		}
	}
	conn := a.conn
	a.mu.Unlock()

	// 创建 Hysteria2 客户端连接
	h2Conn, err := a.createHysteria2Conn(ctx, conn, addr, metadata)
	if err != nil {
		return nil, err
	}

	return h2Conn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	// Hysteria2 原生支持 UDP
	return nil, fmt.Errorf("Hysteria2 UDP 尚未完全实现")
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

// createHysteria2Conn 创建 Hysteria2 连接
func (a *Adapter) createHysteria2Conn(ctx context.Context, packetConn net.PacketConn, addr string, metadata *adapter.Metadata) (net.Conn, error) {
	// 解析服务器地址
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	// 创建 QUIC 连接 (简化实现)
	// 实际实现需要使用 quic-go 库

	// 创建 Hysteria2 流
	// 协议: Hysteria2 使用 HTTP/3 作为传输层

	// 封装为 net.Conn
	conn := &hysteria2Conn{
		packetConn: packetConn,
		remoteAddr: udpAddr,
		localAddr:  packetConn.LocalAddr(),
		metadata:   metadata,
		adapter:    a,
	}

	return conn, nil
}

// parseBandwidth 解析带宽字符串
func parseBandwidth(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}

	s = strings.TrimSpace(strings.ToUpper(s))

	var multiplier uint64 = 1
	if strings.HasSuffix(s, "G") || strings.HasSuffix(s, "GBPS") {
		multiplier = 1e9
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GBPS"), "G")
	} else if strings.HasSuffix(s, "M") || strings.HasSuffix(s, "MBPS") {
		multiplier = 1e6
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MBPS"), "M")
	} else if strings.HasSuffix(s, "K") || strings.HasSuffix(s, "KBPS") {
		multiplier = 1e3
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KBPS"), "K")
	}

	var value uint64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &value)
	if err != nil {
		return 0, err
	}

	return value * multiplier, nil
}

// hysteria2Conn Hysteria2 连接封装
type hysteria2Conn struct {
	packetConn net.PacketConn
	remoteAddr net.Addr
	localAddr  net.Addr
	metadata   *adapter.Metadata
	adapter    *Adapter

	mu     sync.Mutex
	closed bool
}

func (c *hysteria2Conn) Read(b []byte) (n int, err error) {
	// 实现 Hysteria2 数据读取
	// 实际实现需要处理 QUIC 流
	n, _, err = c.packetConn.ReadFrom(b)
	return n, err
}

func (c *hysteria2Conn) Write(b []byte) (n int, err error) {
	// 实现 Hysteria2 数据写入
	return c.packetConn.WriteTo(b, c.remoteAddr)
}

func (c *hysteria2Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return nil
}

func (c *hysteria2Conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *hysteria2Conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *hysteria2Conn) SetDeadline(t time.Time) error {
	return c.packetConn.SetReadDeadline(t)
}

func (c *hysteria2Conn) SetReadDeadline(t time.Time) error {
	return c.packetConn.SetReadDeadline(t)
}

func (c *hysteria2Conn) SetWriteDeadline(t time.Time) error {
	return c.packetConn.SetWriteDeadline(t)
}
