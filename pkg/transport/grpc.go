// Package transport gRPC 传输实现
package transport

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// gRPC 传输头部
const (
	grpcHeaderPrefix = "Content-Type: application/grpc\r\n"
	grpcPathPrefix   = "/"

	// gRPC 帧头格式: [1字节压缩标志][4字节长度]
	grpcHeaderLen = 5
	grpcNoCompress = 0
)

// GRPCConn gRPC 传输连接封装
type GRPCConn struct {
	net.Conn
	serviceName string
	reader      *grpcReader
	writer      *grpcWriter
	multiMode   bool
}

// GRPCConfig gRPC 配置
type GRPCConfig struct {
	ServiceName    string
	MultiMode      bool
	InitialWindows int
	IdleTimeout    time.Duration
	HealthCheck    time.Duration
}

// NewGRPCConn 创建 gRPC 连接
func NewGRPCConn(conn net.Conn, serviceName string, multiMode bool) *GRPCConn {
	gc := &GRPCConn{
		Conn:        conn,
		serviceName: serviceName,
		multiMode:   multiMode,
	}

	gc.reader = &grpcReader{conn: conn}
	gc.writer = &grpcWriter{conn: conn, serviceName: serviceName, multiMode: multiMode}

	return gc
}

// grpcReader gRPC 帧读取器
type grpcReader struct {
	conn net.Conn
	buf  []byte
}

// grpcWriter gRPC 帧写入器
type grpcWriter struct {
	conn        net.Conn
	serviceName string
	multiMode   bool
	mu          sync.Mutex
}

// Read 读取 gRPC 帧
func (r *grpcReader) Read(b []byte) (int, error) {
	// 读取帧头
	header := make([]byte, grpcHeaderLen)
	if _, err := io.ReadFull(r.conn, header); err != nil {
		return 0, err
	}

	// 解析长度
	length := binary.BigEndian.Uint32(header[1:5])
	if length > 0 {
		if int(length) > len(b) {
			return 0, io.ErrShortBuffer
		}
		if _, err := io.ReadFull(r.conn, b[:length]); err != nil {
			return 0, err
		}
		return int(length), nil
	}

	return 0, nil
}

// Write 写入 gRPC 帧
func (w *grpcWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 写入帧头
	header := make([]byte, grpcHeaderLen)
	header[0] = grpcNoCompress
	binary.BigEndian.PutUint32(header[1:5], uint32(len(b)))

	if _, err := w.conn.Write(header); err != nil {
		return 0, err
	}

	// 写入数据
	return w.conn.Write(b)
}

// DialGRPC 通过 gRPC 拨号
func DialGRPC(ctx context.Context, addr, serviceName string, multiMode bool) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// 发送 HTTP/2 请求头
	req := fmt.Sprintf("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}

	// 读取服务器响应
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("gRPC 握手失败: %w", err)
		}
		if line == "\r\n" {
			break
		}
	}

	gc := NewGRPCConn(conn, serviceName, multiMode)
	return gc, nil
}
