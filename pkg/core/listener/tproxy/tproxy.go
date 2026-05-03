// Package tproxy Linux TProxy 透明代理入站监听器
// 使用 IP_TRANSPARENT socket 选项实现透明代理，支持 TCP 和 UDP
package tproxy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/rs/zerolog/log"
)

const (
	// IP_TRANSPARENT 允许非本地源地址绑定
	IP_TRANSPARENT = 19
	// IP_RECVORIGDSTADDR 接收原始目标地址
	IP_RECVORIGDSTADDR = 29
	// SO_ORIGINAL_DST 获取原始目标
	SO_ORIGINAL_DST = 80
)

// TProxyListener TProxy 透明代理监听器
// 支持 TCP 和 UDP 的透明代理
type TProxyListener struct {
	addr       string
	allowLan   bool
	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager

	tcpListener net.Listener
	udpConn     *net.UDPConn

	closed     bool
	mu         sync.Mutex
	connWg     sync.WaitGroup
	shutdownCh chan struct{}
}

// NewTProxyListener 创建 TProxy 透明代理监听器
func NewTProxyListener(
	addr string,
	allowLan bool,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) *TProxyListener {
	return &TProxyListener{
		addr:       addr,
		allowLan:   allowLan,
		adapterMgr: adapterMgr,
		ruleEngine: ruleEngine,
		groupMgr:   groupMgr,
		shutdownCh: make(chan struct{}),
	}
}

// Listen 开始监听 TCP 和 UDP 连接
func (l *TProxyListener) Listen(ctx context.Context) error {
	// 启动 TCP TProxy 监听
	tcpListener, err := tproxyListenTCP(l.addr)
	if err != nil {
		return fmt.Errorf("tproxy tcp listen %s: %w", l.addr, err)
	}
	l.tcpListener = tcpListener
	log.Info().Str("addr", l.addr).Msg("TProxy TCP 监听器已启动")

	// 启动 UDP TProxy 监听
	udpConn, err := tproxyListenUDP(l.addr)
	if err != nil {
		tcpListener.Close()
		return fmt.Errorf("tproxy udp listen %s: %w", l.addr, err)
	}
	l.udpConn = udpConn
	log.Info().Str("addr", l.addr).Msg("TProxy UDP 监听器已启动")

	// 接受 TCP 连接
	l.connWg.Add(1)
	go l.acceptTCPLoop()

	// 接受 UDP 数据包
	l.connWg.Add(1)
	go l.handleUDPLoop()

	// 等待关闭信号
	<-l.shutdownCh
	return nil
}

// Close 关闭监听器
func (l *TProxyListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true
	close(l.shutdownCh)

	if l.tcpListener != nil {
		l.tcpListener.Close()
	}
	if l.udpConn != nil {
		l.udpConn.Close()
	}

	// 等待活跃连接完成
	done := make(chan struct{})
	go func() {
		l.connWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Warn().Msg("tproxy 等待活跃连接超时")
	}
	return nil
}

// Addr 返回监听地址
func (l *TProxyListener) Addr() net.Addr {
	if l.tcpListener != nil {
		return l.tcpListener.Addr()
	}
	return nil
}

// acceptTCPLoop TCP 连接接受循环
func (l *TProxyListener) acceptTCPLoop() {
	defer l.connWg.Done()

	for {
		conn, err := l.tcpListener.Accept()
		if err != nil {
			select {
			case <-l.shutdownCh:
				return
			default:
			}
			log.Error().Err(err).Msg("tproxy tcp accept 失败")
			continue
		}

		// 检查局域网访问
		if !l.allowLan && !l.isLocal(conn.RemoteAddr()) {
			conn.Close()
			continue
		}

		l.connWg.Add(1)
		go l.handleTCPConn(conn)
	}
}

// handleTCPConn 处理 TCP 透明代理连接
func (l *TProxyListener) handleTCPConn(conn net.Conn) {
	defer l.connWg.Done()
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}

	// 获取原始目标地址
	origDst, err := getOriginalDst(tcpConn)
	if err != nil {
		log.Debug().Err(err).Msg("tproxy 获取 TCP 原始目标失败")
		return
	}

	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeTProxy,
		SrcIP:   remoteAddr.AddrPort().Addr(),
		SrcPort: uint16(remoteAddr.Port),
		DstIP:   origDst.Addr(),
		DstPort: origDst.Port(),
		Host:    origDst.Addr().String(),
		NetWork: "tcp",
		InName:  l.addr,
	}

	// 选择适配器
	adapt := l.selectAdapter(metadata)
	if adapt == nil {
		log.Debug().Str("dst", origDst.String()).Msg("tproxy 无可用适配器")
		return
	}

	log.Debug().
		Str("src", remoteAddr.String()).
		Str("dst", origDst.String()).
		Str("adapter", adapt.Name()).
		Msg("tproxy tcp 连接")

	// 连接到原始目标（通过代理）
	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("tproxy tcp dial 失败")
		return
	}
	defer backendConn.Close()

	// 双向转发
	l.relay(tcpConn, backendConn)
}

// handleUDPLoop UDP 数据包处理循环
func (l *TProxyListener) handleUDPLoop() {
	defer l.connWg.Done()

	buf := pool.GetLarge()
	defer pool.Put(buf)

	for {
		// 使用 recvmsg 获取原始目标地址
		n, srcAddr, dstAddr, err := recvmsgUDPWithDst(l.udpConn, buf)
		if err != nil {
			select {
			case <-l.shutdownCh:
				return
			default:
			}
			log.Debug().Err(err).Msg("tproxy udp recvmsg 失败")
			continue
		}

		if !l.allowLan && !isLocalAddr(srcAddr.Addr()) {
			continue
		}

		// 异步处理 UDP 数据包
		data := make([]byte, n)
		copy(data, buf[:n])

		l.connWg.Add(1)
		go l.handleUDPPacket(data, srcAddr, dstAddr)
	}
}

// handleUDPPacket 处理单个 UDP 数据包
func (l *TProxyListener) handleUDPPacket(data []byte, srcAddr, dstAddr netip.AddrPort) {
	defer l.connWg.Done()

	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeTProxy,
		SrcIP:   srcAddr.Addr(),
		SrcPort: srcAddr.Port(),
		DstIP:   dstAddr.Addr(),
		DstPort: dstAddr.Port(),
		Host:    dstAddr.Addr().String(),
		NetWork: "udp",
		InName:  l.addr,
	}

	adapt := l.selectAdapter(metadata)
	if adapt == nil {
		return
	}

	if !adapt.SupportUDP() {
		log.Debug().Str("adapter", adapt.Name()).Msg("tproxy 适配器不支持 UDP")
		return
	}

	// 建立 UDP 连接
	packetConn, err := adapt.DialUDPContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("tproxy udp dial 失败")
		return
	}
	defer packetConn.Close()

	// 转发数据包到目标
	dstUDPAddr := &net.UDPAddr{
		IP:   dstAddr.Addr().AsSlice(),
		Port: int(dstAddr.Port()),
	}
	_, err = packetConn.WriteTo(data, dstUDPAddr)
	if err != nil {
		log.Error().Err(err).Msg("tproxy udp write 失败")
		return
	}

	// 等待响应并转发回客户端
	respBuf := pool.GetLarge()
	defer pool.Put(respBuf)

	packetConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	n, _, err := packetConn.ReadFrom(respBuf)
	if err != nil {
		return
	}

	// 将响应发回客户端（使用原始源地址作为目标）
	srcUDPAddr := &net.UDPAddr{
		IP:   srcAddr.Addr().AsSlice(),
		Port: int(srcAddr.Port()),
	}

	// 使用 IP_TRANSPARENT 发送回客户端
	sendUDPTransparent(l.addr, respBuf[:n], srcUDPAddr)
}

// selectAdapter 选择适配器
func (l *TProxyListener) selectAdapter(metadata *adapter.Metadata) adapter.Adapter {
	if l.ruleEngine != nil {
		if adaptName := l.ruleEngine.Match(metadata); adaptName != "" {
			if adapt := l.adapterMgr.Get(adaptName); adapt != nil {
				return adapt
			}
		}
	}

	if l.groupMgr != nil {
		if g := l.groupMgr.Get("proxy"); g != nil {
			return g.Select(metadata)
		}
	}

	return l.adapterMgr.Get("DIRECT")
}

// relay 双向转发
func (l *TProxyListener) relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		ioCopyBuffer(left, right, buf)
		left.Close()
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		ioCopyBuffer(right, left, buf)
		right.Close()
	}()

	wg.Wait()
}

// isLocal 检查是否为本地地址
func (l *TProxyListener) isLocal(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	return isLocalAddr(netip.MustParseAddr(host))
}

// isLocalAddr 检查 IP 是否为本地地址
func isLocalAddr(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsUnspecified()
}

// ioCopyBuffer 使用指定缓冲区进行 io.Copy
func ioCopyBuffer(dst net.Conn, src net.Conn, buf []byte) {
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			_, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}

// --- 底层 TProxy socket 操作 ---

// tproxyListenTCP 创建 TCP TProxy 监听
func tproxyListenTCP(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, IP_TRANSPARENT, 1)
			})
			return err
		},
	}

	return lc.Listen(nil, "tcp", addr)
}

// tproxyListenUDP 创建 UDP TProxy 监听
func tproxyListenUDP(addr string) (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, IP_TRANSPARENT, 1)
				if err == nil {
					err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, IP_RECVORIGDSTADDR, 1)
				}
			})
			return err
		},
	}

	conn, err := lc.ListenPacket(nil, "udp", udpAddr.String())
	if err != nil {
		return nil, err
	}

	return conn.(*net.UDPConn), nil
}

// recvmsgUDPWithDst 使用 recvmsg 接收 UDP 数据包并获取原始目标地址
func recvmsgUDPWithDst(conn *net.UDPConn, buf []byte) (int, netip.AddrPort, netip.AddrPort, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, netip.AddrPort{}, netip.AddrPort{}, err
	}

	// 控制消息缓冲区（足够容纳 IPv6 的原始目标地址）
	oob := make([]byte, 256)

	var n int
	var srcAddr *net.UDPAddr
	var oobN int
	var recvFlags int

	err = raw.Control(func(fd uintptr) {
		// 构造 msghdr
		var msg syscall.Msghdr
		var iov syscall.Iovec

		iov.Base = &buf[0]
		iov.SetLen(len(buf))

		srcSA4 := &syscall.SockaddrInet4{}
		msg.Name = (*byte)(unsafe.Pointer(srcSA4))
		msg.Namelen = syscall.SizeofSockaddrInet4
		msg.Iov = &iov
		msg.Iovlen = 1
		msg.Control = &oob[0]
		msg.SetControllen(len(oob))

		r0, _, errno := syscall.Syscall6(
			syscall.SYS_RECVMSG,
			fd,
			uintptr(unsafe.Pointer(&msg)),
			0,
			0, 0, 0,
		)

		if errno != 0 {
			return
		}

		n = int(r0)
		oobN = int(msg.Controllen)
		recvFlags = int(msg.Flags)

		// 解析源地址
		port := int(srcSA4.Port)
		ip := net.IPv4(srcSA4.Addr[0], srcSA4.Addr[1], srcSA4.Addr[2], srcSA4.Addr[3])
		srcAddr = &net.UDPAddr{IP: ip, Port: port}
	})

	if err != nil {
		return 0, netip.AddrPort{}, netip.AddrPort{}, err
	}

	_ = recvFlags

	// 从控制消息中解析原始目标地址
	dstAddr, err := parseOrigDstFromCmsg(oob[:oobN])
	if err != nil {
		// 如果无法解析，使用本地地址作为后备
		dstAddr = netip.AddrPort{}
	}

	var src netip.AddrPort
	if srcAddr != nil {
		ip, _ := netip.AddrFromSlice(srcAddr.IP.To4())
		src = netip.AddrPortFrom(ip, uint16(srcAddr.Port))
	}

	return n, src, dstAddr, nil
}

// parseOrigDstFromCmsg 从控制消息中解析原始目标地址
func parseOrigDstFromCmsg(oob []byte) (netip.AddrPort, error) {
	msgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return netip.AddrPort{}, err
	}

	for _, msg := range msgs {
		if msg.Header.Level == syscall.IPPROTO_IP && msg.Header.Type == IP_RECVORIGDSTADDR {
			// IPv4: sockaddr_in
			if len(msg.Data) >= 8 {
				port := uint16(msg.Data[2])<<8 | uint16(msg.Data[3])
				ip, ok := netip.AddrFromSlice(msg.Data[4:8])
				if ok {
					return netip.AddrPortFrom(ip, port), nil
				}
			}
		}
		if msg.Header.Level == syscall.IPPROTO_IPV6 && msg.Header.Type == IP_RECVORIGDSTADDR {
			// IPv6: sockaddr_in6
			if len(msg.Data) >= 24 {
				port := uint16(msg.Data[2])<<8 | uint16(msg.Data[3])
				ip, ok := netip.AddrFromSlice(msg.Data[8:24])
				if ok {
					return netip.AddrPortFrom(ip, port), nil
				}
			}
		}
	}

	return netip.AddrPort{}, fmt.Errorf("未找到原始目标地址控制消息")
}

// getOriginalDst 获取 TCP 连接的原始目标地址
func getOriginalDst(conn *net.TCPConn) (netip.AddrPort, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return netip.AddrPort{}, err
	}

	var result netip.AddrPort

	err = raw.Control(func(fd uintptr) {
		// 尝试 IPv4
		addr4, err := getsockoptOrigDst(fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST)
		if err == nil {
			result = addr4
			return
		}

		// 尝试 IPv6
		addr6, err := getsockoptOrigDst(fd, syscall.IPPROTO_IPV6, SO_ORIGINAL_DST)
		if err == nil {
			result = addr6
		}
	})

	return result, err
}

// getsockoptOrigDst 通过 getsockopt 获取原始目标
func getsockoptOrigDst(fd uintptr, level, opt int) (netip.AddrPort, error) {
	type sockaddrIn struct {
		Family uint16
		Port   [2]byte
		Addr   [4]byte
		Zero   [8]byte
	}

	var addr sockaddrIn
	size := uint32(unsafe.Sizeof(addr))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		fd,
		uintptr(level),
		uintptr(opt),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)

	if errno != 0 {
		return netip.AddrPort{}, errno
	}

	port := uint16(addr.Port[0])<<8 | uint16(addr.Port[1])
	ip, ok := netip.AddrFromSlice(addr.Addr[:])
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("解析 IP 失败")
	}

	return netip.AddrPortFrom(ip, port), nil
}

// sendUDPTransparent 使用 IP_TRANSPARENT 发送 UDP 数据包到指定地址
func sendUDPTransparent(listenAddr string, data []byte, dst *net.UDPAddr) error {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, IP_TRANSPARENT, 1)
			})
			return err
		},
	}

	// 绑定到本地地址
	conn, err := lc.ListenPacket(context.Background(), "udp", ":0")
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.WriteTo(data, dst)
	return err
}

// --- 保留旧版兼容接口 ---

// Listener TProxy 监听器（旧版兼容）
type Listener struct {
	addr     string
	listener net.Listener
	handler  func(conn net.Conn, metadata *adapter.Metadata)
}

// New 创建 TProxy 监听器（旧版兼容）
func New(addr string) *Listener {
	return &Listener{addr: addr}
}

// Start 启动监听（旧版兼容）
func (l *Listener) Start(handler func(conn net.Conn, metadata *adapter.Metadata)) error {
	l.handler = handler

	ln, err := tproxyListenTCP(l.addr)
	if err != nil {
		return fmt.Errorf("tproxy listen %s: %w", l.addr, err)
	}
	l.listener = ln

	go l.accept()
	return nil
}

func (l *Listener) accept() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return
		}
		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(conn net.Conn) {
	defer conn.Close()

	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeTProxy,
		SrcIP:   conn.RemoteAddr().(*net.TCPAddr).AddrPort().Addr(),
		SrcPort: uint16(conn.RemoteAddr().(*net.TCPAddr).Port),
		NetWork: "tcp",
	}

	if l.handler != nil {
		l.handler(conn, metadata)
	}
}

// Close 关闭监听（旧版兼容）
func (l *Listener) Close() error {
	if l.listener != nil {
		return l.listener.Close()
	}
	return nil
}

// ListenUDP 创建 UDP TProxy 监听（旧版兼容）
func ListenUDP(addr string) (*net.UDPConn, error) {
	return tproxyListenUDP(addr)
}
