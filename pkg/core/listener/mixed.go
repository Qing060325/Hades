// Package listener 混合端口监听器
package listener

import (
	"bufio"
	"bytes"
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
	"github.com/Qing060325/Hades/pkg/perf/zerocopy"
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
			addr:            addr,
			allowLan:        allowLan,
			adapterMgr:      adapterMgr,
			ruleEngine:      ruleEngine,
			groupMgr:        groupMgr,
			shutdownCh:      make(chan struct{}),
			shutdownTimeout: 5 * time.Second,
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
	// 跟踪活跃连接
	l.ConnStart()
	defer l.ConnDone()
	defer conn.Close()

	// 检查是否正在关闭
	if l.IsClosed() {
		return
	}

	// 设置读超时
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 使用 bufio.Reader 包装连接，确保 peek 的数据不会丢失
	reader := bufio.NewReaderSize(conn, 4096)
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

	// 构建 MultiReader：将 bufio 中已缓冲的数据与原始连接合并，
	// 确保 relay 不会丢失 bufio 缓冲区中的 body 数据
	bufferedConn := io.MultiReader(reader, conn)

	// 提取元数据
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeHTTP,
		NetWork: "tcp",
		InName: l.addr,
	}

	if req.Method == http.MethodConnect {
		// CONNECT 方法（HTTPS代理）
		host, port := parseHostPort(req.Host)
		metadata.Host = host
		metadata.DstPort = port
		l.handleConnect(bufferedConn, conn, req, metadata)
	} else {
		// 普通 HTTP 请求
		l.handleHTTPRequest(bufferedConn, conn, req, metadata)
	}
}

// handleConnect 处理CONNECT请求
// conn: 合并了 bufio 缓冲数据的 reader（用于读取 TLS 数据）
// rawConn: 原始连接（用于写入响应和作为 net.Conn）
func (l *MixedListener) handleConnect(conn io.Reader, rawConn net.Conn, req *http.Request, metadata *adapter.Metadata) {
	log.Debug().Str("target", req.Host).Msg("[Mixed] CONNECT请求")

	// 响应客户端：连接已建立
	rawConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 通过 Tunnel 调度（Tunnel 内部处理 DNS、规则匹配、适配器选择、转发）
	// 包装 rawConn 为 bufferedConn 以保留 bufio 中的缓冲数据
	bufferedConn := &bufferedWrapper{Reader: conn, Conn: rawConn}
	l.DispatchTCP(bufferedConn, metadata)
}

// bufferedWrapper 将 io.Reader 和 net.Conn 合并为 net.Conn
// 用于传递包含 bufio 缓冲数据的连接给 Tunnel
type bufferedWrapper struct {
	io.Reader
	net.Conn
}

func (bw *bufferedWrapper) Read(p []byte) (int, error) {
	return bw.Reader.Read(p)
}

// handleHTTPRequest 处理普通HTTP请求
// bufferedConn: 合并了 bufio 缓冲数据的 reader（包含可能的 body 数据）
// rawConn: 原始连接（用于写入和作为 net.Conn）
func (l *MixedListener) handleHTTPRequest(bufferedConn io.Reader, rawConn net.Conn, req *http.Request, metadata *adapter.Metadata) {
	metadata.Host = req.Host
	metadata.DstPort = 80

	// 将 HTTP 请求序列化到缓冲区，和 rawConn 合并
	var reqBuf bytes.Buffer
	req.Write(&reqBuf)
	combined := io.MultiReader(&reqBuf, bufferedConn)

	// 通过 Tunnel 调度
	wrappedConn := &bufferedWrapper{Reader: combined, Conn: rawConn}
	l.DispatchTCP(wrappedConn, metadata)
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
	if cmd != 0x01 && cmd != 0x03 { // CONNECT (0x01) 和 UDP ASSOCIATE (0x03)
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

	// 根据命令类型处理
	switch cmd {
	case 0x01: // CONNECT
		l.handleSOCKS5Connect(conn, reader, host, port)
	case 0x03: // UDP ASSOCIATE
		l.handleSOCKS5UDPAssociate(conn, reader, host, port)
	}
}

// handleSOCKS5Connect 处理 SOCKS5 CONNECT 命令
func (l *MixedListener) handleSOCKS5Connect(conn net.Conn, reader *bufio.Reader, host string, port uint16) {
	// 构建元数据
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeSOCKS,
		NetWork: "tcp",
		Host:    host,
		DstPort: port,
		InName: l.addr,
	}

	log.Debug().Str("target", fmt.Sprintf("%s:%d", host, port)).Msg("[Mixed] SOCKS5 CONNECT请求")

	// 响应成功
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// SOCKS5 握手后 bufio 可能缓冲了部分 payload 数据
	bufferedConn := &bufferedWrapper{Reader: io.MultiReader(reader, conn), Conn: conn}

	// 通过 Tunnel 调度
	l.DispatchTCP(bufferedConn, metadata)
}

// handleSOCKS5UDPAssociate 处理 SOCKS5 UDP ASSOCIATE 命令
// 创建 UDP 中继套接字，将地址/端口告知客户端，然后在客户端和代理之间转发 UDP 数据包
func (l *MixedListener) handleSOCKS5UDPAssociate(conn net.Conn, reader *bufio.Reader, clientHost string, clientPort uint16) {
	log.Debug().
		Str("client", fmt.Sprintf("%s:%d", clientHost, clientPort)).
		Msg("[Mixed] SOCKS5 UDP ASSOCIATE 请求")

	// 获取客户端的 TCP 连接地址，用于确定 UDP 回复目标
	tcpAddr := conn.RemoteAddr().(*net.TCPAddr)
	clientIP := tcpAddr.IP

	// 创建 UDP 中继监听套接字
	// 绑定到与 TCP 监听器相同的地址
	udpAddr, err := net.ResolveUDPAddr("udp", l.listener.Addr().String())
	if err != nil {
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Error().Err(err).Msg("[Mixed] 创建 UDP 中继套接字失败")
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer udpConn.Close()

	// 构建回复的 BND.ADDR 和 BND.PORT
	// 使用 UDP 套接字的地址
	relayAddr := udpConn.LocalAddr().(*net.UDPAddr)

	// 构建 SOCKS5 成功响应
	// VER=0x05, REP=0x00(成功), RSV=0x00, ATYP=0x01(IPv4)
	var reply []byte
	if relayAddr.IP.To4() != nil {
		reply = make([]byte, 10)
		reply[0] = 0x05
		reply[1] = 0x00
		reply[2] = 0x00
		reply[3] = 0x01 // IPv4
		copy(reply[4:8], relayAddr.IP.To4())
		reply[8] = byte(relayAddr.Port >> 8)
		reply[9] = byte(relayAddr.Port)
	} else {
		reply = make([]byte, 22)
		reply[0] = 0x05
		reply[1] = 0x00
		reply[2] = 0x00
		reply[3] = 0x04 // IPv6
		copy(reply[4:20], relayAddr.IP.To16())
		reply[20] = byte(relayAddr.Port >> 8)
		reply[21] = byte(relayAddr.Port)
	}

	if _, err := conn.Write(reply); err != nil {
		log.Error().Err(err).Msg("[Mixed] 发送 UDP ASSOCIATE 响应失败")
		return
	}

	log.Debug().
		Str("relay_addr", relayAddr.String()).
		Msg("[Mixed] UDP ASSOCIATE 中继已建立")

	// UDP 中继循环
	// SOCKS5 UDP 数据包格式:
	// +----+------+------+----------+----------+----------+
	// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
	// +----+------+------+----------+----------+----------+
	// | 2  |  1   |  1   | Variable |    2     | Variable |
	// +----+------+------+----------+----------+----------+

	done := make(chan struct{})
	defer close(done)

	// 监控 TCP 连接，客户端断开时停止 UDP 中继
	go func() {
		// 读取 TCP 连接，等待客户端断开
		buf := make([]byte, 1)
		conn.Read(buf) // 会阻塞直到客户端断开
		udpConn.Close()
	}()

	// UDP 中继主循环
	udpBuf := make([]byte, 65535)
	for {
		n, clientUDPAddr, err := udpConn.ReadFromUDP(udpBuf)
		if err != nil {
			select {
			case <-done:
				return
			default:
				return
			}
		}

		// 验证 UDP 数据包来自正确的客户端
		if !clientIP.Equal(clientUDPAddr.IP) {
			// RFC 1928: UDP 数据包必须来自与 TCP 连接相同的客户端
			log.Debug().
				Str("expected", clientIP.String()).
				Str("got", clientUDPAddr.IP.String()).
				Msg("[Mixed] UDP ASSOCIATE 丢弃来自未知地址的数据包")
			continue
		}

		// 解析 SOCKS5 UDP 头部
		if n < 10 { // 最小: RSV(2) + FRAG(1) + ATYP(1) + IPv4(4) + PORT(2)
			continue
		}

		// RSV: 0x0000
		if udpBuf[0] != 0x00 || udpBuf[1] != 0x00 {
			continue
		}

		// FRAG: 仅支持 0（无分片）
		if udpBuf[2] != 0x00 {
			continue
		}

		atyp := udpBuf[3]
		var dstHost string
		var dstPort uint16
		var dataOffset int

		switch atyp {
		case 0x01: // IPv4
			if n < 10 {
				continue
			}
			dstHost = net.IP(udpBuf[4:8]).String()
			dstPort = uint16(udpBuf[8])<<8 | uint16(udpBuf[9])
			dataOffset = 10
		case 0x03: // 域名
			if n < 7 {
				continue
			}
			domainLen := int(udpBuf[4])
			if n < 5+domainLen+2 {
				continue
			}
			dstHost = string(udpBuf[5 : 5+domainLen])
			portOff := 5 + domainLen
			dstPort = uint16(udpBuf[portOff])<<8 | uint16(udpBuf[portOff+1])
			dataOffset = portOff + 2
		case 0x04: // IPv6
			if n < 22 {
				continue
			}
			dstHost = net.IP(udpBuf[4:20]).String()
			dstPort = uint16(udpBuf[20])<<8 | uint16(udpBuf[21])
			dataOffset = 22
		default:
			continue
		}

		if dataOffset >= n {
			continue
		}

		payload := udpBuf[dataOffset:n]

		// 构建元数据
		metadata := &adapter.Metadata{
			Type:    adapter.MetadataTypeSOCKS,
			NetWork: "udp",
			Host:    dstHost,
			DstPort: dstPort,
			InName:  l.addr,
		}

		// 选择适配器
		adapt := l.SelectAdapter(metadata)
		if adapt == nil || !adapt.SupportUDP() {
			continue
		}

		// 异步转发 UDP 数据包
		go func(data []byte, dst string, port uint16, m *adapter.Metadata, a adapter.Adapter, replyAddr *net.UDPAddr) {
			packetConn, err := a.DialUDPContext(context.Background(), m)
			if err != nil {
				log.Debug().Err(err).Msg("[Mixed] UDP ASSOCIATE dial 失败")
				return
			}
			defer packetConn.Close()

			// 发送到目标
			dstUDPAddr := &net.UDPAddr{
				IP:   net.ParseIP(dst),
				Port: int(port),
			}

			_, err = packetConn.WriteTo(data, dstUDPAddr)
			if err != nil {
				return
			}

			// 等待响应
			packetConn.SetReadDeadline(time.Now().Add(30 * time.Second))
			respBuf := make([]byte, 65535)
			n, _, err := packetConn.ReadFrom(respBuf)
			if err != nil {
				return
			}

			// 封装 SOCKS5 UDP 响应头
			var respHeader []byte
			dstIP := net.ParseIP(dst)
			if dstIP.To4() != nil {
				respHeader = make([]byte, 10)
				respHeader[0] = 0x00 // RSV
				respHeader[1] = 0x00 // RSV
				respHeader[2] = 0x00 // FRAG
				respHeader[3] = 0x01 // ATYP IPv4
				copy(respHeader[4:8], dstIP.To4())
				respHeader[8] = byte(port >> 8)
				respHeader[9] = byte(port)
			} else if dstIP.To16() != nil {
				respHeader = make([]byte, 22)
				respHeader[0] = 0x00
				respHeader[1] = 0x00
				respHeader[2] = 0x00
				respHeader[3] = 0x04 // ATYP IPv6
				copy(respHeader[4:20], dstIP.To16())
				respHeader[20] = byte(port >> 8)
				respHeader[21] = byte(port)
			} else {
				// 域名
				domainBytes := []byte(dst)
				respHeader = make([]byte, 5+len(domainBytes)+2)
				respHeader[0] = 0x00
				respHeader[1] = 0x00
				respHeader[2] = 0x00
				respHeader[3] = 0x03
				respHeader[4] = byte(len(domainBytes))
				copy(respHeader[5:5+len(domainBytes)], domainBytes)
				portOff := 5 + len(domainBytes)
				respHeader[portOff] = byte(port >> 8)
				respHeader[portOff+1] = byte(port)
			}

			// 组装完整 UDP 响应包
			fullResp := append(respHeader, respBuf[:n]...)

			// 发回客户端
			udpConn.WriteToUDP(fullResp, replyAddr)
		}(payload, dstHost, dstPort, metadata, adapt, clientUDPAddr)
	}
}

// relay 双向转发
// leftReader: 左侧读取源（可能是 bufio + conn 的 MultiReader）
// leftConn: 左侧完整连接（用于写入和 splice）
// rightConn: 右侧完整连接
// 优先使用 splice 零拷贝（Linux），否则回退到 io.CopyBuffer
func (l *MixedListener) relay(leftReader io.Reader, leftConn net.Conn, rightConn net.Conn) {
	// 尝试 TCP 零拷贝（仅当 leftReader 就是 leftConn 时可用 splice）
	if lc, lok := leftConn.(*net.TCPConn); lok {
		if rc, rok := rightConn.(*net.TCPConn); rok {
			if leftReader == leftConn {
				zerocopy.TCPRelay(lc, rc)
				return
			}
		}
	}

	// 回退到缓冲拷贝
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(leftConn, rightConn, buf)
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(rightConn, leftReader, buf)
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
				addr:            addr,
				allowLan:        allowLan,
				adapterMgr:      adapterMgr,
				ruleEngine:      ruleEngine,
				groupMgr:        groupMgr,
				shutdownCh:      make(chan struct{}),
				shutdownTimeout: 5 * time.Second,
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
				addr:            addr,
				allowLan:        allowLan,
				adapterMgr:      adapterMgr,
				ruleEngine:      ruleEngine,
				groupMgr:        groupMgr,
				shutdownCh:      make(chan struct{}),
				shutdownTimeout: 5 * time.Second,
			},
		},
	}
}
