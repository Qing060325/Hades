package integration

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/adapter/shadowsocks"
)

// TestShadowsocksAEAD_Creation 测试 Shadowsocks AEAD 适配器创建
func TestShadowsocksAEAD_Creation(t *testing.T) {
	ciphers := []struct {
		name     string
		cipher   string
		password string
		wantErr  bool
	}{
		{"AES-256-GCM", "aes-256-gcm", "test-password-ss-256", false},
		{"AES-128-GCM", "aes-128-gcm", "test-password-ss-128", false},
		{"Chacha20-Poly1305", "chacha20-ietf-poly1305", "test-password-ss-chacha", false},
		{"无效密码", "invalid-cipher", "test", true},
	}

	for _, tc := range ciphers {
		t.Run(tc.name, func(t *testing.T) {
			adapt, err := shadowsocks.NewAdapter("ss-test", "127.0.0.1", 8388, tc.cipher, tc.password)
			if tc.wantErr {
				if err == nil {
					t.Error("期望错误但没有返回")
				}
				return
			}
			if err != nil {
				t.Fatalf("创建适配器失败: %v", err)
			}

			if adapt.Name() != "ss-test" {
				t.Errorf("Name() = %q, want %q", adapt.Name(), "ss-test")
			}
			if adapt.Type() != adapter.TypeShadowsocks {
				t.Errorf("Type() = %q, want %q", adapt.Type(), adapter.TypeShadowsocks)
			}
			if adapt.SupportUDP() {
				t.Log("Shadowsocks 适配器支持 UDP")
			}
		})
	}
}

// TestShadowsocksAEAD_ConcurrentDial 测试 Shadowsocks 并发拨号
// 注意：此测试需要真实的 SS 服务器才能验证数据转发
// 此处仅验证并发创建不会 panic
func TestShadowsocksAEAD_ConcurrentDial(t *testing.T) {
	cipher := "aes-256-gcm"
	password := "test-concurrent-ss"

	// 启动一个 TCP 服务器来接受连接
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建服务器失败: %v", err)
	}
	defer ln.Close()

	serverAddr := ln.Addr().String()
	host, port := parseHostPort(serverAddr)

	adapt, err := shadowsocks.NewAdapter("ss-concurrent", host, port, cipher, password)
	if err != nil {
		t.Fatalf("创建适配器失败: %v", err)
	}

	// 后台接受连接
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				c.Read(buf)
			}(conn)
		}
	}()

	const concurrency = 10
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			metadata := &adapter.Metadata{
				Network: "tcp",
				Host:    "127.0.0.1",
				DstPort: 80,
				Type:    adapter.MetadataTypeSOCKS,
			}

			conn, err := adapt.DialContext(ctx, metadata)
			if err != nil {
				// 连接失败是预期的（因为没有真正的 SS 服务器）
				t.Logf("goroutine %d: 拨号失败（预期行为）: %v", id, err)
				return
			}
			conn.Close()
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发测试异常错误: %v", err)
	}
}
