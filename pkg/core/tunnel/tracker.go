package tunnel

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TrackedConn 带跟踪的连接
type TrackedConn struct {
	net.Conn
	tracker   *Tracker
	id        uint64
	startTime time.Time
	closed    int32
}

// Close 关闭连接并更新跟踪
func (c *TrackedConn) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.tracker.remove(c.id)
	return c.Conn.Close()
}

// Duration 连接持续时间
func (c *TrackedConn) Duration() time.Duration {
	return time.Since(c.startTime)
}

// Tracker 连接跟踪器
type Tracker struct {
	conns    map[uint64]*TrackedConn
	mu       sync.RWMutex
	nextID   uint64
	totalIn  int64
	totalOut int64
	startAt  time.Time
}

// NewTracker 创建连接跟踪器
func NewTracker() *Tracker {
	return &Tracker{
		conns:   make(map[uint64]*TrackedConn),
		startAt: time.Now(),
	}
}

// Track 包装连接进行跟踪
func (t *Tracker) Track(conn net.Conn) *TrackedConn {
	id := atomic.AddUint64(&t.nextID, 1)
	tracked := &TrackedConn{
		Conn:      conn,
		tracker:   t,
		id:        id,
		startTime: time.Now(),
	}

	t.mu.Lock()
	t.conns[id] = tracked
	t.mu.Unlock()

	atomic.AddInt64(&t.totalIn, 1)
	return tracked
}

// remove 移除连接
func (t *Tracker) remove(id uint64) {
	t.mu.Lock()
	delete(t.conns, id)
	t.mu.Unlock()

	atomic.AddInt64(&t.totalOut, 1)
}

// ActiveCount 活跃连接数
func (t *Tracker) ActiveCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.conns)
}

// Stats 统计信息
type Stats struct {
	ActiveConns int64     `json:"active_conns"`
	TotalIn     int64     `json:"total_in"`
	TotalOut    int64     `json:"total_out"`
	Uptime      float64   `json:"uptime_seconds"`
	StartedAt   time.Time `json:"started_at"`
}

// Stats 返回统计
func (t *Tracker) Stats() Stats {
	return Stats{
		ActiveConns: int64(t.ActiveCount()),
		TotalIn:     atomic.LoadInt64(&t.totalIn),
		TotalOut:    atomic.LoadInt64(&t.totalOut),
		Uptime:      time.Since(t.startAt).Seconds(),
		StartedAt:   t.startAt,
	}
}

// Close 关闭所有跟踪的连接
func (t *Tracker) Close() {
	t.mu.Lock()
	conns := make([]*TrackedConn, 0, len(t.conns))
	for _, c := range t.conns {
		conns = append(conns, c)
	}
	t.conns = make(map[uint64]*TrackedConn)
	t.mu.Unlock()

	for _, c := range conns {
		c.Conn.Close()
	}
}
