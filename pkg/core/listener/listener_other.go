//go:build !linux

// Package listener 非 Linux 平台的监听器 stub 实现
package listener

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

// StartRedirListener 非 Linux 平台不支持 iptables REDIRECT
func (m *Manager) StartRedirListener(ctx context.Context, addr string, allowLan bool) error {
	log.Warn().Str("addr", addr).Msg("Redir 透明代理仅支持 Linux 平台")
	return fmt.Errorf("redir 透明代理仅支持 Linux 平台")
}

// StartTProxyListener 非 Linux 平台不支持 TProxy
func (m *Manager) StartTProxyListener(ctx context.Context, addr string, allowLan bool) error {
	log.Warn().Str("addr", addr).Msg("TProxy 透明代理仅支持 Linux 平台")
	return fmt.Errorf("tproxy 透明代理仅支持 Linux 平台")
}
