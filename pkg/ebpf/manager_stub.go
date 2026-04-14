//go:build !linux

// Package ebpf 提供 Hades 的 eBPF 加速数据包转发功能。
// 非 Linux 平台的 stub 实现。
package ebpf

import (
	"fmt"
	"runtime"

	"github.com/rs/zerolog/log"
)

// Action 表示 eBPF 规则的动作
type Action uint32

const (
	ActionDirect Action = 1
	ActionReject Action = 2
	ActionProxy  Action = 3
)

// CIDR4Rule IPv4 CIDR 规则
type CIDR4Rule struct {
	Prefix interface{}
	Action Action
}

// CIDR6Rule IPv6 CIDR 规则
type CIDR6Rule struct {
	Prefix interface{}
	Action Action
}

// Stats 流量统计
type Stats struct {
	Packets uint64
	Bytes   uint64
}

// Manager eBPF 管理器 (stub)
type Manager struct{}

// NewManager 创建 eBPF 管理器
func NewManager() *Manager {
	return &Manager{}
}

// Load 加载 eBPF 程序
func (m *Manager) Load() error {
	log.Warn().Str("os", runtime.GOOS).Msg("[eBPF] eBPF 仅支持 Linux，当前平台不可用")
	return fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}

// Attach 将 eBPF 程序附加到网络接口
func (m *Manager) Attach(ifName string, ifIndex int) error {
	return fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}

// Detach 从网络接口分离 eBPF 程序
func (m *Manager) Detach() error { return nil }

// Close 关闭 eBPF 管理器
func (m *Manager) Close() error { return nil }

// UpdateCIDR4Rules 更新 IPv4 CIDR 规则
func (m *Manager) UpdateCIDR4Rules(rules []CIDR4Rule) error {
	return fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}

// UpdateCIDR6Rules 更新 IPv6 CIDR 规则
func (m *Manager) UpdateCIDR6Rules(rules []CIDR6Rule) error {
	return fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}

// GetStats 获取流量统计
func (m *Manager) GetStats() (map[Action]Stats, error) {
	return nil, fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}

// IsAttached 检查是否已附加
func (m *Manager) IsAttached() bool { return false }

// InterfaceName 返回附加的网络接口名
func (m *Manager) InterfaceName() string { return "" }
