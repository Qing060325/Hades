// Package listener 监听器模块
package listener

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/rs/zerolog/log"
)

// Listener 监听器接口
type Listener interface {
	Listen(ctx context.Context) error
	Close() error
	Addr() net.Addr
}

// Manager 监听器管理器
type Manager struct {
	adapterManager *adapter.Manager
	ruleEngine     *rules.Engine
	groupManager   *group.Manager

	listeners map[string]Listener
	mu        sync.RWMutex
}

// NewManager 创建监听器管理器
func NewManager(
	adapterManager *adapter.Manager,
	ruleEngine *rules.Engine,
	groupManager *group.Manager,
) *Manager {
	return &Manager{
		adapterManager: adapterManager,
		ruleEngine:     ruleEngine,
		groupManager:   groupManager,
		listeners:      make(map[string]Listener),
	}
}

// StartMixedListener 启动混合端口监听器
func (m *Manager) StartMixedListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return fmt.Errorf("地址 %s 已被监听", addr)
	}

	listener := NewMixedListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	m.listeners[addr] = listener

	go func() {
		if err := listener.Listen(ctx); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("混合监听器异常")
		}
	}()

	return nil
}

// StartHTTPListener 启动 HTTP 监听器
func (m *Manager) StartHTTPListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return fmt.Errorf("地址 %s 已被监听", addr)
	}

	listener := NewHTTPListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	m.listeners[addr] = listener

	go func() {
		if err := listener.Listen(ctx); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("HTTP 监听器异常")
		}
	}()

	return nil
}

// StartSOCKSListener 启动 SOCKS 监听器
func (m *Manager) StartSOCKSListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return fmt.Errorf("地址 %s 已被监听", addr)
	}

	listener := NewSOCKSListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	m.listeners[addr] = listener

	go func() {
		if err := listener.Listen(ctx); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("SOCKS 监听器异常")
		}
	}()

	return nil
}

// Close 关闭所有监听器
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for addr, listener := range m.listeners {
		if err := listener.Close(); err != nil {
			log.Error().Err(err).Str("addr", addr).Msg("关闭监听器失败")
		}
	}

	m.listeners = make(map[string]Listener)
}

// UpdateManagers 更新管理器引用（热重载时使用）
func (m *Manager) UpdateManagers(adapterMgr *adapter.Manager, ruleEngine *rules.Engine, groupMgr *group.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.adapterManager = adapterMgr
	m.ruleEngine = ruleEngine
	m.groupManager = groupMgr
}

// BaseListener 基础监听器
type BaseListener struct {
	addr       string
	allowLan   bool
	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager

	listener net.Listener
	closed   bool
	mu       sync.Mutex

	// 优雅关闭支持
	connWg        sync.WaitGroup // 跟踪活跃连接
	shutdownCh    chan struct{}   // 关闭信号
	shutdownOnce  sync.Once
	shutdownTimeout time.Duration // 关闭超时
}

// Addr 返回监听地址
func (l *BaseListener) Addr() net.Addr {
	if l.listener != nil {
		return l.listener.Addr()
	}
	return nil
}

// Close 关闭监听器
func (l *BaseListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true

	// 发送关闭信号
	l.shutdownOnce.Do(func() {
		close(l.shutdownCh)
	})

	// 关闭 listener 停止接受新连接
	if l.listener != nil {
		l.listener.Close()
	}

	// 等待活跃连接完成
	timeout := l.shutdownTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	done := make(chan struct{})
	go func() {
		l.connWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Debug().Msg("所有活跃连接已关闭")
	case <-time.After(timeout):
		log.Warn().Dur("timeout", timeout).Msg("等待活跃连接超时，强制关闭")
	}

	return nil
}

// ConnStart 标记一个连接开始处理
func (l *BaseListener) ConnStart() {
	l.connWg.Add(1)
}

// ConnDone 标记一个连接处理完成
func (l *BaseListener) ConnDone() {
	l.connWg.Done()
}

// IsClosed 检查监听器是否已关闭
func (l *BaseListener) IsClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

// Done 返回关闭信号 channel
func (l *BaseListener) Done() <-chan struct{} {
	return l.shutdownCh
}

// SelectAdapter 选择适配器
func (l *BaseListener) SelectAdapter(metadata *adapter.Metadata) adapter.Adapter {
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
