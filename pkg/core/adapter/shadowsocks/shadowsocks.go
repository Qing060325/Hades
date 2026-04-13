// Package shadowsocks Shadowsocks 协议实现
package shadowsocks

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hades/hades/pkg/core/adapter"
	"github.com/hades/hades/pkg/perf/pool"
)

const (
	// saltSize AEAD salt 长度
	saltSize = 32
	// nonceSize AEAD nonce 长度
	nonceSize = 12
	// keySizeMax 最大密钥长度
	keySizeMax = 64
	// payloadSizeLen payload 长度字段
	payloadSizeLen = 2
	// tagSize AEAD 认证标签长度
	tagSize = 16
	// maxPayloadSize 单次加密最大 payload
	maxPayloadSize = 16384 - tagSize
)

// CipherInfo 加密方式信息
type CipherInfo struct {
	KeySize int
	AEAD    bool
	SKIP    bool
}

// 支持的加密方式
var supportedCiphers = map[string]CipherInfo{
	// AEAD 加密
	"aes-128-gcm":        {KeySize: 16, AEAD: true},
	"aes-192-gcm":        {KeySize: 24, AEAD: true},
	"aes-256-gcm":        {KeySize: 32, AEAD: true},
	"chacha20-ietf-poly1305": {KeySize: 32, AEAD: true},
	"xchacha20-ietf-poly1305": {KeySize: 32, AEAD: true},
	"2022-blake3-aes-128-gcm":   {KeySize: 32, AEAD: true},
	"2022-blake3-aes-256-gcm":   {KeySize: 64, AEAD: true},
	"2022-blake3-chacha20-poly1305": {KeySize: 64, AEAD: true},
}

// IsAEAD 判断是否为 AEAD 加密
func IsAEAD(cipher string) bool {
	info, ok := supportedCiphers[cipher]
	return ok && info.AEAD
}

// Cipher 加密器接口
type Cipher interface {
	KeySize() int
	Encrypt(dst, src []byte) ([]byte, error)
	Decrypt(dst, src []byte) ([]byte, error)
}

// Adapter Shadowsocks 适配器
type Adapter struct {
	name     string
	server   string
	port     int
	cipher   string
	password string
	udp      bool
	dialer   *net.Dialer
}

// NewAdapter 创建 Shadowsocks 适配器
func NewAdapter(name, server string, port int, cipher, password string) (*Adapter, error) {
	if _, ok := supportedCiphers[cipher]; !ok {
		return nil, fmt.Errorf("不支持的加密方式: %s", cipher)
	}

	return &Adapter{
		name:     name,
		server:   server,
		port:     port,
		cipher:   cipher,
		password: password,
		udp:      true,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}, nil
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeShadowsocks }

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
		return nil, fmt.Errorf("连接 SS 服务器失败: %w", err)
	}

	// 创建 AEAD 加密流
	cipher, err := newAEADCipher(a.cipher, a.password)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// 创建 Shadowsocks 连接
	ssConn := newSSConn(conn, cipher, clientMode)

	// 发送目标地址
	if err := ssConn.writeTarget(metadata); err != nil {
		conn.Close()
		return nil, fmt.Errorf("发送目标地址失败: %w", err)
	}

	return ssConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("SS UDP 尚未实现")
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

// 连接模式
const (
	clientMode = iota
	serverMode
)

// ssConn Shadowsocks 加密连接
type ssConn struct {
	net.Conn
	cipher    *aeadCipher
	mode      int
	writeSalt []byte
	readSalt  []byte
	writeNonce uint64
	readNonce  uint64
}

func newSSConn(conn net.Conn, cipher *aeadCipher, mode int) *ssConn {
	return &ssConn{
		Conn:   conn,
		cipher: cipher,
		mode:   mode,
	}
}

// writeTarget 发送目标地址
func (c *ssConn) writeTarget(metadata *adapter.Metadata) error {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	// 生成并写入 salt
	salt := make([]byte, saltSize)
	rand.Read(salt)
	copy(buf[:saltSize], salt)
	c.writeSalt = salt

	// 准备目标地址
	target := packAddr(metadata.Host, metadata.DstPort)

	// AEAD 加密目标地址
	encrypted, err := c.cipher.encryptPayload(salt, c.writeNonce, target)
	if err != nil {
		return err
	}
	c.writeNonce++

	// salt + encrypted_target
	fullBuf := make([]byte, saltSize+len(encrypted))
	copy(fullBuf[:saltSize], salt)
	copy(fullBuf[saltSize:], encrypted)

	_, err = c.Conn.Write(fullBuf)
	return err
}

// Read 重写 Read 方法
func (c *ssConn) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}

// Write 重写 Write 方法
func (c *ssConn) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

// packAddr 打包目标地址 (SOCKS5 格式)
func packAddr(host string, port uint16) []byte {
	buf := make([]byte, 0, 64)

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		buf = append(buf, 0x03)
		buf = append(buf, byte(len(host)))
		buf = append(buf, []byte(host)...)
	} else if ip4 := ip.To4(); ip4 != nil {
		// IPv4
		buf = append(buf, 0x01)
		buf = append(buf, ip4...)
	} else {
		// IPv6
		buf = append(buf, 0x04)
		buf = append(buf, ip.To16()...)
	}

	// 端口 (大端)
	buf = append(buf, byte(port>>8), byte(port))

	return buf
}

// unpackAddr 解包目标地址
func unpackAddr(data []byte) (string, uint16, []byte, error) {
	if len(data) < 1 {
		return "", 0, nil, io.ErrShortBuffer
	}

	var host string
	var offset int

	switch data[0] {
	case 0x01: // IPv4
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	case 0x03: // 域名
		domainLen := int(data[1])
		if len(data) < 2+domainLen+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[2 : 2+domainLen])
		offset = 2 + domainLen
	case 0x04: // IPv6
		if len(data) < 19 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:17]).String()
		offset = 17
	default:
		return "", 0, nil, fmt.Errorf("未知的地址类型: %d", data[0])
	}

	port := binary.BigEndian.Uint16(data[offset:])
	remaining := data[offset+2:]

	return host, port, remaining, nil
}
