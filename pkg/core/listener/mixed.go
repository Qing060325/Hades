// Package listener 混合端口监听器
package listener

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/rs/zerolog/log"
)

// MixedListener 混合端口监听器（HTTP + SOCKS5）
type MixedListener struct {
	BaseListener
}

// NewMixedListener 创建混合监听器
func NewMixedListener(
	addr string,
	allowLan bool,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) *MixedListener {
	return &MixedListener{
		BaseListener: BaseListener{
			addr:       addr,
			allowLan:   allowLan,
			adapterMgr: adapterMgr,
			ruleEngine: ruleEngine,
			groupMgr:   groupMgr,
		},
	}
}

// Listen 开始监听
func (l *MixedListener) Listen(ctx context.Context) error {
	var err error

	// 将 * 转换为 0.0.0.0
	listenAddr := l.addr
	if strings.HasPrefix(listenAddr, "*:") {
		listenAddr = "0.0.0.0" + listenAddr[1:]
	} else if listenAddr[0] == ':' {
		listenAddr = "0.0.0.0" + listenAddr
	}

	l.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}

	log.Info().Str("addr", l.addr).Msg("混合端口监听器已启动")

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			if l.closed {
				return nil
			}
			log.Error().Err(err).Msg("接受连接失败")
			continue
		}

		// 检查局域网访问
		if !l.allowLan && !l.isLocal(conn.RemoteAddr()) {
			conn.Close()
			continue
		}

		go l.handleConnection(conn)
	}
}

// handleConnection 处理连接
func (l *MixedListener) handleConnection(conn net.Conn) {
	defer conn.Close()

	// 设置读超时
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 检测协议类型
	reader := bufio.NewReader(conn)
	header, err := reader.Peek(1)
	if err != nil {
		log.Debug().Err(err).Msg("读取协议头失败")
		return
	}

	conn.SetReadDeadline(time.Time{}) // 重置超时

	// 根据第一个字节判断协议
	// SOCKS5 以 0x05 开头
	// HTTP 以方法名开头 (GET, POST, CONNECT, etc.)
	if header[0] == 0x05 {
		// SOCKS5 协议
		l.handleSOCKS5(conn, reader)
	} else {
		// HTTP 协议
		l.handleHTTP(conn, reader)
	}
}

// handleHTTP 处理HTTP请求
func (l *MixedListener) handleHTTP(conn net.Conn, reader *bufio.Reader) {
	req, err := http.ReadRequest(reader)
	if err != nil {
		log.Debug().Err(err).Msg("读取HTTP请求失败")
		return
	}

	// 提取元数据
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeHTTP,
		Network: "tcp",
		Inbound: l.addr,
	}

	if req.Method == http.MethodConnect {
		// CONNECT 方法（HTTPS代理）
		host, port := parseHostPort(req.Host)
		metadata.Host = host
		metadata.DstPort = port
		l.handleConnect(conn, req, metadata)
	} else {
		// 普通 HTTP 请求
		l.handleHTTPRequest(conn, req, metadata)
	}
}

// handleConnect 处理CONNECT请求
func (l *MixedListener) handleConnect(conn net.Conn, req *http.Request, metadata *adapter.Metadata) {
	// 选择适配器
	adapt := l.SelectAdapter(metadata)
	if adapt == nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	log.Debug().Str("adapter", adapt.Name()).Str("target", req.Host).Msg("[Mixed] CONNECT请求")

	// 建立后端连接
	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("建立后端连接失败")
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer backendConn.Close()

	// 响应客户端
	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 双向转发
	l.relay(conn, backendConn)
}

// handleHTTPRequest 处理普通HTTP请求
func (l *MixedListener) handleHTTPRequest(conn net.Conn, req *http.Request, metadata *adapter.Metadata) {
	// 提取目标地址
	host := req.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}

	metadata.Host = req.Host
	metadata.DstPort = 80

	// 选择适配器
	adapt := l.SelectAdapter(metadata)
	if adapt == nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// 建立后端连接
	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer backendConn.Close()

	// 转发请求
	req.Write(backendConn)

	// 双向转发
	l.relay(conn, backendConn)
}

// handleSOCKS5 处理SOCKS5请求
func (l *MixedListener) handleSOCKS5(conn net.Conn, reader *bufio.Reader) {
	// SOCKS5 握手
	// 读取客户端问候
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return
	}

	if header[0] != 0x05 {
		return
	}

	// 读取认证方法
	methods := make([]byte, header[1])
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}

	// 响应：无需认证
	conn.Write([]byte{0x05, 0x00})

	// 读取连接请求
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, reqHeader); err != nil {
		return
	}

	if reqHeader[0] != 0x05 {
		return
	}

	cmd := reqHeader[1]
	if cmd != 0x01 { // 只支持 CONNECT
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 读取目标地址
	var host string
	var port uint16

	switch reqHeader[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03: // 域名
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(reader, lenByte); err != nil {
			return
		}
		domain := make([]byte, lenByte[0])
		if _, err := io.ReadFull(reader, domain); err != nil {
			return
		}
		host = string(domain)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 读取端口
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return
	}
	port = uint16(portBytes[0])<<8 | uint16(portBytes[1])

	// 构建元数据
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeSOCKS,
		Network: "tcp",
		Host:    host,
		DstPort: port,
		Inbound: l.addr,
	}

	log.Debug().Str("target", fmt.Sprintf("%s:%d", host, port)).Msg("[Mixed] SOCKS5请求")

	// 选择适配器
	adapt := l.SelectAdapter(metadata)
	if adapt == nil {
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 建立后端连接
	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("建立后端连接失败")
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer backendConn.Close()

	// 响应成功
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// 双向转发
	l.relay(conn, backendConn)
}

// relay 双向转发
func (l *MixedListener) relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
	}()

	wg.Wait()
}

// isLocal 检查是否为本地地址
func (l *BaseListener) isLocal(addr net.Addr) bool {
	if addr == nil {
		return false
	}

	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}

	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// parseHostPort 解析主机和端口
func parseHostPort(hostPort string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			return hostPort, 80
		}
		return hostPort, 0
	}

	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

// HTTPListener HTTP监听器
type HTTPListener struct {
	MixedListener
}

// NewHTTPListener 创建HTTP监听器
func NewHTTPListener(
	addr string,
	allowLan bool,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) *HTTPListener {
	return &HTTPListener{
		MixedListener: MixedListener{
			BaseListener: BaseListener{
				addr:       addr,
				allowLan:   allowLan,
				adapterMgr: adapterMgr,
				ruleEngine: ruleEngine,
				groupMgr:   groupMgr,
			},
		},
	}
}

// SOCKSListener SOCKS监听器
type SOCKSListener struct {
	MixedListener
}

// NewSOCKSListener 创建SOCKS监听器
func NewSOCKSListener(
	addr string,
	allowLan bool,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) *SOCKSListener {
	return &SOCKSListener{
		MixedListener: MixedListener{
			BaseListener: BaseListener{
				addr:       addr,
				allowLan:   allowLan,
				adapterMgr: adapterMgr,
				ruleEngine: ruleEngine,
				groupMgr:   groupMgr,
			},
		},
	}
}
