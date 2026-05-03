// Package kcp KCP 传输层实现
// KCP 是一种快速可靠的 ARQ 协议，提供比 TCP 更低延迟的可靠传输
package kcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	kcpgo "github.com/metacubex/kcp-go"
)

// Config KCP 传输配置
type Config struct {
	DataChannel string `yaml:"data-channel"` // 数据通道标识
	Seed        string `yaml:"seed"`         // 加密密钥
	MTU         int    `yaml:"mtu"`          // 最大传输单元
	SndWnd      int    `yaml:"sndwnd"`       // 发送窗口大小
	RcvWnd      int    `yaml:"rcvwnd"`       // 接收窗口大小
	NoDelay     int    `yaml:"nodelay"`      // 是否启用无延迟模式 (0/1)
	Interval    int    `yaml:"interval"`     // 内部刷新间隔 (ms)
	Resend      int    `yaml:"resend"`       // 快速重传触发次数
	NC          int    `yaml:"nc"`           // 关闭流控 (0/1)
}

// DefaultConfig 默认 KCP 配置
var DefaultConfig = Config{
	DataChannel: "default",
	MTU:        1400,
	SndWnd:     256,
	RcvWnd:     256,
	NoDelay:    1,
	Interval:   50,
	Resend:     2,
	NC:         1,
}

// Conn KCP 传输连接封装
type Conn struct {
	session *kcpgo.UDPSession
	config  *Config
	mu      sync.Mutex
	closed  bool
}

// NewConn 创建 KCP 连接
func NewConn(session *kcpgo.UDPSession, cfg *Config) *Conn {
	if cfg == nil {
		defaultCfg := DefaultConfig
		cfg = &defaultCfg
	}

	// 应用 KCP 参数
	applyConfig(session, cfg)

	return &Conn{
		session: session,
		config:  cfg,
	}
}

// Dial 建立 KCP 连接
func Dial(ctx context.Context, addr string, cfg *Config) (*Conn, error) {
	if cfg == nil {
		defaultCfg := DefaultConfig
		cfg = &defaultCfg
	}

	// 使用 seed 作为加密密钥建立 KCP 会话
	var session *kcpgo.UDPSession
	var err error

	if cfg.Seed != "" {
		// 使用加密模式
		block, blockErr := kcpgo.NewAESBlockCrypt([]byte(cfg.Seed))
		if blockErr != nil {
			return nil, fmt.Errorf("创建 KCP 加密块失败: %w", blockErr)
		}
		session, err = kcpgo.DialWithOptions(addr, block, cfg.DataShards(), cfg.ParityShards())
	} else {
		// 无加密模式
		session, err = kcpgo.DialWithOptions(addr, nil, cfg.DataShards(), cfg.ParityShards())
	}

	if err != nil {
		return nil, fmt.Errorf("KCP 拨号失败: %w", err)
	}

	// 设置超时
	if deadline, ok := ctx.Deadline(); ok {
		session.SetDeadline(deadline)
	}

	conn := NewConn(session, cfg)
	return conn, nil
}

// Listen 监听 KCP 连接
func Listen(addr string, cfg *Config) (*Listener, error) {
	if cfg == nil {
		defaultCfg := DefaultConfig
		cfg = &defaultCfg
	}

	var ln *kcpgo.Listener
	var err error

	if cfg.Seed != "" {
		block, blockErr := kcpgo.NewAESBlockCrypt([]byte(cfg.Seed))
		if blockErr != nil {
			return nil, fmt.Errorf("创建 KCP 加密块失败: %w", blockErr)
		}
		ln, err = kcpgo.ListenWithOptions(addr, block, cfg.DataShards(), cfg.ParityShards())
	} else {
		ln, err = kcpgo.ListenWithOptions(addr, nil, cfg.DataShards(), cfg.ParityShards())
	}

	if err != nil {
		return nil, fmt.Errorf("KCP 监听失败: %w", err)
	}

	return &Listener{ln: ln, config: cfg}, nil
}

// DataShards 返回数据分片数（默认 10）
func (c *Config) DataShards() int {
	return 10
}

// ParityShards 返回校验分片数（默认 3）
func (c *Config) ParityShards() int {
	return 3
}

// applyConfig 将配置应用到 KCP 会话
func applyConfig(session *kcpgo.UDPSession, cfg *Config) {
	// 设置 KCP 核心参数
	// NoDelay: 0=关闭(默认延迟), 1=开启(无延迟模式)
	// Interval: 内部刷新时钟间隔，单位 ms
	// Resend: 快速重传触发次数，越小越激进
	// NC: 0=开启流控, 1=关闭流控
	session.SetNoDelay(cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NC)

	// 设置窗口大小
	session.SetWindowSize(cfg.SndWnd, cfg.RcvWnd)

	// 设置 MTU
	if cfg.MTU > 0 {
		session.SetMtu(cfg.MTU)
	}
}

// Read 读取数据
func (c *Conn) Read(b []byte) (int, error) {
	return c.session.Read(b)
}

// Write 写入数据
func (c *Conn) Write(b []byte) (int, error) {
	return c.session.Write(b)
}

// Close 关闭连接
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.session.Close()
}

// LocalAddr 返回本地地址
func (c *Conn) LocalAddr() net.Addr {
	return c.session.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *Conn) RemoteAddr() net.Addr {
	return c.session.RemoteAddr()
}

// SetDeadline 设置读写截止时间
func (c *Conn) SetDeadline(t time.Time) error {
	return c.session.SetDeadline(t)
}

// SetReadDeadline 设置读取截止时间
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.session.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入截止时间
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.session.SetWriteDeadline(t)
}

// Listener KCP 监听器
type Listener struct {
	ln     *kcpgo.Listener
	config *Config
}

// Accept 接受连接
func (l *Listener) Accept() (net.Conn, error) {
	session, err := l.ln.AcceptKCP()
	if err != nil {
		return nil, err
	}
	return NewConn(session, l.config), nil
}

// Close 关闭监听器
func (l *Listener) Close() error {
	return l.ln.Close()
}

// Addr 返回监听地址
func (l *Listener) Addr() net.Addr {
	return l.ln.Addr()
}
