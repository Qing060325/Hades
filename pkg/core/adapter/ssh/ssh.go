// Package ssh SSH 协议适配器实现
// 通过 SSH 隧道进行代理转发，使用 direct-tcpip 通道建立 TCP 连接
package ssh

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"golang.org/x/crypto/ssh"
)

// Adapter SSH 适配器
type Adapter struct {
	name       string
	server     string
	port       int
	username   string
	password   string // 密码认证
	privateKey string // 私钥认证 (PEM 格式)
	udp        bool
	dialer     *net.Dialer

	// SSH 客户端连接管理
	mu     sync.Mutex
	client *ssh.Client
}

// Option 配置选项
type Option func(*Adapter)

// WithPassword 设置密码认证
func WithPassword(password string) Option {
	return func(a *Adapter) {
		a.password = password
	}
}

// WithPrivateKey 设置私钥认证
func WithPrivateKey(key string) Option {
	return func(a *Adapter) {
		a.privateKey = key
	}
}

// NewAdapter 创建 SSH 适配器
func NewAdapter(name, server string, port int, username string, opts ...Option) (*Adapter, error) {
	if username == "" {
		return nil, fmt.Errorf("ssh: 用户名不能为空")
	}

	a := &Adapter{
		name:     name,
		server:   server,
		port:     port,
		username: username,
		udp:      false, // SSH 不支持原生 UDP 转发
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	// 必须至少有一种认证方式
	if a.password == "" && a.privateKey == "" {
		return nil, fmt.Errorf("ssh: 必须配置密码或私钥认证")
	}

	return a, nil
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeSSH }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 通过 SSH 隧道建立 TCP 连接
// 使用 SSH 的 direct-tcpip 通道进行端口转发
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	target := metadata.DestinationAddress()

	log.Debug().
		Str("server", a.Addr()).
		Str("target", target).
		Msg("[SSH] 通过隧道建立连接")

	// 获取或建立 SSH 客户端连接
	client, err := a.getSSHClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("ssh: 获取 SSH 客户端失败: %w", err)
	}

	// 使用 direct-tcpip 通道建立 TCP 转发
	conn, err := client.Dial("tcp", target)
	if err != nil {
		// 连接失败时尝试重置客户端
		a.resetClient()
		return nil, fmt.Errorf("ssh: direct-tcpip 转发失败 [%s]: %w", target, err)
	}

	log.Debug().
		Str("target", target).
		Msg("[SSH] 隧道连接建立成功")

	return conn, nil
}

// DialUDPContext 建立 UDP 连接
// SSH 协议不支持原生 UDP 转发，需要通过 TCP 封装实现
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("ssh: 不支持 UDP 转发")
}

// URLTest 健康检查
func (a *Adapter) URLTest(ctx context.Context, testURL string) (time.Duration, error) {
	start := time.Now()
	conn, err := a.DialContext(ctx, &adapter.Metadata{
		Host:    "www.gstatic.com",
		DstPort: 80,
	})
	if err != nil {
		return 0, err
	}
	conn.Close()
	return time.Since(start), nil
}

// getSSHClient 获取或建立 SSH 客户端连接
func (a *Adapter) getSSHClient(ctx context.Context) (*ssh.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 检查现有连接是否可用
	if a.client != nil {
		// 发送 keepalive 检测连接状态
		_, _, err := a.client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			return a.client, nil
		}
		// 连接已断开，关闭并重建
		log.Warn().Err(err).Msg("[SSH] 连接已断开，正在重建")
		a.client.Close()
		a.client = nil
	}

	// 建立新的 SSH 连接
	client, err := a.dialSSH(ctx)
	if err != nil {
		return nil, err
	}

	a.client = client
	return client, nil
}

// resetClient 重置 SSH 客户端连接
func (a *Adapter) resetClient() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
}

// dialSSH 建立 SSH 连接
func (a *Adapter) dialSSH(ctx context.Context) (*ssh.Client, error) {
	// 构建认证方法
	authMethods, err := a.buildAuthMethods()
	if err != nil {
		return nil, err
	}

	// SSH 客户端配置
	config := &ssh.ClientConfig{
		User:            a.username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: 支持已知主机密钥验证
		Timeout:         15 * time.Second,
	}

	addr := a.Addr()
	log.Debug().
		Str("addr", addr).
		Str("user", a.username).
		Msg("[SSH] 建立 SSH 连接")

	// 建立 TCP 连接
	tcpConn, err := a.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP 连接失败: %w", err)
	}

	// SSH 握手
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, config)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("SSH 握手失败: %w", err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	log.Info().
		Str("addr", addr).
		Str("user", a.username).
		Msg("[SSH] SSH 连接建立成功")

	return client, nil
}

// buildAuthMethods 构建认证方法列表
func (a *Adapter) buildAuthMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 密码认证
	if a.password != "" {
		methods = append(methods, ssh.Password(a.password))
	}

	// 私钥认证
	if a.privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(a.privateKey))
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("无可用的认证方法")
	}

	return methods, nil
}
