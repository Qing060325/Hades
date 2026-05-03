// Package trusttunnel TrustTunnel 协议实现
// TrustTunnel 使用 TLS + SNI 路由，支持 WebSocket 和 gRPC 传输
package trusttunnel

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/Qing060325/Hades/pkg/transport"
	"github.com/rs/zerolog/log"
)

const (
	// protocolMagic 协议魔数
	protocolMagic byte = 0x54 // 'T'
	// protocolVersion 协议版本
	protocolVersion byte = 0x01
	// commandConnect TCP CONNECT 命令
	commandConnect byte = 0x01
	// commandUDP UDP ASSOCIATE 命令
	commandUDP byte = 0x03
	// transportTCP TCP 传输模式
	transportTCP = "tcp"
	// transportWS WebSocket 传输模式
	transportWS = "ws"
	// transportGRPC gRPC 传输模式
	transportGRPC = "grpc"
)

// Adapter TrustTunnel 适配器
type Adapter struct {
	name          string
	server        string
	port          int
	password      string
	sni           string
	allowInsecure bool
	transport     string // tcp, ws, grpc
	wsPath        string
	wsHost        string
	grpcService   string
	dialer        *net.Dialer
}

// NewAdapter 创建 TrustTunnel 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:      name,
		server:    server,
		port:      port,
		password:  password,
		transport: transportTCP,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// Option 配置选项
type Option func(*Adapter)

// WithSNI 设置 TLS SNI (用于 SNI 路由)
func WithSNI(sni string) Option {
	return func(a *Adapter) {
		a.sni = sni
	}
}

// WithSkipCertVerify 跳过证书验证
func WithSkipCertVerify(skip bool) Option {
	return func(a *Adapter) {
		a.allowInsecure = skip
	}
}

// WithWebSocket 设置 WebSocket 传输
func WithWebSocket(path, host string) Option {
	return func(a *Adapter) {
		a.transport = transportWS
		a.wsPath = path
		a.wsHost = host
	}
}

// WithGRPC 设置 gRPC 传输
func WithGRPC(serviceName string) Option {
	return func(a *Adapter) {
		a.transport = transportGRPC
		a.grpcService = serviceName
	}
}

// WithDialer 设置自定义拨号器
func WithDialer(dialer *net.Dialer) Option {
	return func(a *Adapter) {
		a.dialer = dialer
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeTrustTunnel }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return false }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	log.Debug().
		Str("server", a.Addr()).
		Str("target", metadata.DestinationAddress()).
		Str("transport", a.transport).
		Msg("[TrustTunnel] 建立连接")

	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 TrustTunnel 服务器失败: %w", err)
	}

	// TLS 封装 (使用 SNI 路由)
	tlsConn, err := a.tlsWrap(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("TrustTunnel TLS 握手失败: %w", err)
	}

	// 传输层封装
	var wrappedConn net.Conn
	switch a.transport {
	case transportWS:
		wrappedConn, err = a.wsWrap(tlsConn)
		if err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("TrustTunnel WebSocket 封装失败: %w", err)
		}
	case transportGRPC:
		wrappedConn, err = a.grpcWrap(tlsConn)
		if err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("TrustTunnel gRPC 封装失败: %w", err)
		}
	default:
		wrappedConn = tlsConn
	}

	// TrustTunnel 握手
	if err := a.handshake(wrappedConn, metadata); err != nil {
		wrappedConn.Close()
		return nil, fmt.Errorf("TrustTunnel 握手失败: %w", err)
	}

	return wrappedConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("TrustTunnel 不支持 UDP")
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

// tlsWrap TLS 封装 (使用 SNI 路由)
func (a *Adapter) tlsWrap(conn net.Conn) (*tls.Conn, error) {
	// TrustTunnel 使用 SNI 进行路由，SNI 字段决定后端路由目标
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: a.allowInsecure,
	}

	tlsConn := tls.Client(conn, tlsConfig)

	// 设置 TLS 握手超时
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, err
	}

	return tlsConn, nil
}

// wsWrap WebSocket 封装
func (a *Adapter) wsWrap(conn net.Conn) (net.Conn, error) {
	host := a.wsHost
	if host == "" {
		host = a.server
	}

	headers := map[string]string{
		"Host": host,
	}

	wsConn, err := transport.NewWebSocketConn(conn, a.wsPath, host, headers)
	if err != nil {
		return nil, err
	}

	return wsConn, nil
}

// grpcWrap gRPC 封装
func (a *Adapter) grpcWrap(conn net.Conn) (net.Conn, error) {
	// gRPC 封装: 使用 HTTP/2 帧
	serviceName := a.grpcService
	if serviceName == "" {
		serviceName = "trusttunnel.TunnelService"
	}

	// 创建 gRPC 连接封装
	gConn := newGRPCConn(conn, serviceName)
	return gConn, nil
}

// handshake TrustTunnel 握手
func (a *Adapter) handshake(conn net.Conn, metadata *adapter.Metadata) error {
	buf := pool.GetMedium()
	defer pool.Put(buf)
	offset := 0

	// 协议魔数
	buf[offset] = protocolMagic
	offset++

	// 协议版本
	buf[offset] = protocolVersion
	offset++

	// 认证哈希 (SHA256 的前 16 字节)
	authHash := computeAuthHash(a.password)
	copy(buf[offset:], authHash[:16])
	offset += 16

	// 命令
	buf[offset] = commandConnect
	offset++

	// 目标地址 (SOCKS5 格式)
	addr := packAddr(metadata.Host, metadata.DstPort)
	copy(buf[offset:], addr)
	offset += len(addr)

	_, err := conn.Write(buf[:offset])
	return err
}

// computeAuthHash 计算认证哈希
func computeAuthHash(password string) [32]byte {
	// 使用密码和固定盐值计算 HMAC-SHA256
	salt := "trusttunnel-auth-salt"
	h := sha256.New()
	h.Write([]byte(password))
	h.Write([]byte(salt))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// gRPCConn gRPC 连接封装
type gRPCConn struct {
	net.Conn
	serviceName string
	mu          sync.Mutex
	closed      bool
}

func newGRPCConn(conn net.Conn, serviceName string) *gRPCConn {
	return &gRPCConn{
		Conn:        conn,
		serviceName: serviceName,
	}
}

// Read 读取数据
func (c *gRPCConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.EOF
	}

	// gRPC 长度前缀帧格式: [1 byte compressed] [4 bytes length] [data]
	header := make([]byte, 5)
	if _, err := io.ReadFull(c.Conn, header); err != nil {
		return 0, err
	}

	// 解析长度
	length := uint32(header[1])<<24 | uint32(header[2])<<16 | uint32(header[3])<<8 | uint32(header[4])
	if length > uint32(len(b)) {
		// 缓冲区不足，读取部分
		n, err := io.ReadFull(c.Conn, b[:length])
		if err != nil {
			return n, err
		}
		// 跳过剩余数据
		remaining := int(length) - n
		if remaining > 0 {
			skip := make([]byte, remaining)
			if _, err := io.ReadFull(c.Conn, skip); err != nil {
				return n, err
			}
		}
		return n, nil
	}

	return io.ReadFull(c.Conn, b[:length])
}

// Write 写入数据
func (c *gRPCConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	// gRPC 长度前缀帧格式
	frame := make([]byte, 5+len(b))
	frame[0] = 0 // 未压缩
	frame[1] = byte(len(b) >> 24)
	frame[2] = byte(len(b) >> 16)
	frame[3] = byte(len(b) >> 8)
	frame[4] = byte(len(b))
	copy(frame[5:], b)

	_, err := c.Conn.Write(frame)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close 关闭连接
func (c *gRPCConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.Conn.Close()
}

// packAddr 打包目标地址 (SOCKS5 格式)
func packAddr(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0
	ip := net.ParseIP(host)
	if ip == nil {
		domain := []byte(host)
		if len(domain) > 255 {
			domain = domain[:255]
		}
		buf[offset] = 0x03
		offset++
		buf[offset] = byte(len(domain))
		offset++
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		buf[offset] = 0x01
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		buf[offset] = 0x04
		offset++
		copy(buf[offset:], ip.To16())
		offset += 16
	}

	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	result := make([]byte, offset)
	copy(result, buf[:offset])
	return result
}

// unpackAddr 解包目标地址
func unpackAddr(data []byte) (string, uint16, []byte, error) {
	if len(data) < 1 {
		return "", 0, nil, io.ErrShortBuffer
	}

	var host string
	var offset int

	switch data[0] {
	case 0x01:
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	case 0x03:
		if len(data) < 2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		domainLen := int(data[1])
		if len(data) < 2+domainLen+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[2 : 2+domainLen])
		offset = 2 + domainLen
	case 0x04:
		if len(data) < 19 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:17]).String()
		offset = 17
	default:
		return "", 0, nil, fmt.Errorf("未知地址类型: 0x%02x", data[0])
	}

	port := binary.BigEndian.Uint16(data[offset:])
	remaining := data[offset+2:]

	return host, port, remaining, nil
}
