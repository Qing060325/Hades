// Package simpleobfs Simple Obfs 混淆
package simpleobfs

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

// ObfsType 混淆类型
type ObfsType string

const (
	ObfsHTTP ObfsType = "http"
	ObfsTLS  ObfsType = "tls"
)

// Config Obfs 配置
type Config struct {
	Type ObfsType `yaml:"type"`
	Host string   `yaml:"host"`
	Port int      `yaml:"port"`
	Path string   `yaml:"path"`
}

// Conn Obfs 连接
type Conn struct {
	net.Conn
	config    *Config
	obfsType  ObfsType
	handshook bool
	reader    *bufio.Reader
}

// NewConn 创建 Obfs 连接
func NewConn(conn net.Conn, cfg *Config) *Conn {
	return &Conn{
		Conn:     conn,
		config:   cfg,
		obfsType: cfg.Type,
		reader:   bufio.NewReader(conn),
	}
}

// Read 读取数据
func (c *Conn) Read(b []byte) (int, error) {
	if !c.handshook {
		if err := c.serverHandshake(); err != nil {
			return 0, err
		}
		c.handshook = true
	}
	return c.reader.Read(b)
}

// Write 写入数据
func (c *Conn) Write(b []byte) (int, error) {
	if !c.handshook {
		if err := c.clientHandshake(); err != nil {
			return 0, err
		}
		c.handshook = true
	}
	return c.Conn.Write(b)
}

// clientHandshake 客户端握手
func (c *Conn) clientHandshake() error {
	switch c.obfsType {
	case ObfsHTTP:
		return c.httpHandshake()
	case ObfsTLS:
		return c.tlsHandshake()
	}
	return nil
}

func (c *Conn) httpHandshake() error {
	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		Path:   c.config.Path,
	}
	req := &http.Request{
		Method: "GET",
		URL:    u,
		Host:   c.config.Host,
		Header: http.Header{
			"User-Agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
		},
	}
	return req.Write(c.Conn)
}

func (c *Conn) tlsHandshake() error {
	// TLS ClientHello 伪装
	return nil
}

// serverHandshake 服务端握手
func (c *Conn) serverHandshake() error {
	if c.obfsType == ObfsHTTP {
		// 读取 HTTP 请求
		_, err := http.ReadRequest(c.reader)
		if err != nil {
			return err
		}
		// 发送 HTTP 响应
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
		_, err = c.Conn.Write([]byte(resp))
		return err
	}
	return nil
}

// WrapListener 包装 Listener
func WrapListener(ln net.Listener, cfg *Config) net.Listener {
	return &obfsListener{Listener: ln, config: cfg}
}

type obfsListener struct {
	net.Listener
	config *Config
}

func (l *obfsListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return NewConn(conn, l.config), nil
}
