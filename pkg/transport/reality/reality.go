// Package reality Reality 协议传输层
package reality

import (
	"crypto/tls"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
)

// Config Reality 配置
type Config struct {
	ServerName  string   `yaml:"server-name"`
	ServerPort  int      `yaml:"server-port"`
	PrivateKey  string   `yaml:"private-key"`
	PublicKey   string   `yaml:"public-key"`
	ShortID     string   `yaml:"short-id"`
	SpiderX     string   `yaml:"spider-x"`
	Target      string   `yaml:"target"`
	Fingerprint string   `yaml:"fingerprint"`
	Alpn        []string `yaml:"alpn"`
}

// Client Reality 客户端
type Client struct {
	config *Config
}

// NewClient 创建 Reality 客户端
func NewClient(cfg *Config) *Client {
	return &Client{config: cfg}
}

// DialContext 建立 Reality 连接
func (c *Client) DialContext(addr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	// 使用 uTLS 进行指纹模拟
	utlsConn := utls.UClient(conn, &utls.Config{
		ServerName:         c.config.ServerName,
		InsecureSkipVerify: true,
	}, c.getFingerprint())

	if err := utlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("reality handshake: %w", err)
	}

	return utlsConn, nil
}

// getFingerprint 获取 uTLS 指纹
func (c *Client) getFingerprint() utls.ClientHelloID {
	switch c.config.Fingerprint {
	case "chrome":
		return utls.HelloChrome_Auto
	case "firefox":
		return utls.HelloFirefox_Auto
	case "safari":
		return utls.HelloSafari_Auto
	case "ios":
		return utls.HelloIOS_Auto
	case "android":
		return utls.HelloAndroid_11_OkHttp
	case "edge":
		return utls.HelloEdge_Auto
	case "random":
		return utls.HelloRandomized
	default:
		return utls.HelloChrome_Auto
	}
}

// Server Reality 服务端
type Server struct {
	config    *Config
	tlsConfig *tls.Config
}

// NewServer 创建 Reality 服务端
func NewServer(cfg *Config) (*Server, error) {
	return &Server{
		config: cfg,
		tlsConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}, nil
}

// ListenAndServe 启动 Reality 服务
func (s *Server) ListenAndServe(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
}
