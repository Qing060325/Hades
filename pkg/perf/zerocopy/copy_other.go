//go:build !linux

// Package zerocopy 非Linux平台的拷贝实现
package zerocopy

import (
	"io"

	"github.com/hades/hades/pkg/perf/pool"
)

// SpliceCopier 在非Linux平台使用缓冲拷贝
type SpliceCopier struct {
	bufferCopier *BufferCopier
}

// NewSpliceCopier 创建拷贝器
func NewSpliceCopier() *SpliceCopier {
	return &SpliceCopier{
		bufferCopier: NewBufferCopier(),
	}
}

// Copy 拷贝数据
func (c *SpliceCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	return c.bufferCopier.Copy(dst, src)
}

// SendFileCopier sendfile拷贝器（非Linux）
type SendFileCopier struct{}

// NewSendFileCopier 创建sendfile拷贝器
func NewSendFileCopier() *SendFileCopier {
	return &SendFileCopier{}
}

// Copy 拷贝数据
func (c *SendFileCopier) Copy(dst io.Writer, src io.Reader) (int64, error) {
	buf := pool.GetLarge()
	defer pool.Put(buf)
	return io.CopyBuffer(dst, src, buf)
}
