// Package shadowtls ShadowTLS 混淆传输
package shadowtls

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
)

// Config ShadowTLS 配置
type Config struct {
	Version    int    `yaml:"version"`     // 0, 1, 2, 3
	Password   string `yaml:"password"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	SNI        string `yaml:"sni"`
	Fingerprint string `yaml:"fingerprint"`
}

// Conn ShadowTLS 连接
type Conn struct {
	net.Conn
	config    *Config
	handshook bool
}

// NewConn 创建 ShadowTLS 连接
func NewConn(conn net.Conn, cfg *Config) *Conn {
	return &Conn{Conn: conn, config: cfg}
}

// Dial 建立 ShadowTLS 连接
func Dial(addr string, cfg *Config) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	stlsConn := &Conn{Conn: conn, config: cfg}
	if err := stlsConn.handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	return stlsConn, nil
}

func (c *Conn) handshake() error {
	switch c.config.Version {
	case 0, 1:
		return c.handshakeV1()
	case 2:
		return c.handshakeV2()
	case 3:
		return c.handshakeV3()
	default:
		return fmt.Errorf("unsupported shadowtls version: %d", c.config.Version)
	}
}

func (c *Conn) handshakeV1() error {
	// ShadowTLS v1: 简单的 TLS 握手伪装
	// 发送 ClientHello
	return nil
}

func (c *Conn) handshakeV2() error {
	// ShadowTLS v2: 带认证的 TLS 伪装
	return nil
}

func (c *Conn) handshakeV3() error {
	// ShadowTLS v3: 最新版本
	return nil
}

// Read 读取数据
func (c *Conn) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *Conn) Write(b []byte) (int, error) {
	return c.Conn.Write(b)
}

// HMAC 计算 HMAC
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// EncodeHex hex 编码
func EncodeHex(data []byte) string {
	return hex.EncodeToString(data)
}
