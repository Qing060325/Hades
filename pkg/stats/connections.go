// Package stats 连接跟踪模块
package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// Connection 连接信息
type Connection struct {
	ID            string    `json:"id"`
	Start         time.Time `json:"start"`
	Metadata      *ConnMeta `json:"metadata"`
	Upload        int64     `json:"upload"`
	Download      int64     `json:"download"`
	UploadSpeed   float64   `json:"uploadSpeed"`
	DownloadSpeed float64   `json:"downloadSpeed"`
	Rule          string    `json:"rule"`
	Chains        []string  `json:"chains"`
}

// ConnMeta 连接元数据
type ConnMeta struct {
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationIP   string `json:"destinationIP"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
	Network         string `json:"network"`
	Process         string `json:"process"`
}

// ConnectionTracker 连接跟踪器
type ConnectionTracker struct {
	connections sync.Map // map[string]*Connection
	counter     uint64
}

// NewConnectionTracker 创建连接跟踪器
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{}
}

// Track 开始跟踪一个连接
func (ct *ConnectionTracker) Track(id string, meta *ConnMeta) *Connection {
	conn := &Connection{
		ID:        id,
		Start:     time.Now(),
		Metadata:  meta,
		Chains:    []string{},
	}
	ct.connections.Store(id, conn)
	return conn
}

// Untrack 停止跟踪一个连接
func (ct *ConnectionTracker) Untrack(id string) {
	ct.connections.Delete(id)
}

// UpdateTraffic 更新连接流量
func (ct *ConnectionTracker) UpdateTraffic(id string, up, down int64) {
	if v, ok := ct.connections.Load(id); ok {
		conn := v.(*Connection)
		atomic.StoreInt64(&conn.Upload, up)
		atomic.StoreInt64(&conn.Download, down)
	}
}

// Get 获取指定连接
func (ct *ConnectionTracker) Get(id string) *Connection {
	if v, ok := ct.connections.Load(id); ok {
		return v.(*Connection)
	}
	return nil
}

// All 获取所有连接
func (ct *ConnectionTracker) All() []*Connection {
	conns := make([]*Connection, 0)
	ct.connections.Range(func(key, value interface{}) bool {
		conns = append(conns, value.(*Connection))
		return true
	})
	return conns
}

// Close 关闭指定连接
func (ct *ConnectionTracker) Close(id string) bool {
	if _, ok := ct.connections.Load(id); ok {
		ct.connections.Delete(id)
		return true
	}
	return false
}

// CloseAll 关闭所有连接
func (ct *ConnectionTracker) CloseAll() {
	ct.connections.Range(func(key, value interface{}) bool {
		ct.connections.Delete(key)
		return true
	})
}

// ActiveCount 活跃连接数
func (ct *ConnectionTracker) ActiveCount() int {
	count := 0
	ct.connections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// TotalUpload 总上传字节数
func (ct *ConnectionTracker) TotalUpload() int64 {
	var total int64
	ct.connections.Range(func(key, value interface{}) bool {
		conn := value.(*Connection)
		total += atomic.LoadInt64(&conn.Upload)
		return true
	})
	return total
}

// TotalDownload 总下载字节数
func (ct *ConnectionTracker) TotalDownload() int64 {
	var total int64
	ct.connections.Range(func(key, value interface{}) bool {
		conn := value.(*Connection)
		total += atomic.LoadInt64(&conn.Download)
		return true
	})
	return total
}
