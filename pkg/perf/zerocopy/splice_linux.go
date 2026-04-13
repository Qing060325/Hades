//go:build linux

// Package zerocopy Linux splice 零拷贝实现
package zerocopy

import (
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	// SpliceBlockSize splice块大小
	SpliceBlockSize = 65536
)

// SpliceCopier Linux splice零拷贝器
type SpliceCopier struct {
	pipePool sync.Pool
}

// NewSpliceCopier 创建splice拷贝器
func NewSpliceCopier() *SpliceCopier {
	return &SpliceCopier{
		pipePool: sync.Pool{
			New: func() interface{} {
				var fds [2]int
				if err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
					return nil
				}
				return &pipe{r: fds[0], w: fds[1]}
			},
		},
	}
}

type pipe struct {
	r int
	w int
}

// Copy 使用splice进行零拷贝
func (c *SpliceCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	// 转换为 *os.File
	dstFile, dstOk := dst.(*os.File)
	srcFile, srcOk := src.(*os.File)

	if dstOk && srcOk {
		return c.spliceCopy(dstFile, srcFile)
	}

	// 无法使用splice，回退到缓冲拷贝
	return c.bufferCopy(dst, src)
}

// spliceCopy 使用splice系统调用拷贝
func (c *SpliceCopier) spliceCopy(dst, src *os.File) (int64, error) {
	pipe := c.getPipe()
	if pipe == nil {
		return c.bufferCopy(dst, src)
	}
	defer c.putPipe(pipe)

	var total int64

	for {
		// splice: src -> pipe
		n, err := unix.Splice(
			int(src.Fd()), nil,
			pipe.w, nil,
			SpliceBlockSize,
			unix.SPLICE_F_MOVE|unix.SPLICE_F_NONBLOCK,
		)

		if n <= 0 {
			if err == unix.EAGAIN {
				continue
			}
			if err == unix.EINTR {
				continue
			}
			if n == 0 || err == io.EOF {
				return total, nil
			}
			return total, err
		}

		// splice: pipe -> dst
		written, err := unix.Splice(
			pipe.r, nil,
			int(dst.Fd()), nil,
			int(n),
			unix.SPLICE_F_MOVE,
		)

		total += int64(written)

		if err != nil {
			return total, err
		}
	}
}

// bufferCopy 缓冲拷贝（回退方案）
func (c *SpliceCopier) bufferCopy(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	return io.CopyBuffer(dst, src, buf)
}

// getPipe 获取管道
func (c *SpliceCopier) getPipe() *pipe {
	if v := c.pipePool.Get(); v != nil {
		return v.(*pipe)
	}
	// 创建新管道
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return nil
	}
	return &pipe{r: fds[0], w: fds[1]}
}

// putPipe 归还管道
func (c *SpliceCopier) putPipe(p *pipe) {
	c.pipePool.Put(p)
}

// SendFileCopier sendfile零拷贝器
type SendFileCopier struct{}

// NewSendFileCopier 创建sendfile拷贝器
func NewSendFileCopier() *SendFileCopier {
	return &SendFileCopier{}
}

// Copy 使用sendfile拷贝（仅适用于文件到socket）
func (c *SendFileCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	dstFile, dstOk := dst.(*os.File)
	srcFile, srcOk := src.(*os.File)

	if !dstOk || !srcOk {
		return 0, unix.EINVAL
	}

	var offset int64
	n, err := unix.Sendfile(int(dstFile.Fd()), int(srcFile.Fd()), &offset, 0)
	return int64(n), err
}
