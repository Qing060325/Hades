// Package xhttp XHTTP 传输层 (WebSocket/HTTP2/QUIC over HTTP)
package xhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// TransportType 传输类型
type TransportType string

const (
	TransportHTTP    TransportType = "http"
	TransportWS      TransportType = "ws"
	TransportGRPC    TransportType = "grpc"
	TransportH2      TransportType = "h2"
	TransportQUIC    TransportType = "quic"
)

// Config XHTTP 配置
type Config struct {
	Type        TransportType `yaml:"type"`
	Host        string        `yaml:"host"`
	Path        string        `yaml:"path"`
	Headers     map[string]string `yaml:"headers"`
	TLS         bool          `yaml:"tls"`
	ServerName  string        `yaml:"server-name"`
	Fingerprint string        `yaml:"fingerprint"`
	MaxEarlyData int          `yaml:"max-early-data"`
	EarlyDataHeaderName string `yaml:"early-data-header-name"`
}

// Client XHTTP 客户端
type Client struct {
	config    *Config
	transport http.RoundTripper
}

// NewClient 创建 XHTTP 客户端
func NewClient(cfg *Config) *Client {
	return &Client{config: cfg}
}

// DialContext 建立连接
func (c *Client) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	switch c.config.Type {
	case TransportWS:
		return c.dialWebSocket(ctx, addr)
	case TransportH2:
		return c.dialHTTP2(ctx, addr)
	case TransportGRPC:
		return c.dialGRPC(ctx, addr)
	default:
		return c.dialHTTP(ctx, addr)
	}
}

func (c *Client) dialWebSocket(ctx context.Context, addr string) (net.Conn, error) {
	// WebSocket 握手
	scheme := "ws"
	if c.config.TLS {
		scheme = "wss"
	}

	u := url.URL{
		Scheme: scheme,
		Host:   addr,
		Path:   c.config.Path,
	}

	_ = u.String()
	// 实现 WebSocket 握手
	return nil, fmt.Errorf("websocket not implemented")
}

func (c *Client) dialHTTP2(ctx context.Context, addr string) (net.Conn, error) {
	// HTTP/2 连接
	return nil, fmt.Errorf("http2 not implemented")
}

func (c *Client) dialGRPC(ctx context.Context, addr string) (net.Conn, error) {
	// gRPC 连接
	return nil, fmt.Errorf("grpc not implemented")
}

func (c *Client) dialHTTP(ctx context.Context, addr string) (net.Conn, error) {
	// 普通 HTTP 隧道
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	if c.config.TLS {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: c.config.ServerName,
			NextProtos: []string{"h2", "http/1.1"},
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return conn, nil
}

// Server XHTTP 服务端
type Server struct {
	config   *Config
	handler  http.Handler
}

// NewServer 创建 XHTTP 服务端
func NewServer(cfg *Config, handler http.Handler) *Server {
	return &Server{config: cfg, handler: handler}
}

// ListenAndServe 启动服务
func (s *Server) ListenAndServe(addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.handler,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	if s.config.TLS {
		return server.ListenAndServeTLS("", "")
	}
	return server.ListenAndServe()
}

// CopyBuffer 双向拷贝
func CopyBuffer(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024)
	_, err := io.CopyBuffer(dst, src, buf)
	return err
}
