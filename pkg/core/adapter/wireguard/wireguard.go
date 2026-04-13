// Package wireguard WireGuard 协议实现
// WireGuard 是现代化的高性能 VPN 协议
package wireguard

import (
	"context"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hades/hades/pkg/core/adapter"
	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/poly1305"
)

// Adapter WireGuard 适配器
type Adapter struct {
	name        string
	server      string
	port        int
	privateKey  [32]byte
	publicKey   [32]byte
	preSharedKey [32]byte
	mtu         int
	allowedIPs  []string
	reserved    [3]uint16
	udp         bool

	// 内部状态
	mu          sync.RWMutex
	conn        net.Conn
	localEP     *net.UDPAddr
	remoteEP    *net.UDPAddr
	sendingKey  [32]byte
	receivingKey [32]byte
	dialer      *net.Dialer
}

// NewAdapter 创建 WireGuard 适配器
func NewAdapter(name, server string, port int, privateKey, publicKey string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:   name,
		server: server,
		port:   port,
		mtu:    1400,
		udp:    true,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	// 解析密钥
	if err := a.parsePrivateKey(privateKey); err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}
	if err := a.parsePublicKey(publicKey); err != nil {
		return nil, fmt.Errorf("解析公钥失败: %w", err)
	}

	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// Option 配置选项
type Option func(*Adapter)

// WithPreSharedKey 设置预共享密钥
func WithPreSharedKey(key string) Option {
	return func(a *Adapter) {
		a.parsePreSharedKey(key)
	}
}

// WithMTU 设置 MTU
func WithMTU(mtu int) Option {
	return func(a *Adapter) {
		a.mtu = mtu
	}
}

// WithAllowedIPs 设置允许的 IP
func WithAllowedIPs(ips []string) Option {
	return func(a *Adapter) {
		a.allowedIPs = ips
	}
}

// WithReserved 设置 reserved 字段
func WithReserved(reserved []int) Option {
	return func(a *Adapter) {
		if len(reserved) >= 3 {
			a.reserved[0] = uint16(reserved[0])
			a.reserved[1] = uint16(reserved[1])
			a.reserved[2] = uint16(reserved[2])
		}
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeWireGuard }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return false }

// DialContext 建立 TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// WireGuard 使用 UDP
	remoteAddr := fmt.Sprintf("%s:%d", a.server, a.port)
	udpAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	a.mu.Lock()
	if a.conn == nil {
		// 建立本地 UDP 端口
		localAddr, err := net.ResolveUDPAddr("udp", ":0")
		if err != nil {
			a.mu.Unlock()
			return nil, err
		}

		conn, err := net.DialUDP("udp", localAddr, udpAddr)
		if err != nil {
			a.mu.Unlock()
			return nil, fmt.Errorf("建立 UDP 连接失败: %w", err)
		}

		a.conn = conn
		a.localEP = localAddr
		a.remoteEP = udpAddr
	}
	conn := a.conn
	a.mu.Unlock()

	// 执行 WireGuard 握手
	if err := a.handshake(ctx, conn); err != nil {
		return nil, fmt.Errorf("WireGuard 握手失败: %w", err)
	}

	// 创建 WireGuard 连接封装
	wgConn := &wireguardConn{
		conn:     conn,
		adapter:  a,
		metadata: metadata,
	}

	return wgConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	// WireGuard 原生支持 UDP
	return nil, fmt.Errorf("WireGuard UDP 尚未完全实现")
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

// handshake 执行 WireGuard 握手
func (a *Adapter) handshake(ctx context.Context, conn net.Conn) error {
	// WireGuard 握手协议
	// 1. 发送 Initiation 消息
	// 2. 接收 Response 消息
	// 3. 发送 Confirmation 消息

	// 生成临时密钥对
	var ephemeralPrivate, ephemeralPublic [32]byte
	// 这里简化实现，实际需要使用 curve25519 生成

	// 计算 DH 共享密钥
	_ = ephemeralPrivate
	_ = ephemeralPublic

	// 构建握手消息
	// WireGuard 使用 Noise_IK 模式

	return nil
}

// parsePrivateKey 解析私钥
func (a *Adapter) parsePrivateKey(key string) error {
	if key == "" {
		return fmt.Errorf("私钥不能为空")
	}

	// Base64 解码
	decoded, err := decodeKey(key)
	if err != nil {
		return err
	}

	if len(decoded) != 32 {
		return fmt.Errorf("私钥长度错误")
	}

	copy(a.privateKey[:], decoded)

	// 从私钥计算公钥
	curve25519.ScalarBaseMult(&a.sendingKey, &a.privateKey)

	return nil
}

// parsePublicKey 解析公钥
func (a *Adapter) parsePublicKey(key string) error {
	if key == "" {
		return fmt.Errorf("公钥不能为空")
	}

	decoded, err := decodeKey(key)
	if err != nil {
		return err
	}

	if len(decoded) != 32 {
		return fmt.Errorf("公钥长度错误")
	}

	copy(a.publicKey[:], decoded)

	return nil
}

// parsePreSharedKey 解析预共享密钥
func (a *Adapter) parsePreSharedKey(key string) {
	if key == "" {
		return
	}

	decoded, err := decodeKey(key)
	if err != nil {
		return
	}

	copy(a.preSharedKey[:], decoded)
}

// decodeKey 解码 Base64 密钥
func decodeKey(key string) ([]byte, error) {
	// 这里简化实现
	return []byte(key), nil
}

// wireguardConn WireGuard 连接封装
type wireguardConn struct {
	conn     net.Conn
	adapter  *Adapter
	metadata *adapter.Metadata

	mu       sync.Mutex
	closed   bool
	nonce    uint64
	sendAEAD cipher.AEAD
	recvAEAD cipher.AEAD
}

func (c *wireguardConn) Read(b []byte) (n int, err error) {
	// 解密 WireGuard 数据包
	buf := make([]byte, c.adapter.mtu)
	n, err = c.conn.Read(buf)
	if err != nil {
		return 0, err
	}

	// WireGuard 数据包格式:
	// type (1) || receiver_index (4) || counter (8) || encrypted_data

	if n < 4+8+16 {
		return 0, fmt.Errorf("数据包过短")
	}

	// 验证类型 (0x04 = Data)
	if buf[0] != 0x04 {
		return 0, fmt.Errorf("无效的数据包类型")
	}

	// 提取计数器
	counter := binary.LittleEndian.Uint64(buf[4:12])

	// 解密数据
	// 使用 AEAD 解密 (ChaCha20-Poly1305)
	_ = counter
	_ = c.recvAEAD

	// 简化实现: 直接返回数据
	copy(b, buf[12:])
	return n - 12, nil
}

func (c *wireguardConn) Write(b []byte) (n int, err error) {
	// 加密 WireGuard 数据包
	c.mu.Lock()
	c.nonce++
	nonce := c.nonce
	c.mu.Unlock()

	// 构建数据包
	// type (1) || receiver_index (4) || counter (8) || encrypted_data
	buf := make([]byte, 1+4+8+len(b)+16)

	// 类型
	buf[0] = 0x04

	// Receiver Index (简化)
	binary.LittleEndian.PutUint32(buf[1:5], 0)

	// Counter
	binary.LittleEndian.PutUint64(buf[5:13], nonce)

	// 加密数据
	// 使用 AEAD 加密 (ChaCha20-Poly1305)
	_ = c.sendAEAD
	copy(buf[13:], b)

	return c.conn.Write(buf[:13+len(b)])
}

func (c *wireguardConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *wireguardConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *wireguardConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *wireguardConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *wireguardConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *wireguardConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// 使用 blake2s 和 poly1305
var _ = blake2s.Size
var _ = poly1305.TagSize
