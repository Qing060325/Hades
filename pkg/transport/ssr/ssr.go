// Package ssr ShadowsocksR 传输层
package ssr

import (
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
)

// Config SSR 配置
type Config struct {
	Password  string `yaml:"password"`
	Method    string `yaml:"method"`
	Obfs      string `yaml:"obfs"`
	ObfsParam string `yaml:"obfs-param"`
	Proto     string `yaml:"protocol"`
	ProtoParam string `yaml:"protocol-param"`
}

// Conn SSR 连接
type Conn struct {
	net.Conn
	config   *Config
	encrypt  cipher.AEAD
	decrypt  cipher.AEAD
	iv       []byte
	key      []byte
}

// NewConn 创建 SSR 连接
func NewConn(conn net.Conn, cfg *Config) (*Conn, error) {
	c := &Conn{Conn: conn, config: cfg}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Conn) init() error {
	// 派生密钥
	c.key = deriveKey([]byte(c.config.Password), 16)
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

// Dial 建立 SSR 连接
func Dial(addr string, cfg *Config) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return NewConn(conn, cfg)
}

func deriveKey(password []byte, keyLen int) []byte {
	key := make([]byte, keyLen)
	h := md5.New()
	for i := 0; i < keyLen; i++ {
		h.Write(password)
		copy(key[i:i+1], h.Sum(nil)[:1])
	}
	return key
}

// ObfsType 混淆类型
type ObfsType string

const (
	ObfsPlain    ObfsType = "plain"
	ObfsHTTP     ObfsType = "http_simple"
	ObfsHTTPPost ObfsType = "http_post"
	ObfsTLS      ObfsType = "tls1.2_ticket_auth"
)

// ProtoType 协议类型
type ProtoType string

const (
	ProtoOrigin  ProtoType = "origin"
	ProtoAuthAES ProtoType = "auth_aes128_md5"
	ProtoAuthSHA ProtoType = "auth_sha1_v4"
)

// EncodeRequest 编码请求
func (c *Conn) EncodeRequest(data []byte) ([]byte, error) {
	// SSR 请求编码
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	return append(header, data...), nil
}

// DecodeResponse 解码响应
func (c *Conn) DecodeResponse(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("ssr: response too short")
	}
	return data[4:], nil
}
