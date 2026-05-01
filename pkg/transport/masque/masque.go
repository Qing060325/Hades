// Package masque MASQUE 传输层 (HTTP/3 代理)
package masque

import (
	"fmt"
	"net"
)

// Config MASQUE 配置
type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Path     string `yaml:"path"`
	Password string `yaml:"password"`
	SNI      string `yaml:"sni"`
}

// Client MASQUE 客户端
type Client struct {
	config *Config
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	return &Client{config: cfg}
}

// Dial 建立 MASQUE 连接
func (c *Client) Dial(network, addr string) (net.Conn, error) {
	// MASQUE 基于 HTTP/3 (QUIC)
	return nil, fmt.Errorf("masque: not implemented yet")
}
