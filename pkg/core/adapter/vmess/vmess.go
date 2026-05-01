// Package vmess VMess 协议实现
package vmess

import (
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/Qing060325/Hades/pkg/transport"
)

const (
	// Version VMess 协议版本
	Version byte = 1
	// AEADHeaderLen AEAD 头部长度
	AEADHeaderLen = 1 + 16 + 16 // version + UUID + IV/Key
	// CmdTCP TCP 命令
	CmdTCP byte = 1
	// CmdUDP UDP 命令
	CmdUDP byte = 2
	// SecurityAES128GCM AES-128-GCM 加密
	SecurityAES128GCM byte = 3
	// SecurityChacha20Poly1305 ChaCha20-Poly1305 加密
	SecurityChacha20Poly1305 byte = 4
	// SecurityNone 无加密
	SecurityNone byte = 0
	// vmessResponseHeaderLen VMess 响应头长度
	vmessResponseHeaderLen = 4
)

// VMess 请求头
type header struct {
	Version  byte
	UUID     [16]byte
	Cmd      byte
	Target   targetInfo
	Options  byte
	Security byte
	Payload  []byte
}

// targetInfo 目标信息
type targetInfo struct {
	Address string
	Port    uint16
	Network string
}

// Adapter VMess 适配器
type Adapter struct {
	name    string
	server  string
	port    int
	uuid    string
	alterID int
	cipher  string
	tls     bool
	sni     string
	network string
	wsPath  string
	wsHost  string
	dialer  *net.Dialer
}

// NewAdapter 创建 VMess 适配器
func NewAdapter(name, server string, port int, uuid string, alterID int, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:    name,
		server:  server,
		port:    port,
		uuid:    uuid,
		alterID: alterID,
		cipher:  "auto",
		network: "tcp",
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

// WithTLS 启用 TLS
func WithTLS(sni string) Option {
	return func(a *Adapter) {
		a.tls = true
		a.sni = sni
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

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeVMess }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.network == "tcp" }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 VMess 服务器失败: %w", err)
	}

	// 如果启用 TLS
	if a.tls {
		conn = a.tlsWrap(conn)
	}

	// 如果是 WebSocket
	if a.network == "ws" {
		conn = a.wsWrap(conn)
	}

	// VMess 握手
	vc, err := a.handshake(conn, metadata)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return vc, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("VMess UDP 尚未实现")
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

// handshake VMess 握手，返回 AEAD 加密连接
func (a *Adapter) handshake(conn net.Conn, metadata *adapter.Metadata) (*vmessConn, error) {
	uuid, err := parseUUID(a.uuid)
	if err != nil {
		return nil, fmt.Errorf("解析 UUID 失败: %w", err)
	}

	// 生成请求体随机 key 和 iv
	requestBodyKey := make([]byte, 16)
	requestBodyIV := make([]byte, 16)
	if _, err := rand.Read(requestBodyKey); err != nil {
		return nil, err
	}
	if _, err := rand.Read(requestBodyIV); err != nil {
		return nil, err
	}

	// 构建请求头明文
	// 格式: Version(1) + UUID(16) + Options(1) + Security(1) + Reserved(1) + Command(1)
	//        + Port(2) + AddressType(1) + Address(var)
	headerBuf := make([]byte, 0, 256)
	headerBuf = append(headerBuf, Version)
	headerBuf = append(headerBuf, uuid[:]...)
	headerBuf = append(headerBuf, 0)                          // Options
	headerBuf = append(headerBuf, SecurityAES128GCM)          // Security: AES-128-GCM
	headerBuf = append(headerBuf, 0)                          // Reserved
	headerBuf = append(headerBuf, CmdTCP)                     // Command: TCP CONNECT

	// Port (big-endian)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, metadata.DstPort)
	headerBuf = append(headerBuf, portBytes...)

	// Address
	addrBytes := packAddress(metadata.Host, metadata.DstPort)
	headerBuf = append(headerBuf, addrBytes...)

	// 加密请求头 (AEAD)
	// KDF: 使用当前时间（按小时取整）+ UUID 作为认证密钥
	authKey := kdf(uuid[:], []byte(fmt.Sprintf("%d", time.Now().Unix()/3600)))

	var authKeyArr [16]byte
	copy(authKeyArr[:], authKey[:16])

	encryptedHeader, err := SealVMessAEADHeader(authKeyArr, headerBuf)
	if err != nil {
		return nil, fmt.Errorf("加密请求头失败: %w", err)
	}

	// 发送: encryptedHeader + requestBodyIV(16) + requestBodyKey(16)
	wireBuf := make([]byte, 0, len(encryptedHeader)+32)
	wireBuf = append(wireBuf, encryptedHeader...)
	wireBuf = append(wireBuf, requestBodyIV...)
	wireBuf = append(wireBuf, requestBodyKey...)

	if _, err := conn.Write(wireBuf); err != nil {
		return nil, fmt.Errorf("发送请求头失败: %w", err)
	}

	// 读取并解密响应头
	// 响应头格式: nonce(8) + encrypted(4 + 16 tag = 20 bytes)
	responseNonce := make([]byte, 8)
	if _, err := io.ReadFull(conn, responseNonce); err != nil {
		return nil, fmt.Errorf("读取响应 nonce 失败: %w", err)
	}

	encryptedResponse := make([]byte, vmessResponseHeaderLen+16) // 4 bytes + 16 tag
	if _, err := io.ReadFull(conn, encryptedResponse); err != nil {
		return nil, fmt.Errorf("读取响应头失败: %w", err)
	}

	// 派生响应解密密钥
	responseKey := vmessResponseKey(requestBodyKey)
	responseNonceDerived := vmessResponseNonce(requestBodyIV)

	responseAEAD, err := CreateAEAD(responseKey)
	if err != nil {
		return nil, fmt.Errorf("创建响应 AEAD 失败: %w", err)
	}

	// 构造 nonce
	nonceBuf := make([]byte, responseAEAD.NonceSize())
	copy(nonceBuf, responseNonceDerived[:responseAEAD.NonceSize()])

	// 解密响应头
	responseHeader, err := responseAEAD.Open(nil, nonceBuf, append(responseNonce, encryptedResponse...), nil)
	if err != nil {
		// 兼容旧版：尝试直接解密 encryptedResponse（无 nonce 前缀）
		responseHeader, err = responseAEAD.Open(nil, nonceBuf, encryptedResponse, nil)
		if err != nil {
			return nil, fmt.Errorf("解密响应头失败: %w", err)
		}
	}

	// 解析响应头: Command(1) + Options(1) + Port(2) = 4 bytes
	if len(responseHeader) < vmessResponseHeaderLen {
		return nil, fmt.Errorf("响应头长度不足: %d", len(responseHeader))
	}

	// 设置 AEAD 加密数据传输
	// 派生数据传输密钥 (与响应头使用相同派生路径)
	dataKey := vmessDataKey(requestBodyKey)
	dataIV := vmessDataIV(requestBodyIV)

	vc := &vmessConn{
		Conn:       conn,
		requestKey: requestBodyKey,
	}

	// 创建数据流 AEAD 加密器
	dataAEAD, err := CreateAEAD(dataKey)
	if err != nil {
		return nil, fmt.Errorf("创建数据 AEAD 失败: %w", err)
	}

	// 设置 AEAD 读写器
	// 初始化 data stream nonce: 前 8 字节用派生的 dataIV 填充
	streamNonce := make([]byte, dataAEAD.NonceSize())
	copy(streamNonce[:8], dataIV[:8])

	vc.reader = &aeadStreamReader{
		reader:  conn,
		aead:    dataAEAD,
		nonce:   make([]byte, dataAEAD.NonceSize()),
		readBuf: make([]byte, 0, 32*1024),
	}
	copy(vc.reader.(*aeadStreamReader).nonce, streamNonce)

	vc.writer = &aeadStreamWriter{
		writer:   conn,
		aead:     dataAEAD,
		nonce:    make([]byte, dataAEAD.NonceSize()),
		writeBuf: make([]byte, 0, 32*1024),
	}
	copy(vc.writer.(*aeadStreamWriter).nonce, streamNonce)

	return vc, nil
}

// tlsWrap TLS 封装
func (a *Adapter) tlsWrap(conn net.Conn) net.Conn {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
	})
	return tlsConn
}

// wsWrap WebSocket 封装
func (a *Adapter) wsWrap(conn net.Conn) net.Conn {
	host := a.wsHost
	if host == "" {
		host = a.server
	}
	headers := map[string]string{
		"Host": host,
	}
	wsConn, err := transport.NewWebSocketConn(conn, a.wsPath, host, headers)
	if err != nil {
		conn.Close()
		return conn
	}
	return wsConn
}

// parseUUID 解析 UUID
func parseUUID(s string) ([16]byte, error) {
	var uuid [16]byte
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return uuid, fmt.Errorf("无效的 UUID: %s", s)
	}

	for i := 0; i < 16; i++ {
		b, err := hexToByte(s[i*2 : i*2+2])
		if err != nil {
			return uuid, err
		}
		uuid[i] = b
	}

	return uuid, nil
}

func hexToByte(s string) (byte, error) {
	var b byte
	for _, c := range s {
		b <<= 4
		switch {
		case '0' <= c && c <= '9':
			b |= byte(c - '0')
		case 'a' <= c && c <= 'f':
			b |= byte(c - 'a' + 10)
		case 'A' <= c && c <= 'F':
			b |= byte(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("无效的 hex: %c", c)
		}
	}
	return b, nil
}

// KDF 密钥派生 (HMAC-SHA256)
func kdf(key []byte, path ...[]byte) []byte {
	h := hmac.New(sha256.New, key)
	for _, p := range path {
		h.Write(p)
	}
	return h.Sum(nil)
}

// vmessConn VMess AEAD 加密连接
type vmessConn struct {
	net.Conn
	reader     io.Reader
	writer     io.Writer
	requestKey []byte
}

// Read 读取 (AEAD 解密)
func (c *vmessConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

// Write 写入 (AEAD 加密)
func (c *vmessConn) Write(b []byte) (int, error) {
	return c.writer.Write(b)
}

// aeadStreamReader AEAD 流式读取器
// 数据格式: chunkSize(2, 加密) + encryptedPayload(chunkSize) + tag(16)
type aeadStreamReader struct {
	reader  io.Reader
	aead    cipher.AEAD
	nonce   []byte
	readBuf []byte
	offset  int
	mu      sync.Mutex
}

// Read 实现 io.Reader，解密 AEAD 数据帧
func (r *aeadStreamReader) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 如果缓冲区中有数据，直接返回
	if r.offset < len(r.readBuf) {
		n := copy(b, r.readBuf[r.offset:])
		r.offset += n
		return n, nil
	}

	// 读取帧大小 (2 bytes, 明文)
	sizeBuf := make([]byte, 2)
	if _, err := io.ReadFull(r.reader, sizeBuf); err != nil {
		return 0, err
	}

	chunkSize := binary.BigEndian.Uint16(sizeBuf)
	if chunkSize == 0 {
		return 0, io.EOF
	}

	// 读取加密数据 (ciphertext + tag)
	encryptedData := make([]byte, int(chunkSize)+r.aead.Overhead())
	if _, err := io.ReadFull(r.reader, encryptedData); err != nil {
		return 0, err
	}

	// 解密
	plaintext, err := r.aead.Open(nil, r.nonce, encryptedData, nil)
	if err != nil {
		return 0, fmt.Errorf("AEAD 解密失败: %w", err)
	}

	// 递增 nonce (2字节计数器 + responseIV 填充)
	incrementNonce(r.nonce)

	// 复制到输出缓冲区
	n := copy(b, plaintext)
	if n < len(plaintext) {
		r.readBuf = plaintext
		r.offset = n
	} else {
		r.readBuf = r.readBuf[:0]
		r.offset = 0
	}

	return n, nil
}

// aeadStreamWriter AEAD 流式写入器
// 数据格式: chunkSize(2, 明文) + encryptedPayload + tag(16)
type aeadStreamWriter struct {
	writer   io.Writer
	aead     cipher.AEAD
	nonce    []byte
	writeBuf []byte
	mu       sync.Mutex
}

// Write 实现 io.Writer，加密并发送数据帧
func (w *aeadStreamWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 加密数据
	encrypted := w.aead.Seal(nil, w.nonce, b, nil)

	// 递增 nonce
	incrementNonce(w.nonce)

	// 写入帧大小 (2 bytes, 明文)
	sizeBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(sizeBuf, uint16(len(b)))

	// 发送: sizeBuf + encrypted
	wireData := make([]byte, 2+len(encrypted))
	copy(wireData, sizeBuf)
	copy(wireData[2:], encrypted)

	if _, err := w.writer.Write(wireData); err != nil {
		return 0, err
	}

	return len(b), nil
}

// incrementNonce 递增 nonce (2字节计数器，大端序)
func incrementNonce(nonce []byte) {
	if len(nonce) >= 2 {
		binary.BigEndian.PutUint16(nonce[:2], binary.BigEndian.Uint16(nonce[:2])+1)
	}
}

// packAddress 打包地址（安全拷贝，不引用 pool buffer）
func packAddress(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		buf[offset] = 2 // 域名类型
		offset++
		domain := []byte(host)
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(domain)))
		offset += 2
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		buf[offset] = 1 // IPv4
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		buf[offset] = 3 // IPv6
		offset++
		copy(buf[offset:], ip.To16())
		offset += 16
	}

	// 端口
	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	// 安全拷贝到新切片，避免 pool buffer 被回收后数据失效
	result := make([]byte, offset)
	copy(result, buf[:offset])
	return result
}

// 协议头部长度
const (
	headerAddrTypeIPv4   = 1
	headerAddrTypeDomain = 2
	headerAddrTypeIPv6   = 3
)
