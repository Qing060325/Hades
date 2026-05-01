// Package snell Snell 协议传输层
package snell

import (
	"encoding/binary"
	"fmt"
	"net"
)

// Config Snell 配置
type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	PSK      string `yaml:"psk"`       // Pre-Shared Key
	Version  int    `yaml:"version"`
	ObfsMode string `yaml:"obfs-mode"`
	ObfsHost string `yaml:"obfs-host"`
}

// Conn Snell 连接
type Conn struct {
	net.Conn
	config  *Config
	version int
}

// Dial 建立 Snell 迄接
func Dial(addr string, cfg *Config) (*Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		Conn:    conn,
		config:  cfg,
		version: cfg.Version,
	}

	if c.version == 0 {
		c.version = 3
	}

	return c, nil
}

// WriteRequest 写入请求
func (c *Conn) WriteRequest(target string, data []byte) error {
	// Snell v3 请求格式
	// header: version(1) + command(1) + id(1) + payload_len(2)
	header := make([]byte, 5)
	header[0] = byte(c.version)
	header[1] = 0x01 // connect
	header[2] = 0x00

	// target: type(1) + host_len(1) + host + port(2)
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	targetBuf := make([]byte, 0, len(host)+4)
	targetBuf = append(targetBuf, 0x01) // domain type
	targetBuf = append(targetBuf, byte(len(host)))
	targetBuf = append(targetBuf, host...)
	targetBuf = append(targetBuf, byte(port>>8), byte(port))

	payload := append(targetBuf, data...)
	binary.BigEndian.PutUint16(header[3:5], uint16(len(payload)))

	_, err = c.Conn.Write(append(header, payload...))
	return err
}

// ReadResponse 读取响应
func (c *Conn) ReadResponse() ([]byte, error) {
	header := make([]byte, 5)
	if _, err := c.Conn.Read(header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint16(header[3:5])
	data := make([]byte, length)
	if _, err := c.Conn.Read(data); err != nil {
		return nil, err
	}

	return data, nil
}

// Handshake 握手
func (c *Conn) Handshake() error {
	// 发送 handshake
	hello := []byte{byte(c.version), 0x00, 0x00, 0x00, 0x01, 0x00}
	if _, err := c.Conn.Write(hello); err != nil {
		return err
	}

	// 读取响应
	resp := make([]byte, 6)
	if _, err := c.Conn.Read(resp); err != nil {
		return err
	}

	if resp[1] != 0x00 {
		return fmt.Errorf("snell: handshake failed")
	}

	return nil
}
