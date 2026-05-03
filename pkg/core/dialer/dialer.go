// Package dialer 拨号器模块
package dialer

import (
	"context"
	"net"
	"syscall"
	"time"

	"github.com/Qing060325/Hades/internal/config"
)

// Manager 拨号器管理器
type Manager struct {
	cfg *config.Config

	// 默认拨号器
	defaultDialer *net.Dialer

	// MPTCP 拨号器
	mptcpDialer *net.Dialer

	// 是否启用 MPTCP
	useMPTCP bool
}

// NewManager 创建拨号器管理器
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg: cfg,
		defaultDialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// Option 拨号器配置选项
type Option func(*Manager)

// WithMPTCP 启用 MPTCP (Multipath TCP) 支持
// MPTCP 允许单一连接同时使用多条网络路径，提高带宽和可靠性
// 需要内核支持 MPTCP (Linux 5.6+)
func WithMPTCP() Option {
	return func(m *Manager) {
		m.useMPTCP = true
		m.mptcpDialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			// 启用 MPTCP - 需要 Go 1.21+ 或使用 syscall 控制
			// 通过 Control 函数设置 IPPROTO_TCP 和 MPTCP socket 选项
			Control: mptcpControl(),
		}
	}
}

// NewManagerWithOptions 创建带选项的拨号器管理器
func NewManagerWithOptions(cfg *config.Config, opts ...Option) *Manager {
	m := NewManager(cfg)
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// mptcpControl 返回 MPTCP socket 控制函数
// 通过设置 TCP 协议选项启用 MPTCP
func mptcpControl() func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		// 仅在 TCP 连接上启用 MPTCP
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil
		}

		var err error
		controlErr := c.Control(func(fd uintptr) {
			// 设置 MPTCP socket 选项
			// IPPROTO_TCP = 6, MPTCP_ENABLED = 42 (Linux 内核特定值)
			// 这里使用通用的 syscall 方式设置
			err = setMPTCP(int(fd))
		})

		if controlErr != nil {
			return controlErr
		}
		return err
	}
}

// UseMPTCP 是否启用了 MPTCP
func (m *Manager) UseMPTCP() bool {
	return m.useMPTCP
}

// DialContext 建立TCP连接
func (m *Manager) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// 如果启用了 MPTCP 且是 TCP 连接，使用 MPTCP 拨号器
	if m.useMPTCP && (network == "tcp" || network == "tcp4" || network == "tcp6") {
		return m.mptcpDialer.DialContext(ctx, network, addr)
	}
	return m.defaultDialer.DialContext(ctx, network, addr)
}

// DialUDP 建立UDP连接
func (m *Manager) DialUDP(network, addr string) (net.PacketConn, error) {
	return net.ListenPacket(network, addr)
}
