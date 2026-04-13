// Package stats 流量统计模块
package stats

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Manager 统计管理器
type Manager struct {
	mu      sync.RWMutex
	traffic *Traffic

	// 连接统计
	activeConnections int64
	totalConnections  int64
}

// Traffic 流量统计
type Traffic struct {
	Upload   int64 `json:"up"`
	Download int64 `json:"down"`
}

// ConnectionStat 连接统计
type ConnectionStat struct {
	ID        string    `json:"id"`
	Start     time.Time `json:"start"`
	Host      string    `json:"host"`
	Download  int64     `json:"download"`
	Upload    int64     `json:"upload"`
	Chains    []string  `json:"chains"`
	Rule      string    `json:"rule"`
	Process   string    `json:"process"`
	UploadSpeed   float64 `json:"uploadSpeed"`
	DownloadSpeed float64 `json:"downloadSpeed"`
}

// NewManager 创建统计管理器
func NewManager() *Manager {
	return &Manager{
		traffic: &Traffic{},
	}
}

// AddUpload 增加上传流量
func (m *Manager) AddUpload(n int) {
	atomic.AddInt64(&m.traffic.Upload, int64(n))
}

// AddDownload 增加下载流量
func (m *Manager) AddDownload(n int) {
	atomic.AddInt64(&m.traffic.Download, int64(n))
}

// GetTraffic 获取流量统计
func (m *Manager) GetTraffic() *Traffic {
	return &Traffic{
		Upload:   atomic.LoadInt64(&m.traffic.Upload),
		Download: atomic.LoadInt64(&m.traffic.Download),
	}
}

// AddConnection 增加连接
func (m *Manager) AddConnection() {
	atomic.AddInt64(&m.totalConnections, 1)
	atomic.AddInt64(&m.activeConnections, 1)
}

// RemoveConnection 减少连接
func (m *Manager) RemoveConnection() {
	atomic.AddInt64(&m.activeConnections, -1)
}

// ActiveConnections 活跃连接数
func (m *Manager) ActiveConnections() int64 {
	return atomic.LoadInt64(&m.activeConnections)
}

// TotalConnections 总连接数
func (m *Manager) TotalConnections() int64 {
	return atomic.LoadInt64(&m.totalConnections)
}

// Reset 重置统计
func (m *Manager) Reset() {
	atomic.StoreInt64(&m.traffic.Upload, 0)
	atomic.StoreInt64(&m.traffic.Download, 0)
	atomic.StoreInt64(&m.totalConnections, 0)
}

// SpeedCounter 速度计算器
type SpeedCounter struct {
	lastBytes    int64
	lastTime     time.Time
	currentSpeed float64
	mu           sync.RWMutex
}

// NewSpeedCounter 创建速度计算器
func NewSpeedCounter() *SpeedCounter {
	return &SpeedCounter{
		lastTime: time.Now(),
	}
}

// Update 更新字节数并计算速度
func (s *SpeedCounter) Update(bytes int64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastTime).Seconds()

	if elapsed > 0.5 { // 每 0.5 秒更新一次
		s.currentSpeed = float64(bytes-s.lastBytes) / elapsed
		s.lastBytes = bytes
		s.lastTime = now
	}

	return s.currentSpeed
}

// Speed 获取当前速度
func (s *SpeedCounter) Speed() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentSpeed
}

// FormatSpeed 格式化速度
func FormatSpeed(bytesPerSec float64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/GB)
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/MB)
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/KB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

// FormatBytes 格式化字节数
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
