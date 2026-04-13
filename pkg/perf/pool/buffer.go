// Package pool 内存池模块
package pool

import (
	"sync"
)

const (
	// SmallBuffer 小缓冲区 4KB
	SmallBuffer = 4 * 1024
	// MediumBuffer 中缓冲区 16KB
	MediumBuffer = 16 * 1024
	// LargeBuffer 大缓冲区 32KB
	LargeBuffer = 32 * 1024
	// XLargeBuffer 超大缓冲区 64KB
	XLargeBuffer = 64 * 1024
)

// BufferPool 分级缓冲池
type BufferPool struct {
	small  sync.Pool // 4KB 池
	medium sync.Pool // 16KB 池
	large  sync.Pool // 32KB 池
	xlarge sync.Pool // 64KB 池
}

// globalPool 全局缓冲池
var globalPool = NewBufferPool()

// NewBufferPool 创建分级缓冲池
func NewBufferPool() *BufferPool {
	return &BufferPool{
		small: sync.Pool{
			New: func() interface{} {
				return make([]byte, SmallBuffer)
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				return make([]byte, MediumBuffer)
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				return make([]byte, LargeBuffer)
			},
		},
		xlarge: sync.Pool{
			New: func() interface{} {
				return make([]byte, XLargeBuffer)
			},
		},
	}
}

// Get 获取指定大小的缓冲区
func (p *BufferPool) Get(size int) []byte {
	switch {
	case size <= SmallBuffer:
		return p.small.Get().([]byte)[:size]
	case size <= MediumBuffer:
		return p.medium.Get().([]byte)[:size]
	case size <= LargeBuffer:
		return p.large.Get().([]byte)[:size]
	default:
		return p.xlarge.Get().([]byte)[:size]
	}
}

// Put 归还缓冲区
func (p *BufferPool) Put(b []byte) {
	switch cap(b) {
	case SmallBuffer:
		p.small.Put(b)
	case MediumBuffer:
		p.medium.Put(b)
	case LargeBuffer:
		p.large.Put(b)
	case XLargeBuffer:
		p.xlarge.Put(b)
	}
}

// GetSmall 获取小缓冲区
func (p *BufferPool) GetSmall() []byte {
	return p.small.Get().([]byte)
}

// GetMedium 获取中缓冲区
func (p *BufferPool) GetMedium() []byte {
	return p.medium.Get().([]byte)
}

// GetLarge 获取大缓冲区
func (p *BufferPool) GetLarge() []byte {
	return p.large.Get().([]byte)
}

// GetXLarge 获取超大缓冲区
func (p *BufferPool) GetXLarge() []byte {
	return p.xlarge.Get().([]byte)
}

// 全局函数

// Get 从全局池获取缓冲区
func Get(size int) []byte {
	return globalPool.Get(size)
}

// Put 归还缓冲区到全局池
func Put(b []byte) {
	globalPool.Put(b)
}

// GetSmall 从全局池获取小缓冲区
func GetSmall() []byte {
	return globalPool.GetSmall()
}

// GetMedium 从全局池获取中缓冲区
func GetMedium() []byte {
	return globalPool.GetMedium()
}

// GetLarge 从全局池获取大缓冲区
func GetLarge() []byte {
	return globalPool.GetLarge()
}

// GetXLarge 从全局池获取超大缓冲区
func GetXLarge() []byte {
	return globalPool.GetXLarge()
}

// BytesBufferPool bytes.Buffer 对象池
type BytesBufferPool struct {
	pool sync.Pool
}

// NewBytesBufferPool 创建 bytes.Buffer 池
func NewBytesBufferPool() *BytesBufferPool {
	return &BytesBufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new([]byte)
			},
		},
	}
}

// Get 获取 []byte
func (p *BytesBufferPool) Get() []byte {
	return *p.pool.Get().(*[]byte)
}

// Put 归还 []byte
func (p *BytesBufferPool) Put(b []byte) {
	// 重置但不保留容量
	if cap(b) > LargeBuffer {
		// 大于32KB的buffer不回收
		return
	}
	b = b[:0]
	p.pool.Put(&b)
}
