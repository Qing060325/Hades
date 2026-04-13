// Package adapter 代理适配器 - Direct 直连适配器
package adapter

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog/log"
)

// Direct 直连适配器
type Direct struct {
	BaseAdapter
	dialer *net.Dialer
}

// NewDirect 创建直连适配器
func NewDirect() *Direct {
	return &Direct{
		BaseAdapter: BaseAdapter{
			name:       "DIRECT",
			typ:        TypeDirect,
			addr:       "",
			supportUDP: true,
		},
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// DialContext 建立TCP连接
func (d *Direct) DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	addr := metadata.DestinationAddress()
	log.Debug().Str("addr", addr).Msg("[Direct] 建立直连")

	conn, err := d.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("直连失败: %w", err)
	}
	return conn, nil
}

// DialUDPContext 建立UDP连接
func (d *Direct) DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error) {
	addr := metadata.DestinationAddress()
	log.Debug().Str("addr", addr).Msg("[Direct] 建立UDP直连")

	// 创建UDP连接
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, fmt.Errorf("创建UDP连接失败: %w", err)
	}
	return conn, nil
}

// URLTest 健康检查
func (d *Direct) URLTest(ctx context.Context, url string) (time.Duration, error) {
	start := time.Now()
	conn, err := d.dialer.DialContext(ctx, "tcp", url)
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}

// Reject 拒绝适配器
type Reject struct {
	BaseAdapter
}

// NewReject 创建拒绝适配器
func NewReject() *Reject {
	return &Reject{
		BaseAdapter: BaseAdapter{
			name:       "REJECT",
			typ:        TypeReject,
			addr:       "",
			supportUDP: true,
		},
	}
}

// DialContext 建立TCP连接 (拒绝)
func (r *Reject) DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	log.Debug().Str("addr", metadata.DestinationAddress()).Msg("[Reject] 拒绝连接")
	return nil, fmt.Errorf("连接被拒绝")
}

// DialUDPContext 建立UDP连接 (拒绝)
func (r *Reject) DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error) {
	log.Debug().Str("addr", metadata.DestinationAddress()).Msg("[Reject] 拒绝UDP连接")
	return nil, fmt.Errorf("UDP连接被拒绝")
}

// URLTest 健康检查 (拒绝)
func (r *Reject) URLTest(ctx context.Context, url string) (time.Duration, error) {
	return 0, fmt.Errorf("拒绝节点不支持健康检查")
}

// RejectDrop 直接丢弃适配器
type RejectDrop struct {
	BaseAdapter
}

// NewRejectDrop 创建丢弃适配器
func NewRejectDrop() *RejectDrop {
	return &RejectDrop{
		BaseAdapter: BaseAdapter{
			name:       "REJECT-DROP",
			typ:        TypeReject,
			addr:       "",
			supportUDP: true,
		},
	}
}

// DialContext 建立TCP连接 (静默丢弃)
func (r *RejectDrop) DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 静默丢弃，不返回错误，返回一个永远阻塞的连接
	return &dropConn{}, nil
}

// DialUDPContext 建立UDP连接 (静默丢弃)
func (r *RejectDrop) DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error) {
	return &dropPacketConn{}, nil
}

// dropConn 丢弃连接 (永不响应)
type dropConn struct {
	net.Conn
}

func (c *dropConn) Read(b []byte) (n int, err error)  { return 0, nil }
func (c *dropConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (c *dropConn) Close() error                      { return nil }

// dropPacketConn 丢弃UDP连接
type dropPacketConn struct {
	net.PacketConn
}

func (c *dropPacketConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	return 0, nil, nil
}
func (c *dropPacketConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	return len(b), nil
}
func (c *dropPacketConn) Close() error { return nil }
