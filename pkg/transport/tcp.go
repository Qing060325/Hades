// Package transport TCP/UDP 传输实现
package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hades/hades/pkg/perf/pool"
	"github.com/hades/hades/pkg/perf/zerocopy"
)

// TCPConn TCP 连接封装
type TCPConn struct {
	net.Conn
}

// NewTCPConn 创建 TCP 连接封装
func NewTCPConn(conn net.Conn) *TCPConn {
	return &TCPConn{Conn: conn}
}

// RelayTCP TCP 双向转发
func RelayTCP(left, right net.Conn) error {
	var wg sync.WaitGroup
	wg.Add(2)

	var copyErr error

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		_, copyErr = io.CopyBuffer(left, right, buf)
		left.Close()
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		_, copyErr = io.CopyBuffer(right, left, buf)
		right.Close()
	}()

	wg.Wait()
	return copyErr
}

// RelayTCPZeroCopy TCP 零拷贝双向转发 (Linux)
func RelayTCPZeroCopy(left, right net.Conn) error {
	leftFile, leftOk := left.(*net.TCPConn)
	rightFile, rightOk := right.(*net.TCPConn)

	if leftOk && rightOk {
		return relayTCPZeroCopy(leftFile, rightFile)
	}

	return RelayTCP(left, right)
}

func relayTCPZeroCopy(left, right *net.TCPConn) error {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		zerocopy.Copy(right, left)
		right.CloseWrite()
	}()

	go func() {
		defer wg.Done()
		zerocopy.Copy(left, right)
		left.CloseWrite()
	}()

	wg.Wait()
	return nil
}

// UDPConn UDP 连接封装
type UDPConn struct {
	conn net.PacketConn
}

// NewUDPConn 创建 UDP 连接封装
func NewUDPConn(conn net.PacketConn) *UDPConn {
	return &UDPConn{conn: conn}
}

// ReadFrom 读取
func (c *UDPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	return c.conn.ReadFrom(b)
}

// WriteTo 写入
func (c *UDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.conn.WriteTo(b, addr)
}

// Close 关闭
func (c *UDPConn) Close() error {
	return c.conn.Close()
}

// UDPAssociate UDP 关联
type UDPAssociate struct {
	conn     net.Conn
	remote   net.PacketConn
	localIP  net.IP
	localPort int
}

// NewUDPAssociate 创建 UDP 关联
func NewUDPAssociate(conn net.Conn) (*UDPAssociate, error) {
	// 创建本地 UDP socket
	udpConn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}

	localAddr := udpConn.LocalAddr().(*net.UDPAddr)

	return &UDPAssociate{
		conn:      conn,
		remote:    udpConn,
		localIP:   localAddr.IP,
		localPort: localAddr.Port,
	}, nil
}

// LocalAddr 返回本地地址
func (u *UDPAssociate) LocalAddr() *net.UDPAddr {
	return &net.UDPAddr{
		IP:   u.localIP,
		Port: u.localPort,
	}
}

// Close 关闭
func (u *UDPAssociate) Close() error {
	if u.remote != nil {
		u.remote.Close()
	}
	return nil
}

// DialContextWithTimeout 带超时的 TCP 拨号
func DialContextWithTimeout(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	return dialer.DialContext(ctx, network, address)
}

// ResolveAddress 解析地址
func ResolveAddress(ctx context.Context, host string, port int) (net.Addr, error) {
	// 简化实现，使用系统 DNS
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("无法解析地址: %s", host)
	}

	ip := ips[0]
	return &net.TCPAddr{
		IP:   ip,
		Port: port,
	}, nil
}
