// Package restls RESTLS 混淆传输实现
// RESTLS 将 TLS 流量伪装为正常的 HTTPS 流量，通过密码和序列填充来抵抗主动探测
package restls

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"
)

// Config RESTLS 配置
type Config struct {
	Password     string `yaml:"password"`       // RESTLS 密码
	PasswordHash string `yaml:"password-sha256"` // 密码的 SHA256 哈希（向后兼容）
}

// Conn RESTLS 连接封装
type Conn struct {
	net.Conn
	config    *Config
	handshook bool
	hashKey   [32]byte // 由密码派生的 256 位哈希密钥
}

// NewConn 创建 RESTLS 连接
func NewConn(conn net.Conn, cfg *Config) (*Conn, error) {
	if cfg == nil {
		return nil, fmt.Errorf("RESTLS 配置不能为空")
	}

	c := &Conn{
		Conn:   conn,
		config: cfg,
	}

	// 优先使用 password-sha256（向后兼容），否则从 password 计算
	if cfg.PasswordHash != "" {
		// 直接使用提供的 SHA256 哈希
		hashBytes := []byte(cfg.PasswordHash)
		if len(hashBytes) != 32 {
			return nil, fmt.Errorf("password-sha256 长度错误: 需要 32 字节，得到 %d 字节", len(hashBytes))
		}
		copy(c.hashKey[:], hashBytes)
	} else if cfg.Password != "" {
		// 从密码计算 SHA256
		hash := sha256.Sum256([]byte(cfg.Password))
		c.hashKey = hash
	} else {
		return nil, fmt.Errorf("RESTLS 必须设置 password 或 password-sha256")
	}

	return c, nil
}

// Dial 建立 RESTLS 连接
func Dial(addr string, cfg *Config) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("RESTLS TCP 连接失败: %w", err)
	}

	restlsConn, err := NewConn(conn, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// 执行 RESTLS 握手
	if err := restlsConn.handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("RESTLS 握手失败: %w", err)
	}

	return restlsConn, nil
}

// Read 读取数据
func (c *Conn) Read(b []byte) (int, error) {
	if !c.handshook {
		if err := c.handshake(); err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *Conn) Write(b []byte) (int, error) {
	if !c.handshook {
		if err := c.handshake(); err != nil {
			return 0, err
		}
	}
	return c.Conn.Write(b)
}

// handshake 执行 RESTLS 握手
// RESTLS 握手流程:
// 1. 将 TLS 握手伪装为正常的 HTTPS 请求
// 2. 使用 hashKey 作为序列填充的种子
// 3. 基于数据包序列号添加/移除填充，使流量模式看起来像普通 HTTPS
// 4. 主动探测保护: 使用 hashKey 派生的密钥验证连接合法性
func (c *Conn) handshake() error {
	if c.handshook {
		return nil
	}

	// TODO: 实现完整的 RESTLS 握手
	// 核心逻辑:
	// 1. 使用 hashKey 派生 padding 密钥
	// 2. 根据数据包序号生成确定性填充（使流量模式匹配正常 HTTPS）
	// 3. 实现 sequence-based padding:
	//    - 每个数据包的填充长度由 (hashKey + sequence_number) 决定
	//    - 填充模式看起来像正常的 HTTP 响应头/体
	// 4. 抗主动探测: 只有知道 password 的客户端才能生成正确的填充序列

	c.handshook = true
	return nil
}

// WrapListener 包装 Listener 为 RESTLS 监听器
func WrapListener(ln net.Listener, cfg *Config) (net.Listener, error) {
	if cfg == nil {
		return nil, fmt.Errorf("RESTLS 配置不能为空")
	}

	return &restlsListener{
		Listener: ln,
		config:   cfg,
	}, nil
}

// restlsListener RESTLS 监听器
type restlsListener struct {
	net.Listener
	config *Config
}

// Accept 接受连接
func (l *restlsListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	restlsConn, err := NewConn(conn, l.config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return restlsConn, nil
}
