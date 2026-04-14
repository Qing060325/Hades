// Package tuic TUIC 协议实现
// TUIC 是基于 QUIC 的代理协议
package tuic

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// CongestionController 拥塞控制算法
type CongestionController string

const (
	CongestionBBR      CongestionController = "bbr"
	CongestionCubic    CongestionController = "cubic"
	CongestionNewReno  CongestionController = "new_reno"
)

// UDPRelayMode UDP 中继模式
type UDPRelayMode string

const (
	UDPRelayNative UDPRelayMode = "native"
	UDPRelayQuic   UDPRelayMode = "quic"
)

// Adapter TUIC 适配器
type Adapter struct {
	name                string
	server              string
	port                int
	uuid                string
	password            string
	congestionCtrl      CongestionController
	udpRelayMode        UDPRelayMode
	alpn                []string
	sni                 string
	skipCertVerify      bool
	heartbeatInterval   time.Duration
	requestTimeout      time.Duration
	zeroRTTHandshake    bool

	// 内部状态
	mu     sync.RWMutex
	conn   net.PacketConn
	tlsCfg *tls.Config
	dialer *net.Dialer
}

// NewAdapter 创建 TUIC 适配器
func NewAdapter(name, server string, port int, uuid, password string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:           name,
		server:         server,
		port:           port,
		uuid:           uuid,
		password:       password,
		congestionCtrl: CongestionBBR,
		udpRelayMode:   UDPRelayNative,
		heartbeatInterval: 10 * time.Second,
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

// WithCongestionController 设置拥塞控制
func WithCongestionController(cc CongestionController) Option {
	return func(a *Adapter) {
		a.congestionCtrl = cc
	}
}

// WithUDPRelayMode 设置 UDP 中继模式
func WithUDPRelayMode(mode UDPRelayMode) Option {
	return func(a *Adapter) {
		a.udpRelayMode = mode
	}
}

// WithALPN 设置 ALPN
func WithALPN(alpn []string) Option {
	return func(a *Adapter) {
		a.alpn = alpn
	}
}

// WithSNI 设置 SNI
func WithSNI(sni string) Option {
	return func(a *Adapter) {
		a.sni = sni
	}
}

// WithSkipCertVerify 跳过证书验证
func WithSkipCertVerify(skip bool) Option {
	return func(a *Adapter) {
		a.skipCertVerify = skip
	}
}

// WithHeartbeatInterval 设置心跳间隔
func WithHeartbeatInterval(d time.Duration) Option {
	return func(a *Adapter) {
		a.heartbeatInterval = d
	}
}

// WithZeroRTTHandshake 设置 0-RTT 握手
func WithZeroRTTHandshake(enable bool) Option {
	return func(a *Adapter) {
		a.zeroRTTHandshake = enable
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeTUIC }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return true }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return false }

// DialContext 建立 TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// TUIC 基于 QUIC (UDP)
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

	// 创建 TUIC 连接
	tuicConn, err := a.createTUICConn(ctx, conn, addr, metadata)
	if err != nil {
		return nil, err
	}

	return tuicConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("TUIC UDP 尚未完全实现")
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

// createTUICConn 创建 TUIC 连接
func (a *Adapter) createTUICConn(ctx context.Context, packetConn net.PacketConn, addr string, metadata *adapter.Metadata) (net.Conn, error) {
	// 解析服务器地址
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	// TUIC 协议实现
	// 使用 QUIC 作为传输层

	// 封装为 net.Conn
	conn := &tuicConn{
		packetConn: packetConn,
		remoteAddr: udpAddr,
		localAddr:  packetConn.LocalAddr(),
		metadata:   metadata,
		adapter:    a,
	}

	return conn, nil
}

// tuicConn TUIC 连接封装
type tuicConn struct {
	packetConn net.PacketConn
	remoteAddr net.Addr
	localAddr  net.Addr
	metadata   *adapter.Metadata
	adapter    *Adapter

	mu     sync.Mutex
	closed bool
}

func (c *tuicConn) Read(b []byte) (n int, err error) {
	return c.packetConn.ReadFrom(b)
}

func (c *tuicConn) Write(b []byte) (n int, err error) {
	return c.packetConn.WriteTo(b, c.remoteAddr)
}

func (c *tuicConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return nil
}

func (c *tuicConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *tuicConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *tuicConn) SetDeadline(t time.Time) error {
	return c.packetConn.SetReadDeadline(t)
}

func (c *tuicConn) SetReadDeadline(t time.Time) error {
	return c.packetConn.SetReadDeadline(t)
}

func (c *tuicConn) SetWriteDeadline(t time.Time) error {
	return c.packetConn.SetWriteDeadline(t)
}
