// Package mieru Mieru 协议适配器实现
// Mieru 是一种基于 TCP 的代理协议，支持端口跳跃 (port hopping) 和连接复用
package mieru

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/perf/pool"
)

// 协议常量
const (
	// mieruVersion 协议版本
	mieruVersion byte = 0x01
	// cmdConnect TCP 连接请求
	cmdConnect byte = 0x01
	// cmdUDPAssociate UDP 关联
	cmdUDPAssociate byte = 0x02
	// cmdKeepalive 保活
	cmdKeepalive byte = 0x03
	// cmdClose 关闭连接
	cmdClose byte = 0x04
	// statusOK 成功
	statusOK byte = 0x00
	// statusError 错误
	statusError byte = 0x01
	// headerSize 协议头大小 (version + cmd + id + length + checksum)
	headerSize = 12
	// maxPayloadSize 单次最大 payload
	maxPayloadSize = 16384
	// defaultPortRange 默认端口范围
	defaultPortRange = 10
)

// addressType 地址类型
const (
	addrIPv4   byte = 0x01
	addrDomain byte = 0x03
	addrIPv6   byte = 0x04
)

// Adapter Mieru 适配器
type Adapter struct {
	name      string
	server    string
	port      int
	password  string
	udp       bool
	portRange int // 端口跳跃范围
	dialer    *net.Dialer

	// 连接池
	mu    sync.Mutex
	conns map[string]net.Conn
}

// Option 配置选项
type Option func(*Adapter)

// WithPortRange 设置端口跳跃范围
func WithPortRange(r int) Option {
	return func(a *Adapter) {
		if r > 0 {
			a.portRange = r
		}
	}
}

// NewAdapter 创建 Mieru 适配器
func NewAdapter(name, server string, port int, password string, opts ...Option) (*Adapter, error) {
	if password == "" {
		return nil, fmt.Errorf("mieru: 密码不能为空")
	}

	a := &Adapter{
		name:      name,
		server:    server,
		port:      port,
		password:  password,
		udp:       false, // Mieru 当前版本不支持 UDP
		portRange: defaultPortRange,
		conns:     make(map[string]net.Conn),
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

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeMieru }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立 Mieru TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	target := metadata.DestinationAddress()

	log.Debug().
		Str("server", a.Addr()).
		Str("target", target).
		Msg("[Mieru] 建立连接")

	// 建立底层 TCP 连接 (支持端口跳跃)
	conn, err := a.dialWithPortHopping(ctx)
	if err != nil {
		return nil, fmt.Errorf("mieru: 连接服务器失败: %w", err)
	}

	// Mieru 握手
	if err := a.handshake(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("mieru: 握手失败: %w", err)
	}

	// 发送连接请求
	if err := a.writeConnect(conn, target); err != nil {
		conn.Close()
		return nil, fmt.Errorf("mieru: 发送连接请求失败: %w", err)
	}

	return &mieruConn{
		Conn:     conn,
		password: a.password,
	}, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("mieru: 不支持 UDP")
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

// dialWithPortHopping 带端口跳跃的拨号
// Mieru 支持端口跳跃以增强抗封锁能力，通过密码哈希确定端口偏移
func (a *Adapter) dialWithPortHopping(ctx context.Context) (net.Conn, error) {
	if a.portRange <= 1 {
		// 不使用端口跳跃
		return a.dialer.DialContext(ctx, "tcp", a.Addr())
	}

	// 计算基于时间的端口偏移
	// 每 60 秒切换一次端口
	timeSlot := time.Now().Unix() / 60
	portOffset := int(crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s:%d", a.password, timeSlot)))) % a.portRange
	targetPort := a.port + portOffset

	addr := fmt.Sprintf("%s:%d", a.server, targetPort)
	log.Debug().
		Str("addr", addr).
		Int("portOffset", portOffset).
		Int("timeSlot", int(timeSlot)).
		Msg("[Mieru] 端口跳跃连接")

	conn, err := a.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		// 端口跳跃失败时回退到基础端口
		log.Warn().Err(err).Msg("[Mieru] 端口跳跃失败，回退到基础端口")
		return a.dialer.DialContext(ctx, "tcp", a.Addr())
	}

	return conn, nil
}

// handshake Mieru 握手协议
// 握手格式: version(1) + magic(4) + client_id(8) + timestamp(8) + checksum(4)
func (a *Adapter) handshake(conn net.Conn) error {
	// 构建握手包
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	// 版本号
	buf[offset] = mieruVersion
	offset++

	// 魔数 (固定值，用于协议识别)
	magic := []byte{0x4D, 0x49, 0x45, 0x52} // "MIER"
	copy(buf[offset:], magic)
	offset += 4

	// 客户端 ID (随机生成)
	clientID := make([]byte, 8)
	rand.Read(clientID)
	copy(buf[offset:], clientID)
	offset += 8

	// 时间戳 (毫秒级)
	timestamp := time.Now().UnixMilli()
	binary.BigEndian.PutUint64(buf[offset:], uint64(timestamp))
	offset += 8

	// 校验和 (CRC32)
	checksum := crc32.ChecksumIEEE(buf[:offset])
	binary.BigEndian.PutUint32(buf[offset:], checksum)
	offset += 4

	// 发送握手包
	if _, err := conn.Write(buf[:offset]); err != nil {
		return fmt.Errorf("发送握手包失败: %w", err)
	}

	// 读取握手响应
	// 响应格式: version(1) + status(1) + server_id(8) + checksum(4)
	respSize := 14
	resp := make([]byte, respSize)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("读取握手响应失败: %w", err)
	}

	// 验证版本
	if resp[0] != mieruVersion {
		return fmt.Errorf("协议版本不匹配: 期望 %d, 收到 %d", mieruVersion, resp[0])
	}

	// 验证状态
	if resp[1] != statusOK {
		return fmt.Errorf("握手被拒绝, 状态码: 0x%02x", resp[1])
	}

	// 验证校验和
	expectedChecksum := binary.BigEndian.Uint32(resp[10:14])
	actualChecksum := crc32.ChecksumIEEE(resp[:10])
	if expectedChecksum != actualChecksum {
		return fmt.Errorf("握手响应校验和不匹配")
	}

	log.Debug().Msg("[Mieru] 握手成功")
	return nil
}

// writeConnect 发送连接请求
// 请求格式: version(1) + cmd(1) + session_id(4) + addr_type(1) + addr + port(2) + payload_len(2) + checksum(4)
func (a *Adapter) writeConnect(conn net.Conn, target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("解析目标地址失败: %w", err)
	}

	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("解析端口失败: %w", err)
	}

	buf := pool.GetMedium()
	defer pool.Put(buf)

	offset := 0

	// 版本号
	buf[offset] = mieruVersion
	offset++

	// 命令
	buf[offset] = cmdConnect
	offset++

	// 会话 ID (随机)
	sessionID := make([]byte, 4)
	rand.Read(sessionID)
	copy(buf[offset:], sessionID)
	offset += 4

	// 目标地址
	addr := packAddress(host, uint16(port))
	copy(buf[offset:], addr)
	offset += len(addr)

	// payload 长度 (连接请求无 payload)
	binary.BigEndian.PutUint16(buf[offset:], 0)
	offset += 2

	// 校验和
	checksum := crc32.ChecksumIEEE(buf[:offset])
	binary.BigEndian.PutUint32(buf[offset:], checksum)
	offset += 4

	// 发送
	if _, err := conn.Write(buf[:offset]); err != nil {
		return fmt.Errorf("发送连接请求失败: %w", err)
	}

	// 读取响应头
	// 响应格式: version(1) + status(1) + session_id(4) + payload_len(2) + checksum(4)
	respHeader := make([]byte, 12)
	if _, err := io.ReadFull(conn, respHeader); err != nil {
		return fmt.Errorf("读取连接响应失败: %w", err)
	}

	// 验证状态
	if respHeader[1] != statusOK {
		return fmt.Errorf("连接被拒绝, 状态码: 0x%02x", respHeader[1])
	}

	// 验证校验和
	expectedChecksum := binary.BigEndian.Uint32(respHeader[8:12])
	actualChecksum := crc32.ChecksumIEEE(respHeader[:8])
	if expectedChecksum != actualChecksum {
		return fmt.Errorf("连接响应校验和不匹配")
	}

	// 读取 payload (如果有)
	payloadLen := binary.BigEndian.Uint16(respHeader[6:8])
	if payloadLen > 0 {
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return fmt.Errorf("读取响应 payload 失败: %w", err)
		}
	}

	log.Debug().
		Str("target", target).
		Msg("[Mieru] 连接请求成功")

	return nil
}

// packAddress 打包目标地址 (SOCKS5 格式)
func packAddress(host string, port uint16) []byte {
	buf := pool.GetSmall()
	defer pool.Put(buf)

	offset := 0

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		domain := []byte(host)
		if len(domain) > 255 {
			domain = domain[:255]
		}
		buf[offset] = addrDomain
		offset++
		buf[offset] = byte(len(domain))
		offset++
		copy(buf[offset:], domain)
		offset += len(domain)
	} else if ip4 := ip.To4(); ip4 != nil {
		// IPv4
		buf[offset] = addrIPv4
		offset++
		copy(buf[offset:], ip4)
		offset += 4
	} else {
		// IPv6
		buf[offset] = addrIPv6
		offset++
		copy(buf[offset:], ip.To16())
		offset += 16
	}

	binary.BigEndian.PutUint16(buf[offset:], port)
	offset += 2

	// 安全拷贝到新切片
	result := make([]byte, offset)
	copy(result, buf[:offset])
	return result
}

// unpackAddress 解包目标地址
func unpackAddress(data []byte) (string, uint16, []byte, error) {
	if len(data) < 1 {
		return "", 0, nil, io.ErrShortBuffer
	}

	var host string
	var offset int

	switch data[0] {
	case addrIPv4:
		if len(data) < 7 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = net.IP(data[1:5]).String()
		offset = 5
	case addrDomain:
		if len(data) < 2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		domainLen := int(data[1])
		if len(data) < 2+domainLen+2 {
			return "", 0, nil, io.ErrShortBuffer
		}
		host = string(data[2 : 2+domainLen])
		offset = 2 + domainLen
	case addrIPv6:
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

// mieruConn Mieru 连接包装器
type mieruConn struct {
	net.Conn
	password string
	closed   bool
}

// Read 读取数据
func (c *mieruConn) Read(b []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *mieruConn) Write(b []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	return c.Conn.Write(b)
}

// Close 关闭连接
func (c *mieruConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true

	// 发送关闭命令
	closePacket := make([]byte, headerSize)
	closePacket[0] = mieruVersion
	closePacket[1] = cmdClose
	checksum := crc32.ChecksumIEEE(closePacket[:8])
	binary.BigEndian.PutUint32(closePacket[8:], checksum)

	// 尝试发送，忽略错误
	c.Conn.Write(closePacket)

	return c.Conn.Close()
}
