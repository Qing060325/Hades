// Package adapter 代理适配器 - HTTP 代理
package adapter

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// HTTP HTTP代理适配器
type HTTP struct {
	BaseAdapter
	server   string
	port     int
	user     string
	password string
	tls      bool
	sni      string
	dialer   *net.Dialer
}

// HTTPOption HTTP代理选项
type HTTPOption struct {
	Name     string
	Server   string
	Port     int
	User     string
	Password string
	TLS      bool
	SNI      string
}

// NewHTTP 创建HTTP代理适配器
func NewHTTP(opt *HTTPOption) *HTTP {
	return &HTTP{
		BaseAdapter: BaseAdapter{
			name:       opt.Name,
			typ:        TypeHTTP,
			addr:       fmt.Sprintf("%s:%d", opt.Server, opt.Port),
			supportUDP: false,
		},
		server:   opt.Server,
		port:     opt.Port,
		user:     opt.User,
		password: opt.Password,
		tls:      opt.TLS,
		sni:      opt.SNI,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// DialContext 建立TCP连接
func (h *HTTP) DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	log.Debug().Str("proxy", h.name).Str("target", metadata.DestinationAddress()).Msg("[HTTP] 建立连接")

	// 连接代理服务器
	var conn net.Conn
	var err error

	if h.tls {
		// TLS 连接
		conn, err = h.dialTLS(ctx)
	} else {
		conn, err = h.dialer.DialContext(ctx, "tcp", h.addr)
	}

	if err != nil {
		return nil, fmt.Errorf("连接HTTP代理失败: %w", err)
	}

	// 发送 CONNECT 请求
	if err := h.connect(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// DialUDPContext HTTP不支持UDP
func (h *HTTP) DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error) {
	return nil, fmt.Errorf("HTTP代理不支持UDP")
}

// connect 发送CONNECT请求
func (h *HTTP) connect(conn net.Conn, metadata *Metadata) error {
	target := metadata.DestinationAddress()

	// 构建CONNECT请求
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: target},
		Host:   target,
		Header: make(http.Header),
	}

	// 设置代理认证
	if h.user != "" && h.password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(h.user + ":" + h.password))
		req.Header.Set("Proxy-Authorization", "Basic "+auth)
	}

	// 发送请求
	if err := req.Write(conn); err != nil {
		return fmt.Errorf("发送CONNECT请求失败: %w", err)
	}

	// 读取响应
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("代理返回错误: %s", resp.Status)
	}

	return nil
}

// dialTLS 建立TLS连接
func (h *HTTP) dialTLS(ctx context.Context) (net.Conn, error) {
	conn, err := h.dialer.DialContext(ctx, "tcp", h.addr)
	if err != nil {
		return nil, err
	}

	// TLS握手
	tlsConfig := &tls.Config{
		ServerName: h.sni,
	}
	if h.sni == "" {
		tlsConfig.ServerName = h.server
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// URLTest 健康检查
func (h *HTTP) URLTest(ctx context.Context, testURL string) (time.Duration, error) {
	start := time.Now()

	conn, err := h.DialContext(ctx, &Metadata{
		Host:    strings.TrimPrefix(testURL, "http://"),
		DstPort: 80,
	})
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return time.Since(start), nil
}

// SOCKS5 SOCKS5代理适配器
type SOCKS5 struct {
	BaseAdapter
	server   string
	port     int
	user     string
	password string
	tls      bool
	sni      string
	udp      bool
	dialer   *net.Dialer
}

// SOCKS5Option SOCKS5代理选项
type SOCKS5Option struct {
	Name     string
	Server   string
	Port     int
	User     string
	Password string
	TLS      bool
	SNI      string
	UDP      bool
}

// NewSOCKS5 创建SOCKS5代理适配器
func NewSOCKS5(opt *SOCKS5Option) *SOCKS5 {
	return &SOCKS5{
		BaseAdapter: BaseAdapter{
			name:       opt.Name,
			typ:        TypeSOCKS5,
			addr:       fmt.Sprintf("%s:%d", opt.Server, opt.Port),
			supportUDP: opt.UDP,
		},
		server:   opt.Server,
		port:     opt.Port,
		user:     opt.User,
		password: opt.Password,
		tls:      opt.TLS,
		sni:      opt.SNI,
		udp:      opt.UDP,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// DialContext 建立TCP连接
func (s *SOCKS5) DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	log.Debug().Str("proxy", s.name).Str("target", metadata.DestinationAddress()).Msg("[SOCKS5] 建立连接")

	// 连接代理服务器
	conn, err := s.dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("连接SOCKS5代理失败: %w", err)
	}

	// SOCKS5握手
	if err := s.handshake(conn, metadata); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// DialUDPContext 建立UDP连接
func (s *SOCKS5) DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error) {
	if !s.udp {
		return nil, fmt.Errorf("SOCKS5代理不支持UDP")
	}

	// TODO: 实现SOCKS5 UDP关联
	return nil, fmt.Errorf("SOCKS5 UDP尚未实现")
}

// handshake SOCKS5握手
func (s *SOCKS5) handshake(conn net.Conn, metadata *Metadata) error {
	// 1. 客户端问候
	// 版本号 + 认证方法数量 + 认证方法
	var authMethods []byte
	if s.user != "" && s.password != "" {
		authMethods = []byte{0x05, 0x02, 0x00, 0x02} // 无认证 + 用户名密码
	} else {
		authMethods = []byte{0x05, 0x01, 0x00} // 无认证
	}

	if _, err := conn.Write(authMethods); err != nil {
		return fmt.Errorf("发送SOCKS5问候失败: %w", err)
	}

	// 2. 服务器响应
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("读取SOCKS5响应失败: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("无效的SOCKS5版本: %d", resp[0])
	}

	// 3. 认证
	if resp[1] == 0x02 {
		// 用户名密码认证
		if err := s.authenticate(conn); err != nil {
			return err
		}
	} else if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5认证失败，方法: %d", resp[1])
	}

	// 4. 发送连接请求
	if err := s.sendConnectRequest(conn, metadata); err != nil {
		return err
	}

	return nil
}

// authenticate 用户名密码认证
func (s *SOCKS5) authenticate(conn net.Conn) error {
	// 用户名密码认证格式：
	// +----+------+----------+------+----------+
	// |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
	// +----+------+----------+------+----------+
	// | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
	// +----+------+----------+------+----------+

	userLen := len(s.user)
	passLen := len(s.password)

	if userLen > 255 || passLen > 255 {
		return fmt.Errorf("用户名或密码过长")
	}

	req := make([]byte, 3+userLen+passLen)
	req[0] = 0x01 // 认证版本
	req[1] = byte(userLen)
	copy(req[2:], s.user)
	req[2+userLen] = byte(passLen)
	copy(req[3+userLen:], s.password)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("发送认证信息失败: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("读取认证响应失败: %w", err)
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("认证失败")
	}

	return nil
}

// sendConnectRequest 发送连接请求
func (s *SOCKS5) sendConnectRequest(conn net.Conn, metadata *Metadata) error {
	// SOCKS5连接请求格式：
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	var req []byte
	targetHost := metadata.Host
	targetPort := metadata.DstPort

	if targetHost != "" {
		// 域名类型
		hostLen := len(targetHost)
		req = make([]byte, 7+hostLen)
		req[0] = 0x05 // SOCKS版本
		req[1] = 0x01 // CONNECT命令
		req[2] = 0x00 // 保留
		req[3] = 0x03 // 域名类型
		req[4] = byte(hostLen)
		copy(req[5:], targetHost)
		req[5+hostLen] = byte(targetPort >> 8)
		req[6+hostLen] = byte(targetPort)
	} else {
		// IPv4类型
		ip := metadata.DstIP.As4()
		req = make([]byte, 10)
		req[0] = 0x05 // SOCKS版本
		req[1] = 0x01 // CONNECT命令
		req[2] = 0x00 // 保留
		req[3] = 0x01 // IPv4类型
		copy(req[4:8], ip[:])
		req[8] = byte(targetPort >> 8)
		req[9] = byte(targetPort)
	}

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("发送连接请求失败: %w", err)
	}

	// 读取响应
	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("读取连接响应失败: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("无效的SOCKS5版本: %d", resp[0])
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5连接失败: %d", resp[1])
	}

	return nil
}

// URLTest 健康检查
func (s *SOCKS5) URLTest(ctx context.Context, testURL string) (time.Duration, error) {
	start := time.Now()

	// 解析测试URL
	u, err := url.Parse(testURL)
	if err != nil {
		return 0, err
	}

	var port uint16 = 80
	if u.Scheme == "https" {
		port = 443
	}

	conn, err := s.DialContext(ctx, &Metadata{
		Host:    u.Hostname(),
		DstPort: port,
	})
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return time.Since(start), nil
}
