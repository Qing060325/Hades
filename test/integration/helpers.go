// Package integration 协议自动化测试套件
//
// 提供 mock 服务器和端到端协议验证框架：
//   - Shadowsocks AEAD mock 服务器
//   - HTTP/SOCKS5 代理 mock 服务器
//   - 回环测试：客户端 → mock 代理服务器 → mock 目标服务器
//   - 断言：数据完整性、延迟、并发安全性
package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
)

// ─── Mock 目标服务器 ──────────────────────────────────────

// MockTarget 启动一个简单的 TCP echo 服务器作为测试目标
func MockTarget(t *testing.T) (addr string, close func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建 mock 目标服务器失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				io.Copy(c, c) // Echo: 将收到的数据原样返回
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

// MockHTTPTarget 启动一个 HTTP 服务器返回固定内容
func MockHTTPTarget(t *testing.T, response string) (addr string, close func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Target", "ok")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	server := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建 mock HTTP 服务器失败: %v", err)
	}

	go server.Serve(ln)

	close = func() {
		server.Close()
	}

	return ln.Addr().String(), close
}

// ─── 测试辅助函数 ──────────────────────────────────────────

// TestAdapterDial 测试适配器拨号和数据回环
func TestAdapterDial(t *testing.T, adapt adapter.Adapter, targetAddr string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metadata := &adapter.Metadata{
		NetWork: "tcp",
		Host:    strings.Split(targetAddr, ":")[0],
		DstPort: uint16(mustParsePort(targetAddr)),
		Type:    adapter.MetadataTypeSOCKS,
	}

	// 如果 Host 为空（如 127.0.0.1），设置 DstIP
	if metadata.Host == "" || metadata.Host == "127.0.0.1" {
		metadata.Host = "127.0.0.1"
	}

	conn, err := adapt.DialContext(ctx, metadata)
	if err != nil {
		t.Fatalf("DialContext 失败: %v", err)
	}
	defer conn.Close()

	// 发送测试数据
	testData := []byte("hello-hades-protocol-test")
	if _, err := conn.Write(testData); err != nil {
		t.Fatalf("写入数据失败: %v", err)
	}

	// 读取 echo 回来的数据
	buf := make([]byte, len(testData))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("读取数据失败: %v", err)
	}

	// 验证数据完整性
	if string(buf) != string(testData) {
		t.Fatalf("数据不匹配: got %q, want %q", string(buf), string(testData))
	}
}

// TestAdapterURLTest 测试适配器健康检查
func TestAdapterURLTest(t *testing.T, adapt adapter.Adapter) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latency, err := adapt.URLTest(ctx, "http://www.gstatic.com/generate_204")
	if err != nil {
		t.Logf("URLTest 失败（可能是网络问题）: %v", err)
		return
	}

	t.Logf("%s URLTest latency: %v", adapt.Name(), latency)
}

// BenchmarkAdapterDial 基准测试适配器拨号
func BenchmarkAdapterDial(b *testing.B, adapt adapter.Adapter, targetAddr string) {
	ctx := context.Background()
	metadata := &adapter.Metadata{
		NetWork: "tcp",
		Host:    "127.0.0.1",
		DstPort: uint16(mustParsePort(targetAddr)),
		Type:    adapter.MetadataTypeSOCKS,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := adapt.DialContext(ctx, metadata)
		if err != nil {
			b.Fatalf("DialContext 失败: %v", err)
		}
		conn.Close()
	}
}

// ─── 辅助 ──────────────────────────────────────────────────

func mustParseAddr(s string) netip.Addr {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

// parseHostPort 解析 host:port
func parseHostPort(addr string) (string, int) {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

func mustParsePort(addr string) int {
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return 0
	}
	var port int
	fmt.Sscanf(parts[len(parts)-1], "%d", &port)
	return port
}
