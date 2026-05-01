// Package transport 连接池实现
package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// PoolConfig 连接池配置
type PoolConfig struct {
	MaxIdle     int           // 最大空闲连接
	MaxActive   int           // 最大活跃连接
	IdleTimeout time.Duration // 空闲超时
	MaxLifetime time.Duration // 最大生命周期
}

// DefaultPoolConfig 默认连接池配置
var DefaultPoolConfig = PoolConfig{
	MaxIdle:     8,
	MaxActive:   32,
	IdleTimeout: 5 * time.Minute,
	MaxLifetime: 30 * time.Minute,
}

// ConnPool 连接池
type ConnPool struct {
	config PoolConfig
	factory func(ctx context.Context) (net.Conn, error)

	mu        sync.Mutex
	idle      map[string][]*poolEntry
	active    map[string]int
	createdAt map[string]time.Time
}

type poolEntry struct {
	conn   net.Conn
	putAt  time.Time
}

// NewConnPool 创建连接池
func NewConnPool(config PoolConfig, factory func(ctx context.Context) (net.Conn, error)) *ConnPool {
	return &ConnPool{
		config:    config,
		factory:   factory,
		idle:      make(map[string][]*poolEntry),
		active:    make(map[string]int),
		createdAt: make(map[string]time.Time),
	}
}

// Get 获取连接
func (p *ConnPool) Get(ctx context.Context, key string) (net.Conn, error) {
	p.mu.Lock()

	// 尝试从空闲池获取
	if entries, ok := p.idle[key]; ok {
		now := time.Now()
		for len(entries) > 0 {
			entry := entries[len(entries)-1]
			entries = entries[:len(entries)-1]

			// 检查是否过期
			if now.Sub(entry.putAt) > p.config.IdleTimeout {
				entry.conn.Close()
				continue
			}

			// 检查最大生命周期
			if created, ok := p.createdAt[key]; ok {
				if now.Sub(created) > p.config.MaxLifetime {
					entry.conn.Close()
					delete(p.createdAt, key)
					continue
				}
			}

			// 检查连接是否有效
			// 使用 SetReadDeadline + 0 字节读取来探测连接状态
			if err := entry.conn.SetReadDeadline(time.Now()); err != nil {
				entry.conn.Close()
				continue
			}

			// 尝试读取来检测连接状态（deadline 已过，立即返回）
			probeBuf := make([]byte, 1)
			_, probeErr := entry.conn.Read(probeBuf)

			// 重置 deadline
			entry.conn.SetReadDeadline(time.Time{})

			if probeErr != nil {
				// 超时说明连接仍然存活（只是没有数据或 deadline 已过）
				if netErr, ok := probeErr.(net.Error); ok && netErr.Timeout() {
					// 连接存活，继续使用
				} else {
					// 其他错误（EOF、连接重置等）说明连接已死
					entry.conn.Close()
					continue
				}
			} else {
				// 读到了 1 字节数据，连接存活但消耗了 1 字节
				// 为简单起见关闭此连接（实际数据不应在空闲连接上出现）
				entry.conn.Close()
				continue
			}

			p.idle[key] = entries
			p.mu.Unlock()
			return &pooledConn{Conn: entry.conn, pool: p, key: key}, nil
		}

		p.idle[key] = entries
	}

	// 检查活跃连接数
	if p.active[key] >= p.config.MaxActive {
		p.mu.Unlock()
		return nil, fmt.Errorf("连接池已满: %s", key)
	}

	p.active[key]++
	if _, ok := p.createdAt[key]; !ok {
		p.createdAt[key] = time.Now()
	}
	p.mu.Unlock()

	// 创建新连接
	conn, err := p.factory(ctx)
	if err != nil {
		p.mu.Lock()
		p.active[key]--
		p.mu.Unlock()
		return nil, err
	}

	return &pooledConn{Conn: conn, pool: p, key: key}, nil
}

// Put 归还连接
func (p *ConnPool) Put(key string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查是否超过最大空闲数
	if len(p.idle[key]) >= p.config.MaxIdle {
		conn.Close()
		p.active[key]--
		return
	}

	p.idle[key] = append(p.idle[key], &poolEntry{
		conn:  conn,
		putAt: time.Now(),
	})
	p.active[key]--
}

// Close 关闭连接池
func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, entries := range p.idle {
		for _, entry := range entries {
			entry.conn.Close()
		}
		delete(p.idle, key)
	}
}

// Stats 返回连接池统计
func (p *ConnPool) Stats() map[string]PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make(map[string]PoolStats)
	for key, entries := range p.idle {
		stats[key] = PoolStats{
			Idle:   len(entries),
			Active: p.active[key],
		}
	}
	return stats
}

// PoolStats 连接池统计
type PoolStats struct {
	Idle   int
	Active int
}

// pooledConn 连接池连接封装
type pooledConn struct {
	net.Conn
	pool *ConnPool
	key  string
	closed bool
}

// Close 关闭连接（归还到池中）
func (c *pooledConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	c.pool.Put(c.key, c.Conn)
	return nil
}

// RealClose 真正关闭连接
func (c *pooledConn) RealClose() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Conn.Close()
}
