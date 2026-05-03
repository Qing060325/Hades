// Package masque MASQUE 协议实现 (RFC 9298)
// MASQUE 基于 HTTP/3 (QUIC) 协议，支持代理 UDP 和 IP 数据包
package masque

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	masqueTransport "github.com/Qing060325/Hades/pkg/transport/masque"
	"github.com/rs/zerolog/log"
)

const (
	// defaultPath 默认 HTTP/3 路径
	defaultPath = "/masque"
	// dialTimeout 连接超时
	dialTimeout = 30 * time.Second
	// keepAliveInterval 保活间隔
	keepAliveInterval = 30 * time.Second
)

// Adapter MASQUE 适配器
type Adapter struct {
	name           string
	server         string
	port           int
	password       string
	sni            string
	path           string
	allowInsecure  bool
	supportUDP     bool
	dialer         *net.Dialer
	client         *masqueTransport.Client
	connMu         sync.Mutex
	conn           net.Conn
}

// NewAdapter 创建 MASQUE 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:       name,
		server:     server,
		port:       port,
		password:   password,
		path:       defaultPath,
		supportUDP: true, // MASQUE 原生支持 UDP 代理
		dialer: &net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: keepAliveInterval,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	// 创建 MASQUE 传输层客户端
	a.client = masqueTransport.NewClient(&masqueTransport.Config{
		Host:     server,
		Port:     port,
		Path:     a.path,
		Password: password,
		SNI:      a.sni,
	})

	return a, nil
}

// Option 配置选项
type Option func(*Adapter)

// WithSNI 设置 TLS SNI
func WithSNI(sni string) Option {
	return func(a *Adapter) {
		a.sni = sni
	}
}

// WithPath 设置 HTTP/3 路径
func WithPath(path string) Option {
	return func(a *Adapter) {
		a.path = path
	}
}

// WithSkipCertVerify 跳过证书验证
func WithSkipCertVerify(skip bool) Option {
	return func(a *Adapter) {
		a.allowInsecure = skip
	}
}

// WithUDP 启用/禁用 UDP 支持
func WithUDP(enable bool) Option {
	return func(a *Adapter) {
		a.supportUDP = enable
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeMASQUE }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.supportUDP }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	log.Debug().
		Str("server", a.Addr()).
		Str("target", metadata.DestinationAddress()).
		Msg("[MASQUE] 建立连接")

	// 通过 MASQUE 传输层建立连接
	targetAddr := metadata.DestinationAddress()
	conn, err := a.client.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("MASQUE 连接失败: %w", err)
	}

	// 缓存连接以供复用
	a.connMu.Lock()
	a.conn = conn
	a.connMu.Unlock()

	return conn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	if !a.supportUDP {
		return nil, fmt.Errorf("MASQUE UDP 未启用")
	}

	log.Debug().
		Str("server", a.Addr()).
		Str("target", metadata.DestinationAddress()).
		Msg("[MASQUE] 建立 UDP 连接")

	// MASQUE 支持 UDP 代理 (RFC 9298)
	// 通过 CONNECT-UDP 方法建立 UDP 代理通道
	conn, err := a.client.Dial("udp", metadata.DestinationAddress())
	if err != nil {
		return nil, fmt.Errorf("MASQUE UDP 连接失败: %w", err)
	}

	// 将 net.Conn 包装为 net.PacketConn
	pc := newPacketConnWrapper(conn)
	return pc, nil
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

// Close 关闭适配器
func (a *Adapter) Close() error {
	a.connMu.Lock()
	defer a.connMu.Unlock()

	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// packetConnWrapper 将 net.Conn 包装为 net.PacketConn
type packetConnWrapper struct {
	conn     net.Conn
	mu       sync.Mutex
	readAddr net.Addr
}

func newPacketConnWrapper(conn net.Conn) *packetConnWrapper {
	return &packetConnWrapper{
		conn:     conn,
		readAddr: conn.RemoteAddr(),
	}
}

// ReadFrom 从连接读取数据 (实现 net.PacketConn)
func (w *packetConnWrapper) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := w.conn.Read(b)
	if err != nil {
		return 0, nil, err
	}
	return n, w.readAddr, nil
}

// WriteTo 向指定地址写入数据 (实现 net.PacketConn)
func (w *packetConnWrapper) WriteTo(b []byte, addr net.Addr) (int, error) {
	return w.conn.Write(b)
}

// Close 关闭连接
func (w *packetConnWrapper) Close() error {
	return w.conn.Close()
}

// LocalAddr 本地地址
func (w *packetConnWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

// SetDeadline 设置截止时间
func (w *packetConnWrapper) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

// SetReadDeadline 设置读取截止时间
func (w *packetConnWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入截止时间
func (w *packetConnWrapper) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}

// newTLSConfig 创建 TLS 配置
func (a *Adapter) newTLSConfig() *tls.Config {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}
	return &tls.Config{
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: a.allowInsecure,
		NextProtos:         []string{"h3"}, // MASQUE 使用 HTTP/3
	}
}
