// Package vless VLESS 协议实现
package vless

import (
	"context"
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
	// Version VLESS 协议版本
	Version byte = 0

	// AddonTypeFlow XTLS Flow 类型
	AddonTypeFlow byte = 0x0a
)

// Flow 类型
const (
	FlowVision  = "xtls-rprx-vision"
	FlowReality = "reality"
)

// Adapter VLESS 适配器
type Adapter struct {
	name     string
	server   string
	port     int
	uuid     string
	flow     string
	tls      bool
	sni      string
	network  string
	wsPath   string
	wsHost   string
	grpcName string
	fingerprint string
	dialer   *net.Dialer
}

// NewAdapter 创建 VLESS 适配器
func NewAdapter(name, server string, port int, uuid string, opts ...Option) (*Adapter, error) {
	a := &Adapter{
		name:    name,
		server:  server,
		port:    port,
		uuid:    uuid,
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

// WithFlow 设置 Flow
func WithFlow(flow string) Option {
	return func(a *Adapter) {
		a.flow = flow
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

// WithReality 设置 Reality
func WithReality(sni string) Option {
	return func(a *Adapter) {
		a.flow = FlowReality
		a.sni = sni
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeVLESS }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool {
	// 仅 TCP 网络支持 UDP
	return a.network == "tcp" || a.network == "grpc"
}

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// 连接服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 VLESS 服务器失败: %w", err)
	}

	// TLS 封装
	if a.tls {
		conn = a.tlsWrap(conn)
	}

	// 传输层封装
	switch a.network {
	case "ws":
		conn = a.wsWrap(conn)
	case "grpc":
		conn = a.grpcWrap(conn)
	}

	// VLESS 握手
	if err := a.handshake(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}

	return &vlessConn{Conn: conn}, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("VLESS UDP 尚未实现")
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

// handshake VLESS 握手
func (a *Adapter) handshake(conn net.Conn, metadata *adapter.Metadata) error {
	uuid, err := parseUUID(a.uuid)
	if err != nil {
		return fmt.Errorf("解析 UUID 失败: %w", err)
	}

	buf := pool.GetMedium()
	defer pool.Put(buf)
	offset := 0

	// 版本号
	buf[offset] = Version
	offset++

	// UUID (16 bytes)
	copy(buf[offset:], uuid[:])
	offset += 16

	// 附加信息
	addonLen := byte(0)
	if a.flow != "" {
		addonLen = 1 + byte(len(a.flow))
	}

	// 附加信息长度
	buf[offset] = addonLen
	offset++

	// 附加信息
	if a.flow != "" {
		buf[offset] = AddonTypeFlow
		offset++
		copy(buf[offset:], []byte(a.flow))
		offset += len(a.flow)
	}

	// 目标地址
	addr := packAddress(metadata.Host, metadata.DstPort)
	copy(buf[offset:], addr)
	offset += len(addr)

	// 发送
	_, err = conn.Write(buf[:offset])
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

// grpcWrap gRPC 封装
func (a *Adapter) grpcWrap(conn net.Conn) net.Conn {
	// TODO: gRPC 封装
	return conn
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

// packAddress 打包地址
func packAddress(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		buf[offset] = 2
		offset++
		domain := []byte(host)
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(domain)))
		offset += 2
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		buf[offset] = 1
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		buf[offset] = 3
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

	switch data[0] {
	case 1: // IPv4
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	case 2: // 域名
		domainLen := int(binary.BigEndian.Uint16(data[1:3]))
		if len(data) < 3+domainLen+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[3 : 3+domainLen])
		offset = 3 + domainLen
	case 3: // IPv6
		if len(data) < 19 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:17]).String()
		offset = 17
	default:
		return "", 0, nil, fmt.Errorf("未知地址类型: %d", data[0])
	}

	port := binary.BigEndian.Uint16(data[offset:])
	remaining := data[offset+2:]

	return host, port, remaining, nil
}

// vlessConn VLESS 连接封装
type vlessConn struct {
	net.Conn
}

// Read 读取
func (c *vlessConn) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}

// Write 写入
func (c *vlessConn) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

// XTLS Vision 处理器
type VisionProcessor struct {
	conn   net.Conn
	reader io.Reader
	writer io.Writer
}

// NewVisionProcessor 创建 XTLS Vision 处理器
func NewVisionProcessor(conn net.Conn) *VisionProcessor {
	return &VisionProcessor{
		conn: conn,
	}
}

// Process 处理 Vision 流
func (v *VisionProcessor) Process() {
	// TODO: 实现 XTLS-RPRX-Vision 流处理
	// 包括 TLS-in-TLS 检测、直接转发优化等
}
