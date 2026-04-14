// Package trojan Trojan 协议实现
package trojan

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hades/hades/pkg/core/adapter"
	"github.com/hades/hades/pkg/perf/pool"
)

const (
	// CRLF 回车换行
	CRLF = "\r\n"
	// trojanProtocolVersion 协议版本
	trojanProtocolVersion = "trojan"
)

// Adapter Trojan 适配器
type Adapter struct {
	name           string
	server         string
	port           int
	password       string
	sni            string
	allowInsecure  bool
	udp            bool
	network        string
	wsPath         string
	wsHost         string
	grpcName       string
	fingerprint    string
	skipCertVerify bool
	dialer         *net.Dialer
}

// NewAdapter 创建 Trojan 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:      name,
		server:    server,
		port:      port,
		password:  password,
		network:   "tcp",
		udp:       true,
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

// WithNetwork 设置网络类型
func WithNetwork(network string) Option {
	return func(a *Adapter) {
		a.network = network
	}
}

// WithWebSocket 设置 WebSocket
func WithWebSocket(path, host string) Option {
	return func(a *Adapter) {
		a.network = "ws"
		a.wsPath = path
		a.wsHost = host
	}
}

// WithGRPC 设置 gRPC
func WithGRPC(serviceName string) Option {
	return func(a *Adapter) {
		a.network = "grpc"
		a.grpcName = serviceName
	}
}

// WithFingerprint 设置 TLS 指纹
func WithFingerprint(fp string) Option {
	return func(a *Adapter) {
		a.fingerprint = fp
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeTrojan }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 Trojan 服务器失败: %w", err)
	}

	// TLS 封装
	tlsConn := a.tlsWrap(conn)

	// 传输层封装
	switch a.network {
	case "ws":
		// TODO: WebSocket
	case "grpc":
		// TODO: gRPC
	}

	// Trojan 握手
	if err := a.handshake(tlsConn, metadata); err != nil {
		conn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	// Trojan 使用 Trojan Request 来建立 UDP
	return nil, fmt.Errorf("Trojan UDP 尚未实现")
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

// handshake Trojan 握手
func (a *Adapter) handshake(conn net.Conn, metadata *adapter.Metadata) error {
	// Trojan 协议格式:
	// SHA224(password) + CRLF + target_addr + CRLF + payload

	passwordHash := sha256.Sum224([]byte(a.password))

	buf := pool.GetMedium()
	defer pool.Put(buf)
	offset := 0

	// 密码哈希 (56 bytes)
	copy(buf[offset:], passwordHash[:])
	offset += 28 // SHA224 = 28 bytes

	// CRLF
	buf[offset] = '\r'
	offset++
	buf[offset] = '\n'
	offset++

	// 命令
	buf[offset] = 0x01 // TCP CONNECT
	offset++

	// 目标地址
	addr := packAddress(metadata.Host, metadata.DstPort)
	copy(buf[offset:], addr)
	offset += len(addr)

	// CRLF
	buf[offset] = '\r'
	offset++
	buf[offset] = '\n'
	offset++

	// 发送
	_, err := conn.Write(buf[:offset])
	return err
}

// tlsWrap TLS 封装
func (a *Adapter) tlsWrap(conn net.Conn) net.Conn {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}
	tlsConfig := &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}
	if a.skipCertVerify {
		tlsConfig.InsecureSkipVerify = true
	}
	return tls.Client(conn, tlsConfig)
}

// packAddress 打包地址
func packAddress(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		domain := []byte(host)
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(domain)))
		offset += 2
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		buf[offset] = 0x01
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		buf[offset] = 0x03
		offset++
		copy(buf[offset:], ip.To16())
		offset += 16
	}

	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	return buf[:offset]
}

// unpackAddress 解包地址
func unpackAddress(data []byte) (string, uint16, []byte, error) {
	if len(data) < 1 {
		return "", 0, nil, io.ErrShortBuffer
	}

	var host string
	var offset int

	// 判断地址类型
	if data[0] == 0x01 {
		// IPv4
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	} else if data[0] == 0x03 {
		// IPv6
		if len(data) < 19 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:17]).String()
		offset = 17
	} else {
		// 域名 (长度前缀)
		domainLen := binary.BigEndian.Uint16(data[:2])
		if len(data) < 2+int(domainLen)+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[2 : 2+domainLen])
		offset = 2 + int(domainLen)
	}

	port := binary.BigEndian.Uint16(data[offset:])
	remaining := data[offset+2:]

	return host, port, remaining, nil
}

// Trojan UDP 数据包格式
const (
	// UDPRequest Trojan UDP 请求
	UDPRequest = 0x03
	// UDPAssociate Trojan UDP 关联
	UDPAssociate = 0x01
)

// ParsePasswordHash 解析 Trojan 密码哈希
func ParsePasswordHash(password string) [28]byte {
	hash := sha256.Sum224([]byte(password))
	var result [28]byte
	copy(result[:], hash[:28])
	return result
}

// ValidatePasswordHash 验证 Trojan 密码哈希
func ValidatePasswordHash(data []byte, password string) bool {
	if len(data) < 28+2 {
		return false
	}

	hash := sha256.Sum224([]byte(password))
	return string(data[:28]) == string(hash[:28]) &&
		data[28] == '\r' && data[29] == '\n'
}

// trojanReader Trojan 读取器
type trojanReader struct {
	conn   io.Reader
	buf    []byte
	offset int
}

func newTrojanReader(conn net.Conn) *trojanReader {
	return &trojanReader{
		conn: conn,
		buf:  make([]byte, 0),
	}
}

// Read 读取数据
func (r *trojanReader) Read(b []byte) (int, error) {
	if r.offset < len(r.buf) {
		n := copy(b, r.buf[r.offset:])
		r.offset += n
		return n, nil
	}

	// 读取新数据
	return r.conn.Read(b)
}

// trojanConn Trojan 加密连接
type trojanConn struct {
	net.Conn
	passwordHash [28]byte
	reader       *trojanReader
}

// NewTrojanConn 创建 Trojan 连接
func NewTrojanConn(conn net.Conn, password string) *trojanConn {
	return &trojanConn{
		Conn:         conn,
		passwordHash: ParsePasswordHash(password),
		reader:       newTrojanReader(conn),
	}
}

// Relay 双向转发
func Relay(left, right net.Conn) {
	// Trojan 基于 TLS，直接转发
	buf := pool.GetLarge()
	defer pool.Put(buf)

	go func() {
		io.CopyBuffer(left, right, buf)
		left.Close()
	}()

	buf2 := pool.GetLarge()
	defer pool.Put(buf2)
	io.CopyBuffer(right, left, buf2)
	right.Close()
}


