// Package dialer 拨号器模块
package dialer

import (
	"context"
	"net"
	"time"

	"github.com/Qing060325/Hades/internal/config"
)

// Manager 拨号器管理器
type Manager struct {
	cfg *config.Config

	// 默认拨号器
	defaultDialer *net.Dialer
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

// DialContext 建立TCP连接
func (m *Manager) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return m.defaultDialer.DialContext(ctx, network, addr)
}

// DialUDP 建立UDP连接
func (m *Manager) DialUDP(network, addr string) (net.PacketConn, error) {
	return net.ListenPacket(network, addr)
}
