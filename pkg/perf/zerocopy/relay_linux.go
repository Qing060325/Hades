//go:build linux

// Package zerocopy 零拷贝模块 - Linux TCP relay 使用 splice
package zerocopy

import (
	"io"
	"net"
	"sync"

	"github.com/Qing060325/Hades/pkg/perf/pool"
	"golang.org/x/sys/unix"
)

// TCPRelay 在两个 TCP 连接之间进行双向零拷贝转发
// 优先使用 splice 系统调用，处理 half-close
func TCPRelay(left, right *net.TCPConn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		singleTCPRelay(left, right)
	}()

	go func() {
		defer wg.Done()
		singleTCPRelay(right, left)
	}()

	wg.Wait()
}

// singleTCPRelay 使用 splice 进行单向 TCP 数据转发
func singleTCPRelay(dst, src *net.TCPConn) {
	defer func() {
		// half-close: 关闭写入端，通知对方不再发送
		dst.CloseWrite()
	}()

	// 获取文件描述符用于 splice
	srcFile, sfErr := src.File()
	dstFile, dfErr := dst.File()
	if sfErr == nil && dfErr == nil {
		srcFd := int(srcFile.Fd())
		dstFd := int(dstFile.Fd())

		if err := spliceByFd(dstFd, srcFd); err == nil {
			srcFile.Close()
			dstFile.Close()
			return
		}

		srcFile.Close()
		dstFile.Close()
	}

	// splice 失败，回退到缓冲拷贝
	buf := pool.GetLarge()
	defer pool.Put(buf)
	io.CopyBuffer(dst, src, buf)
}

// spliceByFd 使用文件描述符进行 splice 转发
func spliceByFd(dstFd, srcFd int) error {
	// 创建管道
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return err
	}
	pipeR, pipeW := fds[0], fds[1]
	defer unix.Close(pipeR)
	defer unix.Close(pipeW)

	for {
		// splice: src -> pipe
		n, err := unix.Splice(srcFd, nil, pipeW, nil, SpliceBlockSize,
			unix.SPLICE_F_MOVE|unix.SPLICE_F_NONBLOCK)

		if n <= 0 {
			if err == unix.EAGAIN || err == unix.EINTR {
				continue
			}
			if n == 0 || err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			return nil
		}

		// splice: pipe -> dst（循环写入确保全部数据传递）
		remaining := int(n)
		for remaining > 0 {
			written, werr := unix.Splice(pipeR, nil, dstFd, nil, remaining,
				unix.SPLICE_F_MOVE)
			if written > 0 {
				remaining -= int(written)
			}
			if werr != nil {
				if werr == unix.EAGAIN || werr == unix.EINTR {
					continue
				}
				return werr
			}
		}
	}
}

// isTCPSocket 检查连接是否为 TCP 连接
func isTCPSocket(conn net.Conn) (*net.TCPConn, bool) {
	tc, ok := conn.(*net.TCPConn)
	return tc, ok
}
