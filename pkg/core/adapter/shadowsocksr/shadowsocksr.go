// Package shadowsocksr ShadowsocksR 协议实现
// ShadowsocksR 是 Shadowsocks 的增强版本，支持协议和混淆插件
package shadowsocksr

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/transport/ssr"
)

// 支持的混淆类型
var supportedObfs = map[string]bool{
	"plain":               true,
	"http_simple":         true,
	"http_post":           true,
	"tls1.2_ticket_auth": true,
}

// 支持的协议类型
var supportedProtocol = map[string]bool{
	"origin":          true,
	"auth_aes128_md5": true,
	"auth_sha1_v4":    true,
}

// 支持的加密方式
var supportedCipher = map[string]int{
	"aes-128-ctr":      16,
	"aes-192-ctr":      24,
	"aes-256-ctr":      32,
	"aes-128-cfb":      16,
	"aes-192-cfb":      24,
	"aes-256-cfb":      32,
	"bf-cfb":           16,
	"cast5-cfb":        16,
	"des-cfb":          8,
	"rc4-md5":          16,
	"chacha20-ietf":    32,
	"xchacha20":        32,
}

// Adapter ShadowsocksR 适配器
type Adapter struct {
	name       string
	server     string
	port       int
	cipher     string
	password   string
	obfs       string
	obfsParam  string
	protocol   string
	protoParam string
	udp        bool
	dialer     *net.Dialer
}

// NewAdapter 创建 ShadowsocksR 适配器
func NewAdapter(name, server string, port int, cipher, password string, opts ...Option) (*Adapter, error) {
	// 验证加密方式
	if _, ok := supportedCipher[cipher]; !ok {
		return nil, fmt.Errorf("不支持的 SSR 加密方式: %s", cipher)
	}

	a := &Adapter{
		name:     name,
		server:   server,
		port:     port,
		cipher:   cipher,
		password: password,
		obfs:     "plain",
		protocol: "origin",
		udp:      true,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(a)
	}

	// 验证混淆类型
	if !supportedObfs[a.obfs] {
		return nil, fmt.Errorf("不支持的 SSR 混淆类型: %s", a.obfs)
	}

	// 验证协议类型
	if !supportedProtocol[a.protocol] {
		return nil, fmt.Errorf("不支持的 SSR 协议类型: %s", a.protocol)
	}

	return a, nil
}

// Option 配置选项
type Option func(*Adapter)

// WithObfs 设置混淆类型
func WithObfs(obfs string, param string) Option {
	return func(a *Adapter) {
		a.obfs = obfs
		a.obfsParam = param
	}
}

// WithProtocol 设置协议类型
func WithProtocol(proto string, param string) Option {
	return func(a *Adapter) {
		a.protocol = proto
		a.protoParam = param
	}
}

// Name 返回名称
func (a *Adapter) Name() string { return a.name }

// Type 返回类型
func (a *Adapter) Type() adapter.AdapterType { return adapter.TypeShadowsocksR }

// Addr 返回地址
func (a *Adapter) Addr() string { return fmt.Sprintf("%s:%d", a.server, a.port) }

// SupportUDP 是否支持 UDP
func (a *Adapter) SupportUDP() bool { return a.udp }

// SupportWithDialer 是否支持自定义拨号器
func (a *Adapter) SupportWithDialer() bool { return true }

// DialContext 建立 TCP 连接
func (a *Adapter) DialContext(ctx context.Context, metadata *adapter.Metadata) (net.Conn, error) {
	// 连接 SSR 服务器
	conn, err := a.dialer.DialContext(ctx, "tcp", a.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接 SSR 服务器失败: %w", err)
	}

	// 创建 SSR 连接配置
	cfg := &ssr.Config{
		Password:   a.password,
		Method:     a.cipher,
		Obfs:       a.obfs,
		ObfsParam:  a.obfsParam,
		Proto:      a.protocol,
		ProtoParam: a.protoParam,
	}

	// 包装为 SSR 连接
	ssrConn, err := ssr.NewConn(conn, cfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建 SSR 连接失败: %w", err)
	}

	// 编码并发送目标地址
	target := packAddr(metadata.Host, metadata.DstPort)
	encoded, err := ssrConn.EncodeRequest(target)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("编码 SSR 请求失败: %w", err)
	}

	if _, err := ssrConn.Write(encoded); err != nil {
		conn.Close()
		return nil, fmt.Errorf("发送 SSR 请求失败: %w", err)
	}

	return ssrConn, nil
}

// DialUDPContext 建立 UDP 连接
func (a *Adapter) DialUDPContext(ctx context.Context, metadata *adapter.Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("SSR UDP 尚未实现")
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

// packAddr 打包目标地址 (SOCKS5 格式)
func packAddr(host string, port uint16) []byte {
	buf := make([]byte, 0, 64)

	ip := net.ParseIP(host)
	if ip == nil {
		// 域名
		buf = append(buf, 0x03)
		buf = append(buf, byte(len(host)))
		buf = append(buf, []byte(host)...)
	} else if ip4 := ip.To4(); ip4 != nil {
		// IPv4
		buf = append(buf, 0x01)
		buf = append(buf, ip4...)
	} else {
		// IPv6
		buf = append(buf, 0x04)
		buf = append(buf, ip.To16()...)
	}

	// 端口 (大端)
	buf = append(buf, byte(port>>8), byte(port))

	return buf
}
