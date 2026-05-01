//go:build !linux

// Package zerocopy 零拷贝模块 - 非 Linux 平台回退实现
package zerocopy

import (
	"io"
	"net"
	"sync"

	"github.com/Qing060325/Hades/pkg/perf/pool"
)

// TCPRelay 在两个 TCP 连接之间进行双向转发（非 Linux 回退到 io.CopyBuffer）
func TCPRelay(left, right *net.TCPConn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
		left.CloseWrite()
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
		right.CloseWrite()
	}()

	wg.Wait()
}

// isTCPSocket 检查连接是否为 TCP 连接
func isTCPSocket(conn net.Conn) (*net.TCPConn, bool) {
	tc, ok := conn.(*net.TCPConn)
	return tc, ok
}
