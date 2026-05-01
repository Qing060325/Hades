// Package redir Linux Redir 入站监听器
package redir

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// Listener Redir 监听器
type Listener struct {
	addr     string
	listener net.Listener
	handler  func(conn net.Conn, metadata *adapter.Metadata)
}

// New 创建 Redir 监听器
func New(addr string) *Listener {
	return &Listener{addr: addr}
}

// Start 启动监听
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
		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(conn net.Conn) {
	defer conn.Close()

	// 获取原始目标地址
	metadata, err := getOriginalDest(conn)
	if err != nil {
		return
	}

	if l.handler != nil {
		l.handler(conn, metadata)
	}
}

// Close 关闭监听
func (l *Listener) Close() error {
	if l.listener != nil {
		return l.listener.Close()
	}
	return nil
}

// getOriginalDest 获取 iptables REDIRECT 的原始目标地址
func getOriginalDest(conn net.Conn) (*adapter.Metadata, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("not a TCP connection")
	}

	raw, err := tcpConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var destAddr *net.TCPAddr
	err = raw.Control(func(fd uintptr) {
		// SO_ORIGINAL_DST = 80
		const SO_ORIGINAL_DST = 80
		addr, err := getsockoptOrigDest(fd, SO_ORIGINAL_DST)
		if err == nil {
			destAddr = addr
		}
	})

	if err != nil || destAddr == nil {
		return nil, fmt.Errorf("get original dest: %v", err)
	}

	metadata := &adapter.Metadata{
		Type:    adapter.MetadataTypeRedir,
		SrcIP:   tcpConn.RemoteAddr().(*net.TCPAddr).AddrPort().Addr(),
		DstIP:   destAddr.AddrPort().Addr(),
		SrcPort: uint16(tcpConn.RemoteAddr().(*net.TCPAddr).Port),
		DstPort: uint16(destAddr.Port),
		NetWork: "tcp",
	}

	return metadata, nil
}

// getsockoptOrigDest 使用 getsockopt 获取原始目标
func getsockoptOrigDest(fd uintptr, opt int) (*net.TCPAddr, error) {
	// sockaddr_in 结构
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
