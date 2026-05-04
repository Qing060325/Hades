// Package listener 监听器模块
package listener

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/core/tunnel"
	"github.com/Qing060325/Hades/pkg/perf/pool"
	"github.com/Qing060325/Hades/pkg/perf/zerocopy"
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
	tunnel         *tunnel.Tunnel

	listeners    map[string]Listener
	activeConns  int64 // 跨所有监听器的活跃连接数
	mu           sync.RWMutex
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

// SetTunnel 设置 Tunnel（流量调度中枢）
func (m *Manager) SetTunnel(t *tunnel.Tunnel) {
	m.tunnel = t
}

// StartMixedListener 启动混合端口监听器
func (m *Manager) StartMixedListener(ctx context.Context, addr string, allowLan bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.listeners[addr]; ok {
		return fmt.Errorf("地址 %s 已被监听", addr)
	}

	listener := NewMixedListener(addr, allowLan, m.adapterManager, m.ruleEngine, m.groupManager)
	listener.tunnel = m.tunnel
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
	listener.tunnel = m.tunnel
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
	listener.tunnel = m.tunnel
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

	// 关闭 Tunnel
	if m.tunnel != nil {
		m.tunnel.Close()
	}
}

// UpdateManagers 更新管理器引用（热重载时使用）
func (m *Manager) UpdateManagers(adapterMgr *adapter.Manager, ruleEngine *rules.Engine, groupMgr *group.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.adapterManager = adapterMgr
	m.ruleEngine = ruleEngine
	m.groupManager = groupMgr

	// 同步更新 Tunnel
	if m.tunnel != nil {
		m.tunnel.UpdateAdapterManager(adapterMgr)
		m.tunnel.UpdateRuleEngine(ruleEngine)
		m.tunnel.UpdateGroupManager(groupMgr)
	}
}

// ActiveConnections 返回所有监听器的活跃连接数
func (m *Manager) ActiveConnections() int {
	return int(m.activeConns)
}

// IncrActiveConns 增加活跃连接计数
func (m *Manager) IncrActiveConns() {
	m.mu.Lock()
	m.activeConns++
	m.mu.Unlock()
}

// DecrActiveConns 减少活跃连接计数
func (m *Manager) DecrActiveConns() {
	m.mu.Lock()
	if m.activeConns > 0 {
		m.activeConns--
	}
	m.mu.Unlock()
}

// Stats 返回所有监听器的统计信息
func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	listeners := make([]map[string]interface{}, 0, len(m.listeners))
	for addr, l := range m.listeners {
		listenerInfo := map[string]interface{}{
			"addr": addr,
		}
		if l.Addr() != nil {
			listenerInfo["local_addr"] = l.Addr().String()
		}
		listeners = append(listeners, listenerInfo)
	}

	return map[string]interface{}{
		"count":            len(m.listeners),
		"active_conns":     m.activeConns,
		"listeners":        listeners,
	}
}

// BaseListener 基础监听器
type BaseListener struct {
	addr       string
	allowLan   bool
	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager
	tunnel     *tunnel.Tunnel

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

// SelectAdapter 选择适配器（回退路径，无 Tunnel 时使用）
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

// DispatchTCP 通过 Tunnel 调度 TCP 连接
// 如果 Tunnel 存在，走统一调度；否则回退到 SelectAdapter
func (l *BaseListener) DispatchTCP(conn net.Conn, metadata *adapter.Metadata) {
	if l.tunnel != nil {
		l.tunnel.HandleTCP(conn, metadata)
		return
	}

	// 回退：无 Tunnel 时的直接处理（兼容模式）
	l.selectAndRelay(conn, metadata)
}

// selectAndRelay 无 Tunnel 时的回退路径
func (l *BaseListener) selectAndRelay(conn net.Conn, metadata *adapter.Metadata) {
	adapt := l.SelectAdapter(metadata)
	if adapt == nil {
		conn.Close()
		return
	}

	backendConn, err := adapt.DialContext(context.Background(), metadata)
	if err != nil {
		log.Error().Err(err).Str("adapter", adapt.Name()).Msg("建立后端连接失败")
		conn.Close()
		return
	}
	defer backendConn.Close()
	defer conn.Close()

	// 双向转发
	l.relay(conn, backendConn)
}

// relay 双向流量转发
func (l *BaseListener) relay(left, right net.Conn) {
	// 尝试 TCP 零拷贝
	if lc, lok := left.(*net.TCPConn); lok {
		if rc, rok := right.(*net.TCPConn); rok {
			zerocopy.TCPRelay(lc, rc)
			return
		}
	}

	// 回退到缓冲拷贝
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
	}()

	wg.Wait()
}
