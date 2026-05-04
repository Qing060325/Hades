//go:build linux

// Package redir Linux iptables REDIRECT 透明代理入站监听器
package redir

import (
	"context"
	"fmt"
	"io"
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

// SO_ORIGINAL_DST 获取 iptables REDIRECT 前的原始目标地址
const SO_ORIGINAL_DST = 80

// RedirListener iptables REDIRECT 透明代理监听器
type RedirListener struct {
	addr       string
	allowLan   bool
	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager
	listener   net.Listener
	closed     bool
	mu         sync.Mutex
	connWg     sync.WaitGroup
	shutdownCh chan struct{}
}

// NewRedirListener 创建 Redir 透明代理监听器
func NewRedirListener(
	addr string,
	allowLan bool,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) *RedirListener {
	return &RedirListener{
		addr:       addr,
		allowLan:   allowLan,
		adapterMgr: adapterMgr,
		ruleEngine: ruleEngine,
		groupMgr:   groupMgr,
		shutdownCh: make(chan struct{}),
	}
}

// Listen 开始监听并处理连接
func (l *RedirListener) Listen(ctx context.Context) error {
	var err error

	listenAddr := l.addr
	if listenAddr == "" {
		listenAddr = "0.0.0.0:0"
	}

	l.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("redir listen %s: %w", listenAddr, err)
	}

	log.Info().Str("addr", l.listener.Addr().String()).Msg("Redir 透明代理监听器已启动")

	for {
		conn, err := l.listener.Accept()
		if err != nil {
			select {
			case <-l.shutdownCh:
				return nil
			default:
			}
			log.Error().Err(err).Msg("redir accept 失败")
			continue
		}

		// 检查局域网访问
		if !l.allowLan && !l.isLocal(conn.RemoteAddr()) {
			conn.Close()
			continue
		}

		l.connWg.Add(1)
		go l.handleConn(conn)
	}
}

// Close 关闭监听器
func (l *RedirListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true
	close(l.shutdownCh)

	if l.listener != nil {
		l.listener.Close()
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
		log.Warn().Msg("redir 等待活跃连接超时")
	}
	return nil
}

// Addr 返回监听地址
func (l *RedirListener) Addr() net.Addr {
	if l.listener != nil {
		return l.listener.Addr()
	}
	return nil
}

// handleConn 处理单个 redir 连接
func (l *RedirListener) handleConn(conn net.Conn) {
	defer l.connWg.Done()
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}

	// 获取 iptables REDIRECT 前的原始目标地址
	origDst, err := getOriginalDst(tcpConn)
	if err != nil {
		log.Debug().Err(err).Msg("redir 获取原始目标失败")
		return
	}

	// 构建元数据
	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeRedir,
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
		log.Debug().Str("dst", origDst.String()).Msg("redir 无可用适配器")
		return
	}

	log.Debug().
		Str("src", remoteAddr.String()).
		Str("dst", origDst.String()).
		Str("adapter", adapt.Name()).
		Msg("redir 连接")

	// 连接到原始目标（通过代理）
	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("redir dial 失败")
		return
	}
	defer backendConn.Close()

	// 双向转发
	l.relay(tcpConn, backendConn)
}

// selectAdapter 选择适配器
func (l *RedirListener) selectAdapter(metadata *adapter.Metadata) adapter.Adapter {
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
func (l *RedirListener) relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
		left.Close()
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
		right.Close()
	}()

	wg.Wait()
}

// isLocal 检查是否为本地地址
func (l *RedirListener) isLocal(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// getOriginalDst 获取 iptables REDIRECT 的原始目标地址
// 使用 SO_ORIGINAL_DST (80) getsockopt 获取被 REDIRECT 前的真实目标
func getOriginalDst(conn *net.TCPConn) (netip.AddrPort, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("syscall conn: %w", err)
	}

	var result netip.AddrPort
	var ctrlErr error

	err = raw.Control(func(fd uintptr) {
		// 尝试 IPv4 SO_ORIGINAL_DST
		addr4, err := getsockoptOrigDest(fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST)
		if err == nil {
			result = addr4
			return
		}

		// 尝试 IPv6 IP6T_SO_ORIGINAL_DST (80, same value on IPv6 level)
		addr6, err := getsockoptOrigDest(fd, syscall.IPPROTO_IPV6, SO_ORIGINAL_DST)
		if err == nil {
			result = addr6
			return
		}

		ctrlErr = fmt.Errorf("无法获取原始目标: IPv4=%v, IPv6=%v", err, ctrlErr)
	})

	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("control: %w", err)
	}
	if ctrlErr != nil {
		return netip.AddrPort{}, ctrlErr
	}

	return result, nil
}

// getsockoptOrigDest 通过 getsockopt 获取原始目标地址
func getsockoptOrigDest(fd uintptr, level, opt int) (netip.AddrPort, error) {
	// sockaddr_in 结构 (16 bytes)
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

// getOriginalDst 公开版本，供外部使用
// 获取 iptables REDIRECT 前的原始目标地址
func GetOriginalDst(conn *net.TCPConn) (netip.AddrPort, error) {
	return getOriginalDst(conn)
}

// --- 以下保留旧版兼容接口 ---

// Listener Redir 监听器（旧版兼容）
type Listener struct {
	addr     string
	listener net.Listener
	handler  func(conn net.Conn, metadata *adapter.Metadata)
}

// New 创建 Redir 监听器（旧版兼容）
func New(addr string) *Listener {
	return &Listener{addr: addr}
}

// Start 启动监听（旧版兼容）
func (l *Listener) Start(handler func(conn net.Conn, metadata *adapter.Metadata)) error {
	l.handler = handler

	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("redir listen %s: %w", l.addr, err)
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
		go l.handleConnOld(conn)
	}
}

func (l *Listener) handleConnOld(conn net.Conn) {
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}

	origDst, err := getOriginalDst(tcpConn)
	if err != nil {
		return
	}

	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeRedir,
		SrcIP:   remoteAddr.AddrPort().Addr(),
		SrcPort: uint16(remoteAddr.Port),
		DstIP:   origDst.Addr(),
		DstPort: origDst.Port(),
		Host:    origDst.Addr().String(),
		NetWork: "tcp",
	}

	if l.handler != nil {
		l.handler(conn, metadata)
	}
}

// getOriginalDest 获取 iptables REDIRECT 的原始目标地址（旧版兼容）
func getOriginalDest(conn net.Conn) (*adapter.Metadata, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("not a TCP connection")
	}

	origDst, err := getOriginalDst(tcpConn)
	if err != nil {
		return nil, err
	}

	remoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	return &adapter.Metadata{
		Type:    adapter.MetadataTypeRedir,
		SrcIP:   remoteAddr.AddrPort().Addr(),
		DstIP:   origDst.Addr(),
		SrcPort: uint16(remoteAddr.Port),
		DstPort: origDst.Port(),
		NetWork: "tcp",
	}, nil
}

// getsockoptOrigDest 使用 getsockopt 获取原始目标（旧版兼容）
func getsockoptOrigDestLegacy(fd uintptr, opt int) (*net.TCPAddr, error) {
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
		uintptr(syscall.IPPROTO_IP),
		uintptr(opt),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)

	if errno != 0 {
		return nil, errno
	}

	port := int(addr.Port[0])<<8 | int(addr.Port[1])
	ip := net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])

	return &net.TCPAddr{IP: ip, Port: port}, nil
}


