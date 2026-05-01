// Package ssh SSH 传输层
package ssh

import (
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

// Config SSH 配置
type Config struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	PrivateKey string `yaml:"private-key"`
}

// Client SSH 客户端
type Client struct {
	config    *Config
	sshClient *ssh.Client
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	return &Client{config: cfg}
}

// Connect 建立连接
func (c *Client) Connect() error {
	authMethods := []ssh.AuthMethod{}

	if c.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.config.Password))
	}

	if c.config.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(c.config.PrivateKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            c.config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return err
	}

	c.sshClient = client
	return nil
}

// Dial 通过 SSH 隧道拨号
func (c *Client) Dial(network, addr string) (net.Conn, error) {
	if c.sshClient == nil {
		if err := c.Connect(); err != nil {
			return nil, err
		}
	}
	return c.sshClient.Dial(network, addr)
}

// Close 关闭连接
func (c *Client) Close() error {
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}
