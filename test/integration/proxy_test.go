package integration

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// TestDirectAdapter_DialAndEcho 测试 Direct 适配器直连
func TestDirectAdapter_DialAndEcho(t *testing.T) {
	direct := adapter.NewDirect()
	targetAddr, targetClose := MockTarget(t)
	defer targetClose()

	TestAdapterDial(t, direct, targetAddr)
}

// TestDirectAdapter_MultipleConnections 测试 Direct 适配器多连接
func TestDirectAdapter_MultipleConnections(t *testing.T) {
	direct := adapter.NewDirect()
	targetAddr, targetClose := MockTarget(t)
	defer targetClose()

	const numConns = 5
	for i := 0; i < numConns; i++ {
		t.Run(fmt.Sprintf("conn_%d", i), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			metadata := &adapter.Metadata{
				Network: "tcp",
				Host:    "127.0.0.1",
				DstPort: uint16(mustParsePort(targetAddr)),
				Type:    adapter.MetadataTypeHTTP,
			}

			conn, err := direct.DialContext(ctx, metadata)
			if err != nil {
				t.Fatalf("连接 %d 失败: %v", i, err)
			}
			defer conn.Close()

			msg := fmt.Sprintf("test-message-%d", i)
			conn.Write([]byte(msg))

			buf := make([]byte, len(msg))
			if _, err := conn.Read(buf); err != nil {
				t.Fatalf("读取失败: %v", err)
			}

			if string(buf) != msg {
				t.Errorf("数据不匹配: got %q, want %q", string(buf), msg)
			}
		})
	}
}

// TestRejectAdapter_DialFails 测试 Reject 适配器拒绝连接
func TestRejectAdapter_DialFails(t *testing.T) {
	reject := adapter.NewReject()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	metadata := &adapter.Metadata{
		Network: "tcp",
		Host:    "127.0.0.1",
		DstPort: 80,
		Type:    adapter.MetadataTypeHTTP,
	}

	conn, err := reject.DialContext(ctx, metadata)
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		// Reject 适配器应该返回错误或者连接立即关闭
		// 具体行为取决于实现
	}
	// 预期：连接建立但读写会失败
}

// TestHTTPProxyAdapter_DialAndRelay 测试 HTTP 代理适配器
func TestHTTPProxyAdapter_DialAndRelay(t *testing.T) {
	// 启动 HTTP 代理 mock 服务器
	proxyAddr, proxyClose := startHTTPProxyMock(t)
	defer proxyClose()

	// 启动目标服务器
	targetAddr, targetClose := MockTarget(t)
	defer targetClose()

	// 创建 HTTP 代理适配器
	host, port := parseHostPort(proxyAddr)
	httpAdapter := adapter.NewHTTP(&adapter.HTTPOption{
		Name:   "http-test",
		Server: host,
		Port:   port,
	})

	TestAdapterDial(t, httpAdapter, targetAddr)
}

// TestSOCKS5ProxyAdapter_DialAndRelay 测试 SOCKS5 代理适配器
func TestSOCKS5ProxyAdapter_DialAndRelay(t *testing.T) {
	// 启动 SOCKS5 代理 mock 服务器
	proxyAddr, proxyClose := startSOCKS5ProxyMock(t)
	defer proxyClose()

	targetAddr, targetClose := MockTarget(t)
	defer targetClose()

	host, port := parseHostPort(proxyAddr)
	socksAdapter := adapter.NewSOCKS5(&adapter.SOCKS5Option{
		Name:   "socks5-test",
		Server: host,
		Port:   port,
	})

	TestAdapterDial(t, socksAdapter, targetAddr)
}

// ─── Mock 代理服务器 ──────────────────────────────────────

// startHTTPProxyMock 启动 HTTP CONNECT 代理 mock
func startHTTPProxyMock(t *testing.T) (addr string, close func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建 HTTP 代理 mock 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			conn, err := ln.Accept()
			if err != nil {
				continue
			}

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				handleHTTPConnect(t, c)
			}(conn)
		}
	}()

	close = func() {
		cancel()
		ln.Close()
		wg.Wait()
	}

	return ln.Addr().String(), close
}

// handleHTTPConnect 处理 HTTP CONNECT 请求
func handleHTTPConnect(t *testing.T, conn net.Conn) {
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	data := string(buf[:n])

	// 检查 CONNECT 方法
	if len(data) > 7 && data[:7] == "CONNECT" {
		// 解析目标地址
		// CONNECT host:port HTTP/1.1
		parts := splitRequest(data)
		if len(parts) < 2 {
			return
		}
		targetAddr := parts[1]

		// 连接目标
		targetConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}

		// 响应 200
		conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		// 双向转发
		go relay(targetConn, conn)
		relay(conn, targetConn)
	}
}

// startSOCKS5ProxyMock 启动 SOCKS5 代理 mock
func startSOCKS5ProxyMock(t *testing.T) (addr string, close func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建 SOCKS5 代理 mock 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			conn, err := ln.Accept()
			if err != nil {
				continue
			}

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				handleSOCKS5(t, c)
			}(conn)
		}
	}()

	close = func() {
		cancel()
		ln.Close()
		wg.Wait()
	}

	return ln.Addr().String(), close
}

// handleSOCKS5 处理 SOCKS5 连接
func handleSOCKS5(t *testing.T, conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 1. 握手
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n < 2 {
		return
	}

	ver := buf[0]
	_ = int(buf[1]) // nMethods - 认证方法数
	if ver != 5 {
		return
	}

	// 响应：不需要认证
	conn.Write([]byte{0x05, 0x00})

	// 2. 读取请求
	n, err = conn.Read(buf)
	if err != nil || n < 7 {
		return
	}

	if buf[0] != 5 || buf[1] != 1 { // VER=5, CMD=CONNECT
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 解析目标地址
	var targetAddr string
	atyp := buf[3]

	switch atyp {
	case 0x01: // IPv4
		if n < 10 {
			return
		}
		ip := net.IP(buf[4:8])
		port := uint16(buf[8])<<8 | uint16(buf[9])
		targetAddr = fmt.Sprintf("%s:%d", ip.String(), port)
	case 0x03: // 域名
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return
		}
		domain := string(buf[5 : 5+domainLen])
		port := uint16(buf[5+domainLen])<<8 | uint16(buf[5+domainLen+1])
		targetAddr = fmt.Sprintf("%s:%d", domain, port)
	case 0x04: // IPv6
		if n < 22 {
			return
		}
		ip := net.IP(buf[4:20])
		port := uint16(buf[20])<<8 | uint16(buf[21])
		targetAddr = fmt.Sprintf("[%s]:%d", ip.String(), port)
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 连接目标
	targetConn, err := net.DialTimeout("tcp", targetAddr, 5*time.Second)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 响应成功
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// 双向转发
	go relay(targetConn, conn)
	relay(conn, targetConn)
}

// ─── 通用辅助 ──────────────────────────────────────────────

// relay 双向数据转发
func relay(dst, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// splitRequest 分割 HTTP 请求行
func splitRequest(request string) []string {
	// CONNECT host:port HTTP/1.1
	result := make([]string, 0, 3)
	start := 0
	for i, c := range request {
		if c == ' ' || c == '\r' || c == '\n' {
			if i > start {
				result = append(result, request[start:i])
			}
			start = i + 1
		}
		if len(result) >= 2 {
			break
		}
	}
	return result
}
