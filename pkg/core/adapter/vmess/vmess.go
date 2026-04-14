// Package vmess VMess 协议实现
package vmess

import (
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/hades/hades/pkg/core/adapter"
	"github.com/hades/hades/pkg/perf/pool"
	"github.com/hades/hades/pkg/transport"
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
	name     string
	server   string
	port     int
	uuid     string
	alterID  int
	cipher   string
	tls      bool
	sni      string
	network  string
	wsPath   string
	wsHost   string
	dialer   *net.Dialer
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
	var conn net.Conn
	var err error

	conn, err = a.dialer.DialContext(ctx, "tcp", a.Addr())
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
	if err := a.handshake(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
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

// handshake VMess 握手
func (a *Adapter) handshake(conn net.Conn, metadata *adapter.Metadata) error {
	uuid, err := parseUUID(a.uuid)
	if err != nil {
		return fmt.Errorf("解析 UUID 失败: %w", err)
	}

	// 生成随机 payload
	randBytes := make([]byte, 33)
	if _, err := rand.Read(randBytes); err != nil {
		return err
	}

	// 构建请求头
	hdr := &header{
		Version: Version,
		Cmd:     CmdTCP,
		Target: targetInfo{
			Address: metadata.Host,
			Port:    metadata.DstPort,
			Network: "tcp",
		},
		Options:  0,
		Security: 0, // AES-128-GCM
		Payload:  randBytes,
	}
	copy(hdr.UUID[:], uuid[:])

	// AEAD 加密握手
	if err := a.aeadHandshake(conn, hdr); err != nil {
		return fmt.Errorf("VMess AEAD 握手失败: %w", err)
	}

	return nil
}

// aeadHandshake AEAD 加密握手
func (a *Adapter) aeadHandshake(conn net.Conn, hdr *header) error {
	// 生成随机 key 和 iv
	key := make([]byte, 16) // AES-128
	iv := make([]byte, 16)
	rand.Read(key)
	rand.Read(iv)

	// 创建加密器
	_, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	// 构建 header 数据
	headerData := make([]byte, 0, 64)
	headerData = append(headerData, Version)
	headerData = append(headerData, hdr.UUID[:]...)

	// 构建完整的请求数据
	// 这里简化实现，实际 VMess AEAD 握手更复杂
	_, err = conn.Write(headerData)
	return err
}

// tlsWrap TLS 封装
func (a *Adapter) tlsWrap(conn net.Conn) net.Conn {
	serverName := a.sni
	if serverName == "" {
		serverName = a.server
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
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

// KDF 密钥派生
func kdf(key []byte, path ...[]byte) []byte {
	h := md5.New()
	h.Write(key)
	for _, p := range path {
		h.Write(p)
	}
	return h.Sum(nil)
}

// vmessConn VMess 加密连接
type vmessConn struct {
	net.Conn
	reader io.Reader
	writer io.Writer
}

// Read 读取
func (c *vmessConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

// Write 写入
func (c *vmessConn) Write(b []byte) (int, error) {
	return c.writer.Write(b)
}

// 协议辅助函数

// packAddress 打包地址
func packAddress(host string, port uint16, network string) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	// 地址类型 + 地址
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

	return buf[:offset]
}

// 协议头部长度
const (
	headerAddrTypeIPv4   = 1
	headerAddrTypeDomain = 2
	headerAddrTypeIPv6   = 3
)
