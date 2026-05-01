// Package tproxy Linux TProxy 透明代理入站监听器
package tproxy

import (
	"fmt"
	"net"
	"syscall"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// Listener TProxy 监听器
type Listener struct {
	addr     string
	listener net.Listener
	handler  func(conn net.Conn, metadata *adapter.Metadata)
}

// New 创建 TProxy 监听器
func New(addr string) *Listener {
	return &Listener{addr: addr}
}

// Start 启动监听
func (l *Listener) Start(handler func(conn net.Conn, metadata *adapter.Metadata)) error {
	l.handler = handler

	// 创建 TProxy socket
	ln, err := tproxyListen("tcp", l.addr)
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

// Close 关闭监听
func (l *Listener) Close() error {
	if l.listener != nil {
		return l.listener.Close()
	}
	return nil
}

// tproxyListen 创建 TProxy 监听
func tproxyListen(network, addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				// IP_TRANSPARENT = 19
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, 19, 1)
			})
			return err
		},
	}

	return lc.Listen(nil, network, addr)
}

// ListenUDP 创建 UDP TProxy 监听
func ListenUDP(addr string) (*net.UDPConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				// IP_TRANSPARENT
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, 19, 1)
				if err == nil {
					// IP_RECVORIGDSTADDR = 29
					err = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, 29, 1)
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
