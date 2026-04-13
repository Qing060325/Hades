// Package transport WebSocket 传输实现
package transport

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// WebSocketConn WebSocket 连接封装
type WebSocketConn struct {
	net.Conn
	reader *wsReader
	writer *wsWriter
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	Path    string
	Host    string
	Headers map[string]string
}

// NewWebSocketConn 创建 WebSocket 连接
func NewWebSocketConn(conn net.Conn, path, host string, headers map[string]string) (*WebSocketConn, error) {
	// 发送 WebSocket 握手请求
	if err := wsClientHandshake(conn, path, host, headers); err != nil {
		return nil, fmt.Errorf("WebSocket 握手失败: %w", err)
	}

	// 读取服务器响应
	if err := wsReadServerHandshake(conn); err != nil {
		return nil, fmt.Errorf("WebSocket 服务器响应失败: %w", err)
	}

	wsc := &WebSocketConn{
		Conn: conn,
	}

	return wsc, nil
}

// wsClientHandshake 发送 WebSocket 客户端握手
func wsClientHandshake(conn net.Conn, path, host string, headers map[string]string) error {
	// 生成 Sec-WebSocket-Key
	keyBytes := make([]byte, 16)
	rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)

	// 构建请求
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", path)
	req += fmt.Sprintf("Host: %s\r\n", host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"
	req += fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", key)
	req += "Sec-WebSocket-Version: 13\r\n"

	// 自定义 headers
	for k, v := range headers {
		req += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	req += "\r\n"

	_, err := conn.Write([]byte(req))
	return err
}

// wsReadServerHandshake 读取 WebSocket 服务器握手响应
func wsReadServerHandshake(conn net.Conn) error {
	reader := bufio.NewReader(conn)

	// 读取状态行
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	if !strings.Contains(statusLine, "101") {
		return fmt.Errorf("WebSocket 握手失败: %s", strings.TrimSpace(statusLine))
	}

	// 读取头部直到空行
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" {
			break
		}
	}

	return nil
}

// wsReader WebSocket 帧读取器
type wsReader struct {
	conn net.Conn
	mask bool
}

// wsWriter WebSocket 帧写入器
type wsWriter struct {
	conn     net.Conn
	mask     bool
	fragBuf  []byte
	mu       sync.Mutex
}

// wsFrame WebSocket 帧头
type wsFrame struct {
	Fin           bool
	Rsv1          bool
	Rsv2          bool
	Rsv3          bool
	OpCode        byte
	Mask          bool
	PayloadLength uint64
	MaskingKey    [4]byte
}

const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xa
)

// wsMaxFrameSize WebSocket 最大帧大小
const wsMaxFrameSize = 1 << 20 // 1MB

// WriteFrame 写入 WebSocket 帧
func (w *wsWriter) WriteFrame(fin bool, opCode byte, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var header []byte

	// 第一个字节
	var b1 byte
	if fin {
		b1 |= 0x80
	}
	b1 |= opCode
	header = append(header, b1)

	// 长度
	length := len(data)
	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126)
		header = append(header, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		for i := 56; i >= 0; i -= 8 {
			header = append(header, byte(length>>i))
		}
	}

	// Mask
	maskKey := [4]byte{}
	rand.Read(maskKey[:])
	header = append(header, maskKey[:]...)

	// 写入头部
	if _, err := w.conn.Write(header); err != nil {
		return err
	}

	// 写入掩码后的数据
	masked := make([]byte, len(data))
	for i, b := range data {
		masked[i] = b ^ maskKey[i%4]
	}

	_, err := w.conn.Write(masked)
	return err
}

// ReadFrame 读取 WebSocket 帧
func (r *wsReader) ReadFrame() ([]byte, byte, error) {
	// 读取头部
	header := make([]byte, 2)
	if _, err := io.ReadFull(r.conn, header); err != nil {
		return nil, 0, err
	}

	// 解析帧头
	_ = header[0]&0x80 != 0
	opCode := header[0] & 0x0f
	mask := header[1]&0x80 != 0
	payloadLen := uint64(header[1] & 0x7f)

	// 扩展长度
	switch {
	case payloadLen == 126:
		extLen := make([]byte, 2)
		if _, err := io.ReadFull(r.conn, extLen); err != nil {
			return nil, 0, err
		}
		payloadLen = uint64(extLen[0])<<8 | uint64(extLen[1])
	case payloadLen == 127:
		extLen := make([]byte, 8)
		if _, err := io.ReadFull(r.conn, extLen); err != nil {
			return nil, 0, err
		}
		payloadLen = binary.BigEndian.Uint64(extLen)
	}

	// Mask
	var maskKey [4]byte
	if mask {
		if _, err := io.ReadFull(r.conn, maskKey[:]); err != nil {
			return nil, 0, err
		}
	}

	// 读取 payload
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r.conn, payload); err != nil {
		return nil, 0, err
	}

	// 去掩码
	if mask {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	// 处理控制帧
	if opCode >= wsOpClose {
		switch opCode {
		case wsOpPing:
			// 回应 Pong
			r.writePong(payload)
		case wsOpPong:
			// 忽略 Pong
		case wsOpClose:
			return nil, 0, io.EOF
		}
		return nil, 0, nil
	}

	return payload, opCode, nil
}

func (r *wsReader) writePong(data []byte) {
	// TODO: 实现 Pong 回复
}

// DialWebSocket 通过 WebSocket 拨号
func DialWebSocket(ctx context.Context, addr, path, host string, headers map[string]string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	wsc, err := NewWebSocketConn(conn, path, host, headers)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return wsc, nil
}
