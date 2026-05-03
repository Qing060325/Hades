// Package amneziawg Amnezia WireGuard 协议实现
// AmneziaWG 是基于 WireGuard 的混淆变体，通过添加垃圾包和随机填充来抵抗 DPI 检测
package amneziawg

import (
	"context"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// Adapter AmneziaWG 适配器
type Adapter struct {
	name       string
	server     string
	port       int
	privateKey [32]byte
	publicKey  [32]byte
	preSharedKey [32]byte
	mtu        int
	allowedIPs []string
	reserved   [3]uint16
	udp        bool

	// AmneziaWG 混淆参数
	jc  uint8  // 垃圾包数量 (Junk packet count)
	jmin uint16 // 垃圾包最小尺寸 (Junk packet min size)
	jmax uint16 // 垃圾包最大尺寸 (Junk packet max size)
	s1  uint16 // 初始化包垃圾数据大小 (Init packet junk size)
	s2  uint16 // 响应包垃圾数据大小 (Response packet junk size)
	h1  uint32 // 握手消息1的哈希值 (Handshake hash 1)
	h2  uint32 // 握手消息2的哈希值 (Handshake hash 2)
	h3  uint32 // 握手消息3的哈希值 (Handshake hash 3)
	h4  uint32 // 握手消息4的哈希值 (Handshake hash 4)

	// 内部状态
	mu          sync.RWMutex
	conn        net.Conn
	localEP     *net.UDPAddr
	remoteEP    *net.UDPAddr
	sendingKey  [32]byte
	receivingKey [32]byte
	dialer      *net.Dialer
}

// NewAdapter 创建 AmneziaWG 适配器
func NewAdapter(name, server string, port int, privateKey, publicKey string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:   name,
		server: server,
		port:   port,
		mtu:    1400,
		udp:    true,
		// AmneziaWG 默认混淆参数
		jc:   3,    // 默认 3 个垃圾包
		jmin: 15,   // 最小 15 字节
		jmax: 150,  // 最大 150 字节
		s1:   35,   // Init 包填充 35 字节
		s2:   45,   // Response 包填充 45 字节
		h1:   0,    // 握手哈希（由协议自动派生）
		h2:   0,
		h3:   0,
		h4:   0,
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

// WithAllowedIPs 设置允许的 IP 列表
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

// WithJc 设置垃圾包数量
func WithJc(jc uint8) Option {
	return func(a *Adapter) {
		a.jc = jc
	}
}

// WithJmin 设置垃圾包最小尺寸
func WithJmin(jmin uint16) Option {
	return func(a *Adapter) {
		a.jmin = jmin
	}
}

// WithJmax 设置垃圾包最大尺寸
func WithJmax(jmax uint16) Option {
	return func(a *Adapter) {
		a.jmax = jmax
	}
}

// WithS1 设置初始化包垃圾数据大小
func WithS1(s1 uint16) Option {
	return func(a *Adapter) {
		a.s1 = s1
	}
}

// WithS2 设置响应包垃圾数据大小
func WithS2(s2 uint16) Option {
	return func(a *Adapter) {
		a.s2 = s2
	}
}

// WithH1 设置握手哈希值 1
func WithH1(h1 uint32) Option {
	return func(a *Adapter) {
		a.h1 = h1
	}
}

// WithH2 设置握手哈希值 2
func WithH2(h2 uint32) Option {
	return func(a *Adapter) {
		a.h2 = h2
	}
}

// WithH3 设置握手哈希值 3
func WithH3(h3 uint32) Option {
	return func(a *Adapter) {
		a.h3 = h3
	}
}

// WithH4 设置握手哈希值 4
func WithH4(h4 uint32) Option {
	return func(a *Adapter) {
		a.h4 = h4
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeAmneziaWG }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return false }

// DialContext 建立 TCP 连接（通过 AmneziaWG 隧道）
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	remoteAddr := fmt.Sprintf("%s:%d", a.server, a.port)
	udpAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	a.mu.Lock()
	if a.conn == nil {
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

	// 执行 AmneziaWG 握手（带混淆参数）
	if err := a.handshake(ctx, conn); err != nil {
		return nil, fmt.Errorf("AmneziaWG 握手失败: %w", err)
	}

	// 创建 AmneziaWG 连接封装
	awgConn := &amneziaWGConn{
		conn:     conn,
		adapter:  a,
		metadata: metadata,
	}

	return awgConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("AmneziaWG UDP 尚未完全实现")
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

// handshake 执行 AmneziaWG 握手
// AmneziaWG 的握手过程在标准 WireGuard 基础上增加了混淆:
// 1. 发送前插入 jc 个随机大小 (jmin~jmax) 的垃圾包
// 2. Init 包附加 s1 字节的随机填充
// 3. Response 包附加 s2 字节的随机填充
// 4. 使用 h1~h4 作为握手哈希的额外混淆因子
func (a *Adapter) handshake(ctx context.Context, conn net.Conn) error {
	// TODO: 实现完整的 AmneziaWG Noise_IK 握手（带混淆）
	// 握手流程:
	// 1. 生成临时 curve25519 密钥对
	// 2. 插入 jc 个垃圾包 (大小 jmin~jmax 之间的随机值)
	// 3. 构建 Initiation 消息并附加 s1 字节随机填充
	// 4. 发送 Initiation 消息，等待 Response
	// 5. 从 Response 中解密得到传输密钥（Response 附加 s2 字节填充）
	// 6. 使用 h1~h4 进行额外的哈希混淆
	// 7. 使用 HKDF 派生 sendingKey / receivingKey (ChaCha20-Poly1305)
	return nil
}

// parsePrivateKey 解析私钥
func (a *Adapter) parsePrivateKey(key string) error {
	if key == "" {
		return fmt.Errorf("私钥不能为空")
	}
	decoded, err := decodeKey(key)
	if err != nil {
		return err
	}
	if len(decoded) != 32 {
		return fmt.Errorf("私钥长度错误")
	}
	copy(a.privateKey[:], decoded)
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
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	return decoded, nil
}

// amneziaWGConn AmneziaWG 连接封装
type amneziaWGConn struct {
	conn     net.Conn
	adapter  *Adapter
	metadata *adapter.Metadata

	mu       sync.Mutex
	closed   bool
	nonce    uint64
	sendAEAD cipher.AEAD
	recvAEAD cipher.AEAD
}

func (c *amneziaWGConn) Read(b []byte) (n int, err error) {
	if c.recvAEAD == nil {
		return 0, fmt.Errorf("AmneziaWG: recvAEAD 未初始化，握手未完成")
	}

	buf := make([]byte, c.adapter.mtu)
	n, err = c.conn.Read(buf)
	if err != nil {
		return 0, err
	}

	// AmneziaWG 数据包格式:
	// type (1) || receiver_index (4) || counter (8) || encrypted_data
	// 比标准 WireGuard 多了可能的垃圾包前缀，需要在底层过滤
	if n < 4+8+16 {
		return 0, fmt.Errorf("数据包过短")
	}

	// 验证类型 (0x04 = Data)
	if buf[0] != 0x04 {
		return 0, fmt.Errorf("无效的数据包类型: 0x%02x", buf[0])
	}

	// 提取计数器
	counter := binary.LittleEndian.Uint64(buf[4:12])
	_ = counter

	// 使用 AEAD 解密 (ChaCha20-Poly1305)
	_ = c.recvAEAD

	// 简化实现: 直接返回数据
	copy(b, buf[12:])
	if n-12 > len(b) {
		return len(b), nil
	}
	return n - 12, nil
}

func (c *amneziaWGConn) Write(b []byte) (n int, err error) {
	if c.sendAEAD == nil {
		return 0, fmt.Errorf("AmneziaWG: sendAEAD 未初始化，握手未完成")
	}

	c.mu.Lock()
	c.nonce++
	nonce := c.nonce
	c.mu.Unlock()

	// 构建数据包: type (1) || receiver_index (4) || counter (8) || encrypted_data
	buf := make([]byte, 1+4+8+len(b)+16)
	buf[0] = 0x04
	binary.LittleEndian.PutUint32(buf[1:5], 0)
	binary.LittleEndian.PutUint64(buf[5:13], nonce)

	_ = c.sendAEAD
	copy(buf[13:], b)

	return c.conn.Write(buf[:13+len(b)])
}

func (c *amneziaWGConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *amneziaWGConn) LocalAddr() net.Addr  { return c.conn.LocalAddr() }
func (c *amneziaWGConn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

func (c *amneziaWGConn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *amneziaWGConn) SetReadDeadline(t time.Time) error   { return c.conn.SetReadDeadline(t) }
func (c *amneziaWGConn) SetWriteDeadline(t time.Time) error  { return c.conn.SetWriteDeadline(t) }
