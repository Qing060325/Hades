// Package hysteria Hysteria v1 协议实现
// Hysteria v1 是基于 QUIC 的高性能代理协议，支持带宽控制和混淆
package hysteria

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
	"github.com/Qing060325/Hades/pkg/transport/hysteria"
)

// Adapter Hysteria v1 适配器
type Adapter struct {
	name           string
	server         string
	port           int
	password       string
	sni            string
	obfs           string
	up             string
	down           string
	alpn           []string
	skipCertVerify bool
	auth           []byte

	// 内部状态
	mu     sync.RWMutex
	client *hysteria.Client

	// 拨号器
	dialer *net.Dialer
}

// NewAdapter 创建 Hysteria v1 适配器
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

	// 初始化传输层客户端
	cfg := &hysteria.Config{
		Up:       a.up,
		Down:     a.down,
		Password: a.password,
		Host:     a.server,
		Port:     a.port,
		SNI:      a.sni,
		Obfs:     a.obfs,
		Auth:     a.auth,
	}
	a.client = hysteria.NewClient(cfg)

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
func WithObfs(obfs string) Option {
	return func(a *Adapter) {
		a.obfs = obfs
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

// WithAuth 设置认证数据
func WithAuth(auth []byte) Option {
	return func(a *Adapter) {
		a.auth = auth
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeHysteria }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return true }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return false }

// DialContext 建立 TCP 连接 (TCP over QUIC)
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	addr := net.JoinHostPort(a.server, strconv.Itoa(a.port))

	// 通过 QUIC 建立连接
	conn, err := a.client.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("连接 Hysteria 服务器失败: %w", err)
	}

	// 封装为 Hysteria 连接，附加目标元数据
	hConn := &hysteriaConn{
		Conn:     conn,
		metadata: metadata,
		adapter:  a,
	}

	return hConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	// Hysteria 原生支持 UDP over QUIC
	return nil, fmt.Errorf("Hysteria v1 UDP 尚未完全实现")
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

// hysteriaConn Hysteria 连接封装
type hysteriaConn struct {
	net.Conn
	metadata *adapter.Metadata
	adapter  *Adapter
	mu       sync.Mutex
	closed   bool
}

// Close 关闭连接
func (c *hysteriaConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Conn.Close()
}

// parseBandwidth 解析带宽字符串（如 "100mbps", "1g"）
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

// buildTLSConfig 构建 TLS 配置
func (a *Adapter) buildTLSConfig() *tls.Config {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}

	alpn := a.alpn
	if len(alpn) == 0 {
		alpn = []string{"hysteria"}
	}

	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: a.skipCertVerify,
		NextProtos:         alpn,
		MinVersion:         tls.VersionTLS13,
	}
}
