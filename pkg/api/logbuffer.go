package api

import (
	"sync"
	"time"
)

// LogEntry 日志条目
type LogEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Time    string `json:"time"`
}

// LogBuffer 日志缓冲区，支持 SSE 订阅
type LogBuffer struct {
	entries []LogEntry
	max     int
	mu      sync.RWMutex
	subs    map[string]chan LogEntry
}

// NewLogBuffer 创建日志缓冲区
func NewLogBuffer(max int) *LogBuffer {
	if max <= 0 {
		max = 500
	}
	return &LogBuffer{
		entries: make([]LogEntry, 0, max),
		max:     max,
		subs:    make(map[string]chan LogEntry),
	}
}

// Add 添加一条日志
func (lb *LogBuffer) Add(entry LogEntry) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// 设置时间戳
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}

	// 追加到缓冲区
	lb.entries = append(lb.entries, entry)

	// 超过最大容量时裁剪
	if len(lb.entries) > lb.max {
		lb.entries = lb.entries[len(lb.entries)-lb.max:]
	}

	// 通知所有订阅者
	for _, ch := range lb.subs {
		select {
		case ch <- entry:
		default:
			// 跳过满的通道，避免阻塞
		}
	}
}

// Subscribe 订阅日志流，返回一个只读通道和取消函数
func (lb *LogBuffer) Subscribe(id string, bufSize int) (<-chan LogEntry, func()) {
	if bufSize <= 0 {
		bufSize = 100
	}

	ch := make(chan LogEntry, bufSize)

	lb.mu.Lock()
	lb.subs[id] = ch
	lb.mu.Unlock()

	unsubscribe := func() {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		if _, ok := lb.subs[id]; ok {
			delete(lb.subs, id)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Unsubscribe 取消订阅
func (lb *LogBuffer) Unsubscribe(id string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if ch, ok := lb.subs[id]; ok {
		delete(lb.subs, id)
		close(ch)
	}
}

// Recent 获取最近 n 条日志
func (lb *LogBuffer) Recent(n int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if n <= 0 || n > len(lb.entries) {
		n = len(lb.entries)
	}

	start := len(lb.entries) - n
	result := make([]LogEntry, n)
	copy(result, lb.entries[start:])
	return result
}
