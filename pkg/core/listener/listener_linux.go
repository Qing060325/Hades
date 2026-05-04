//go:build linux

// Package listener Linux 平台特有的监听器实现
package listener

import (
	"context"

	"github.com/Qing060325/Hades/pkg/core/listener/redir"
	"github.com/Qing060325/Hades/pkg/core/listener/tproxy"
	"github.com/rs/zerolog/log"
)

// StartRedirListener 启动 iptables REDIRECT 透明代理监听器
func (m *Manager) StartRedirListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return nil // 地址已被监听
	}

	l := redir.NewRedirListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	m.listeners[addr] = l

	go func() {
		if err := l.Listen(ctx); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("Redir 监听器异常")
		}
	}()

	return nil
}

// StartTProxyListener 启动 TProxy 透明代理监听器
func (m *Manager) StartTProxyListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return nil // 地址已被监听
	}

	l := tproxy.NewTProxyListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	m.listeners[addr] = l

	go func() {
		if err := l.Listen(ctx); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("TProxy 监听器异常")
		}
	}()

	return nil
}
