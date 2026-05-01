// Package hysteria Hysteria 传输层
package hysteria

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
)

// Config Hysteria 配置
type Config struct {
	Up         string `yaml:"up"`
	Down       string `yaml:"down"`
	Password   string `yaml:"password"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	SNI        string `yaml:"sni"`
	Obfs       string `yaml:"obfs"`
	Auth       []byte `yaml:"auth"`
	RecvBuf    int    `yaml:"recv-window-conn"`
	RecvBufUDP int    `yaml:"recv-window-conn-udp"`
}

// Client Hysteria 客户端
type Client struct {
	config   *Config
	session  interface{}
	mu       sync.Mutex
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	return &Client{config: cfg}
}

// Dial 建立连接
func (c *Client) Dial(ctx context.Context, addr string) (net.Conn, error) {
	// Hysteria 基于 QUIC
	tlsCfg := &tls.Config{
		ServerName: c.config.SNI,
		NextProtos: []string{"hysteria2"},
		MinVersion: tls.VersionTLS13,
	}

	_ = tlsCfg
	return nil, fmt.Errorf("hysteria: not fully implemented")
}

// Server Hysteria 服务端
type Server struct {
	config *Config
}

// NewServer 创建服务端
func NewServer(cfg *Config) *Server {
	return &Server{config: cfg}
}

// ListenAndServe 启动服务
func (s *Server) ListenAndServe(addr string) error {
	return fmt.Errorf("hysteria server: not implemented")
}
