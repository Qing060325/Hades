// Package singmux Sing-Mux 多路复用传输实现
// Sing-Mux 支持 smux/yamux/h2mux 三种多路复用协议，可配置连接数、流数和暴力模式
package singmux

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Protocol 多路复用协议类型
type Protocol string

const (
	ProtocolSMux  Protocol = "smux"  // smux 协议
	ProtocolYAMux Protocol = "yamux" // yamux 协议
	ProtocolH2Mux Protocol = "h2mux" // h2mux 协议
)

// BrutalConfig 暴力模式配置（固定带宽分配）
type BrutalConfig struct {
	Up   int `yaml:"up"`   // 上行带宽 (Mbps)
	Down int `yaml:"down"` // 下行带宽 (Mbps)
}

// Config Sing-Mux 配置
type Config struct {
	Protocol      Protocol     `yaml:"protocol"`       // 多路复用协议 (smux/yamux/h2mux)
	MaxConnections int         `yaml:"max-connections"` // 最大连接数
	MinStreams     int         `yaml:"min-streams"`     // 每连接最小流数
	MaxStreams     int         `yaml:"max-streams"`     // 每连接最大流数
	Statistic     bool         `yaml:"statistic"`       // 启用统计
	Padding       bool         `yaml:"padding"`         // 启用填充
	Brutal        *BrutalConfig `yaml:"brutal"`         // 暴力模式配置
}

// DefaultConfig 默认 Sing-Mux 配置
var DefaultConfig = Config{
	Protocol:       ProtocolSMux,
	MaxConnections: 1,
	MinStreams:     1,
	MaxStreams:     0, // 0 表示不限制
}

// Session 多路复用会话
type Session struct {
	config    *Config
	dialer    func(ctx context.Context) (net.Conn, error) // 底层连接拨号器
	conns     []*muxConn                                   // 底层连接池
	mu        sync.Mutex
	closed    bool
	streamIdx uint32
}

// NewSession 创建多路复用会话
func NewSession(cfg *Config, dialer func(ctx context.Context) (net.Conn, error)) (*Session, error) {
	if cfg == nil {
		defaultCfg := DefaultConfig
		cfg = &defaultCfg
	}

	if dialer == nil {
		return nil, fmt.Errorf("拨号器不能为空")
	}

	// 验证协议类型
	switch cfg.Protocol {
	case ProtocolSMux, ProtocolYAMux, ProtocolH2Mux:
	default:
		return nil, fmt.Errorf("不支持的多路复用协议: %s", cfg.Protocol)
	}

	s := &Session{
		config: cfg,
		dialer: dialer,
		conns:  make([]*muxConn, 0, cfg.MaxConnections),
	}

	// 预创建底层连接
	if err := s.ensureConns(context.Background()); err != nil {
		return nil, err
	}

	return s, nil
}

// ensureConns 确保连接池中有足够的底层连接
func (s *Session) ensureConns(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.conns) < s.config.MaxConnections {
		conn, err := s.dialer(ctx)
		if err != nil {
			return fmt.Errorf("建立底层连接失败: %w", err)
		}

		muxC := &muxConn{
			conn:     conn,
			protocol: s.config.Protocol,
		}
		s.conns = append(s.conns, muxC)
	}

	return nil
}

// DialStream 建立新的多路复用流
func (s *Session) DialStream(ctx context.Context) (net.Conn, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("会话已关闭")
	}

	// 选择一个底层连接（简单轮询）
	if len(s.conns) == 0 {
		s.mu.Unlock()
		if err := s.ensureConns(ctx); err != nil {
			return nil, err
		}
		s.mu.Lock()
	}

	idx := s.streamIdx % uint32(len(s.conns))
	s.streamIdx++
	muxC := s.conns[idx]
	s.mu.Unlock()

	// 根据协议类型创建流
	return s.openStream(muxC)
}

// openStream 在底层连接上打开新流
func (s *Session) openStream(muxC *muxConn) (net.Conn, error) {
	switch s.config.Protocol {
	case ProtocolSMux:
		return s.openSMuxStream(muxC)
	case ProtocolYAMux:
		return s.openYAMuxStream(muxC)
	case ProtocolH2Mux:
		return s.openH2MuxStream(muxC)
	default:
		return nil, fmt.Errorf("不支持的协议: %s", s.config.Protocol)
	}
}

// openSMuxStream 打开 smux 流
func (s *Session) openSMuxStream(muxC *muxConn) (net.Conn, error) {
	// TODO: 实现 smux 流打开
	// 使用 smux 库在底层连接上创建新流
	return &streamConn{
		conn:    muxC.conn,
		session: s,
	}, nil
}

// openYAMuxStream 打开 yamux 流
func (s *Session) openYAMuxStream(muxC *muxConn) (net.Conn, error) {
	// TODO: 实现 yamux 流打开
	return &streamConn{
		conn:    muxC.conn,
		session: s,
	}, nil
}

// openH2MuxStream 打开 h2mux 流
func (s *Session) openH2MuxStream(muxC *muxConn) (net.Conn, error) {
	// TODO: 实现 h2mux 流打开
	return &streamConn{
		conn:    muxC.conn,
		session: s,
	}, nil
}

// Close 关闭会话及所有底层连接
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	var firstErr error
	for _, c := range s.conns {
		if err := c.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.conns = nil

	return firstErr
}

// Stats 返回会话统计信息
func (s *Session) Stats() SessionStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return SessionStats{
		Protocol:      s.config.Protocol,
		Connections:   len(s.conns),
		MaxConnections: s.config.MaxConnections,
		Padding:       s.config.Padding,
		BrutalEnabled: s.config.Brutal != nil,
	}
}

// SessionStats 会话统计
type SessionStats struct {
	Protocol       Protocol
	Connections    int
	MaxConnections int
	Padding        bool
	BrutalEnabled  bool
}

// muxConn 底层多路复用连接
type muxConn struct {
	conn     net.Conn
	protocol Protocol
}

// streamConn 多路复用流连接
type streamConn struct {
	conn     net.Conn
	session  *Session
	closed   bool
	mu       sync.Mutex
}

func (s *streamConn) Read(b []byte) (int, error) {
	return s.conn.Read(b)
}

func (s *streamConn) Write(b []byte) (int, error) {
	return s.conn.Write(b)
}

func (s *streamConn) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	// 流关闭不关闭底层连接，只关闭逻辑流
	return nil
}

func (s *streamConn) LocalAddr() net.Addr                { return s.conn.LocalAddr() }
func (s *streamConn) RemoteAddr() net.Addr               { return s.conn.RemoteAddr() }
func (s *streamConn) SetDeadline(t time.Time) error      { return s.conn.SetDeadline(t) }
func (s *streamConn) SetReadDeadline(t time.Time) error   { return s.conn.SetReadDeadline(t) }
func (s *streamConn) SetWriteDeadline(t time.Time) error  { return s.conn.SetWriteDeadline(t) }

// WrapConn 将普通连接包装为支持多路复用的连接
func WrapConn(conn net.Conn, cfg *Config) (net.Conn, error) {
	if cfg == nil {
		defaultCfg := DefaultConfig
		cfg = &defaultCfg
	}

	session, err := NewSession(cfg, func(ctx context.Context) (net.Conn, error) {
		return conn, nil
	})
	if err != nil {
		return nil, err
	}

	return session.DialStream(context.Background())
}

// Relay 多路复用双向转发
func Relay(left io.ReadWriteCloser, right io.ReadWriteCloser) error {
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(left, right)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(right, left)
		errCh <- err
	}()

	err1 := <-errCh
	_ = <-errCh

	if err1 != nil {
		return err1
	}
	return nil
}
