// Package anytls AnyTLS 协议实现
// AnyTLS 使用 TLS 1.3 + 基于密码的密钥派生，在 TLS 连接上多路复用流
package anytls

import (
	"context"
	"crypto/rand"
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
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/hkdf"
)

const (
	// protocolVersion AnyTLS 协议版本
	protocolVersion byte = 0x01
	// commandConnect TCP CONNECT 命令
	commandConnect byte = 0x01
	// commandUDP UDP ASSOCIATE 命令
	commandUDP byte = 0x03
	// headerSize 流头大小 (2字节流ID + 2字节数据长度)
	headerSize = 4
	// maxStreamID 最大流 ID
	maxStreamID = 65535
)

// Adapter AnyTLS 适配器
type Adapter struct {
	name          string
	server        string
	port          int
	password      string
	sni           string
	allowInsecure bool
	dialer        *net.Dialer
}

// NewAdapter 创建 AnyTLS 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	if password == "" {
		return nil, fmt.Errorf("AnyTLS 密码不能为空")
	}

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

// WithSkipCertVerify 跳过证书验证
func WithSkipCertVerify(skip bool) Option {
	return func(a *Adapter) {
		a.allowInsecure = skip
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
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeAnyTLS }

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
		Msg("[AnyTLS] 建立连接")

	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 AnyTLS 服务器失败: %w", err)
	}

	// TLS 封装
	tlsConn, err := a.tlsWrap(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("AnyTLS TLS 握手失败: %w", err)
	}

	// 派生会话密钥
	sessionKey, err := deriveSessionKey(a.password)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("AnyTLS 密钥派生失败: %w", err)
	}

	// 发送握手认证
	if err := a.authenticate(tlsConn, sessionKey); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("AnyTLS 认证失败: %w", err)
	}

	// 创建多路复用连接
	muxConn := newMuxConn(tlsConn, sessionKey)

	// 打开新流并发送目标地址
	stream, err := muxConn.OpenStream()
	if err != nil {
		muxConn.Close()
		return nil, fmt.Errorf("AnyTLS 打开流失败: %w", err)
	}

	// 发送 CONNECT 命令和目标地址
	if err := stream.sendTarget(commandConnect, metadata); err != nil {
		muxConn.Close()
		return nil, fmt.Errorf("AnyTLS 发送目标地址失败: %w", err)
	}

	return stream, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("AnyTLS 不支持 UDP")
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

// tlsWrap TLS 封装
func (a *Adapter) tlsWrap(conn net.Conn) (*tls.Conn, error) {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}

	tlsConfig := &tls.Config{
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS13,
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

// authenticate 发送认证信息
func (a *Adapter) authenticate(conn net.Conn, sessionKey []byte) error {
	// AnyTLS 认证格式:
	// [1 byte version] [32 bytes key hash] [16 bytes random nonce]
	keyHash := sha256.Sum256(sessionKey)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	buf := make([]byte, 1+32+16)
	buf[0] = protocolVersion
	copy(buf[1:33], keyHash[:])
	copy(buf[33:], nonce)

	_, err := conn.Write(buf)
	return err
}

// deriveSessionKey 使用 HKDF 从密码派生会话密钥
func deriveSessionKey(password string) ([]byte, error) {
	salt := []byte("anytls-session-key-salt")
	hkdfReader := hkdf.New(sha256.New, []byte(password), salt, []byte("anytls-session-key"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// muxConn 多路复用连接
type muxConn struct {
	conn       net.Conn
	sessionKey []byte
	mu         sync.Mutex
	streams    map[uint16]*stream
	nextID     uint16
	closed     bool
}

func newMuxConn(conn net.Conn, sessionKey []byte) *muxConn {
	return &muxConn{
		conn:       conn,
		sessionKey: sessionKey,
		streams:    make(map[uint16]*stream),
		nextID:     1,
	}
}

// OpenStream 打开新流
func (m *muxConn) OpenStream() (*stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("连接已关闭")
	}

	if m.nextID >= maxStreamID {
		return nil, fmt.Errorf("流 ID 已耗尽")
	}

	id := m.nextID
	m.nextID++

	s := &stream{
		conn:       m.conn,
		id:         id,
		sessionKey: m.sessionKey,
		readBuf:    make([]byte, 0, 65536),
	}
	m.streams[id] = s

	return s, nil
}

// Close 关闭连接
func (m *muxConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	for _, s := range m.streams {
		s.Close()
	}
	return m.conn.Close()
}

// stream 多路复用流
type stream struct {
	conn       net.Conn
	id         uint16
	sessionKey []byte
	mu         sync.Mutex
	readBuf    []byte
	readOffset int
	closed     bool
}

// sendTarget 发送目标地址
func (s *stream) sendTarget(cmd byte, metadata *adapter.Metadata) error {
	buf := pool.GetMedium()
	defer pool.Put(buf)
	offset := 0

	// 流头: [2 bytes stream ID] [2 bytes payload length (后续填入)]
	binary.BigEndian.PutUint16(buf[offset:], s.id)
	offset += headerSize // 预留 4 字节头

	// 命令
	buf[offset] = cmd
	offset++

	// 目标地址 (SOCKS5 格式)
	addr := packAddr(metadata.Host, metadata.DstPort)
	copy(buf[offset:], addr)
	offset += len(addr)

	// 计算 payload 长度并填入头
	payloadLen := offset - headerSize
	binary.BigEndian.PutUint16(buf[2:], uint16(payloadLen))

	_, err := s.conn.Write(buf[:offset])
	return err
}

// Read 读取数据
func (s *stream) Read(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, io.EOF
	}

	// 如果缓冲区有数据，先返回缓冲区数据
	if s.readOffset < len(s.readBuf) {
		n := copy(b, s.readBuf[s.readOffset:])
		s.readOffset += n
		return n, nil
	}

	// 读取流头
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(s.conn, header); err != nil {
		return 0, err
	}

	streamID := binary.BigEndian.Uint16(header[:2])
	payloadLen := binary.BigEndian.Uint16(header[2:])

	// 检查流 ID
	if streamID != s.id {
		return 0, fmt.Errorf("AnyTLS 流 ID 不匹配: 期望 %d, 收到 %d", s.id, streamID)
	}

	// 读取 payload
	if cap(s.readBuf) < int(payloadLen) {
		s.readBuf = make([]byte, payloadLen)
	} else {
		s.readBuf = s.readBuf[:payloadLen]
	}
	s.readOffset = 0

	if _, err := io.ReadFull(s.conn, s.readBuf); err != nil {
		return 0, err
	}

	n := copy(b, s.readBuf)
	s.readOffset = n
	return n, nil
}

// Write 写入数据
func (s *stream) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, io.ErrClosedPipe
	}

	// 构造帧: [2 bytes stream ID] [2 bytes length] [payload]
	frame := make([]byte, headerSize+len(b))
	binary.BigEndian.PutUint16(frame[:2], s.id)
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(b)))
	copy(frame[4:], b)

	_, err := s.conn.Write(frame)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close 关闭流
func (s *stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// LocalAddr 本地地址
func (s *stream) LocalAddr() net.Addr { return s.conn.LocalAddr() }

// RemoteAddr 远程地址
func (s *stream) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }

// SetDeadline 设置截止时间
func (s *stream) SetDeadline(t time.Time) error { return s.conn.SetDeadline(t) }

// SetReadDeadline 设置读取截止时间
func (s *stream) SetReadDeadline(t time.Time) error { return s.conn.SetReadDeadline(t) }

// SetWriteDeadline 设置写入截止时间
func (s *stream) SetWriteDeadline(t time.Time) error { return s.conn.SetWriteDeadline(t) }

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
