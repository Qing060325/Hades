// Package zerocopy 零拷贝模块
package zerocopy

import (
	"io"
	"runtime"
	"sync"

	"github.com/Qing060325/Hades/pkg/perf/pool"
)

// Copier 拷贝器接口
type Copier interface {
	Copy(dst io.Writer, src io.Reader) (int64, error)
}

// NewCopier 创建拷贝器（跨平台）
func NewCopier() Copier {
	if runtime.GOOS == "linux" {
		return &SpliceCopier{}
	}
	return &BufferCopier{}
}

// BufferCopier 缓冲拷贝器（跨平台通用）
type BufferCopier struct {
	pool *pool.BufferPool
}

// NewBufferCopier 创建缓冲拷贝器
func NewBufferCopier() *BufferCopier {
	return &BufferCopier{
		pool: pool.NewBufferPool(),
	}
}

// Copy 拷贝数据
func (c *BufferCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	buf := c.pool.GetLarge()
	defer c.pool.Put(buf)

	return io.CopyBuffer(dst, src, buf)
}

// BiCopy 双向拷贝
func BiCopy(left, right io.ReadWriter) (int64, int64, error) {
	var leftToRight, rightToLeft int64
	var err error
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		leftToRight, err = Copy(right, left)
	}()

	go func() {
		defer wg.Done()
		rightToLeft, err = Copy(left, right)
	}()

	wg.Wait()
	return leftToRight, rightToLeft, err
}

// Copy 全局拷贝函数
var copier = NewCopier()

func Copy(dst io.Writer, src io.Reader) (int64, error) {
	return copier.Copy(dst, src)
}

// CopyN 拷贝N字节
func CopyN(dst io.Writer, src io.Reader, n int64) (int64, error) {
	return io.CopyN(dst, src, n)
}

// CopyBuffer 使用指定缓冲区拷贝
func CopyBuffer(dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	if buf == nil {
		buf = pool.GetLarge()
		defer pool.Put(buf)
	}
	return io.CopyBuffer(dst, src, buf)
}

// ReadAll 读取全部数据
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
