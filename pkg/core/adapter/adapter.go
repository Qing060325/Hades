// Package adapter 代理适配器核心模块
package adapter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// AdapterType 适配器类型
type AdapterType string

const (
	TypeDirect       AdapterType = "Direct"
	TypeReject       AdapterType = "Reject"
	TypeHTTP         AdapterType = "HTTP"
	TypeSOCKS5       AdapterType = "SOCKS5"
	TypeShadowsocks  AdapterType = "Shadowsocks"
	TypeShadowsocksR AdapterType = "ShadowsocksR"
	TypeVMess        AdapterType = "VMess"
	TypeVLESS        AdapterType = "VLESS"
	TypeTrojan       AdapterType = "Trojan"
	TypeHysteria     AdapterType = "Hysteria"
	TypeHysteria2    AdapterType = "Hysteria2"
	TypeTUIC         AdapterType = "TUIC"
	TypeWireGuard    AdapterType = "WireGuard"
	TypeSnell        AdapterType = "Snell"
	TypeSSH          AdapterType = "SSH"
	TypeMieru        AdapterType = "Mieru"
	TypeRelay        AdapterType = "Relay"
	TypeAmneziaWG    AdapterType = "AmneziaWG"
	TypeAnyTLS       AdapterType = "AnyTLS"
	TypeMASQUE       AdapterType = "MASQUE"
	TypeTrustTunnel  AdapterType = "TrustTunnel"
	TypeSudoku       AdapterType = "Sudoku"
)

// MetadataType 元数据类型
type MetadataType string

const (
	MetadataTypeHTTP   MetadataType = "HTTP"
	MetadataTypeSOCKS  MetadataType = "SOCKS"
	MetadataTypeTUN    MetadataType = "TUN"
	MetadataTypeRedir  MetadataType = "Redir"
	MetadataTypeTProxy MetadataType = "TProxy"
)

// DNSMode DNS 模式
type DNSMode string

const (
	DNSModeNormal  DNSMode = "normal"
	DNSModeFakeIP  DNSMode = "fakeip"
	DNSModeMapping DNSMode = "mapping"
)

// Adapter 代理适配器统一接口
type Adapter interface {
	// 基础信息
	Name() string
	Type() AdapterType
	Addr() string

	// 连接处理
	DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error)
	DialUDPContext(ctx context.Context, metadata *Metadata) (net.PacketConn, error)

	// 状态
	SupportUDP() bool
	SupportWithDialer() bool

	// 健康检查
	URLTest(ctx context.Context, url string) (time.Duration, error)
}

// Metadata 连接元数据
type Metadata struct {
	Type         MetadataType // 入站类型
	SrcIP        netip.Addr   // 源IP
	DstIP        netip.Addr   // 目标IP
	SrcPort      uint16       // 源端口
	DstPort      uint16       // 目标端口
	Host         string       // 目标主机名
	NetWork      string       // 网络类型 (tcp/udp)
	DNSMode      DNSMode      // DNS 模式
	ProcessName  string       // 进程名
	ProcessPath  string       // 进程路径
	InName       string       // 入站名称
	SpecialProxy string       // 特殊代理
	DSCP         int          // DSCP 值
	ASN          uint         // ASN
	SourceIP     string       // 源IP字符串
	DestIP       string       // 目标IP字符串
	SourcePort   string       // 源端口字符串
	DestPort     string       // 目标端口字符串
	Addr         string       // 完整地址
}

// SourceAddress 源地址
func (m *Metadata) SourceAddress() string {
	return fmt.Sprintf("%s:%d", m.SrcIP.String(), m.SrcPort)
}

// DestinationAddress 目标地址
func (m *Metadata) DestinationAddress() string {
	if m.Host != "" {
		return fmt.Sprintf("%s:%d", m.Host, m.DstPort)
	}
	return fmt.Sprintf("%s:%d", m.DstIP.String(), m.DstPort)
}

// RemoteAddress 远程地址 (优先域名)
func (m *Metadata) RemoteAddress() string {
	if m.Host != "" {
		return m.Host
	}
	return m.DstIP.String()
}

// SourceIPString 源IP字符串
func (m *Metadata) SourceIPString() string {
	if !m.SrcIP.IsValid() {
		return ""
	}
	return m.SrcIP.String()
}

// DestinationIPString 目标IP字符串
func (m *Metadata) DestinationIPString() string {
	if !m.DstIP.IsValid() {
		return ""
	}
	return m.DstIP.String()
}

// SetRemoteAddress 设置远程地址
func (m *Metadata) SetRemoteAddress(host string, port uint16) {
	m.Host = host
	m.DstPort = port
}

// Clone 克隆元数据
func (m *Metadata) Clone() *Metadata {
	return &Metadata{
		Type:         m.Type,
		SrcIP:        m.SrcIP,
		DstIP:        m.DstIP,
		SrcPort:      m.SrcPort,
		DstPort:      m.DstPort,
		Host:         m.Host,
		DNSMode:      m.DNSMode,
		ProcessName:  m.ProcessName,
		ProcessPath:  m.ProcessPath,
		NetWork:      m.NetWork,
		InName:       m.InName,
		SpecialProxy: m.SpecialProxy,
	}
}

// Manager 适配器管理器
type Manager struct {
	adapters map[string]Adapter
	byType   map[AdapterType][]Adapter
}

// NewManager 创建适配器管理器
func NewManager() *Manager {
	return &Manager{
		adapters: make(map[string]Adapter),
		byType:   make(map[AdapterType][]Adapter),
	}
}

// Add 添加适配器
func (m *Manager) Add(adapter Adapter) {
	m.adapters[adapter.Name()] = adapter
	m.byType[adapter.Type()] = append(m.byType[adapter.Type()], adapter)
}

// Get 获取适配器
func (m *Manager) Get(name string) Adapter {
	return m.adapters[name]
}

// GetByType 按类型获取适配器
func (m *Manager) GetByType(typ AdapterType) []Adapter {
	return m.byType[typ]
}

// All 获取所有适配器
func (m *Manager) All() map[string]Adapter {
	return m.adapters
}

// Names 获取所有适配器名称
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.adapters))
	for name := range m.adapters {
		names = append(names, name)
	}
	return names
}

// Remove 移除适配器
func (m *Manager) Remove(name string) {
	if adapt, ok := m.adapters[name]; ok {
		delete(m.adapters, name)
		// 从类型索引中移除
		typedList := m.byType[adapt.Type()]
		for i, a := range typedList {
			if a.Name() == name {
				m.byType[adapt.Type()] = append(typedList[:i], typedList[i+1:]...)
				break
			}
		}
	}
}

// ParseProxyConfig 解析代理配置
func ParseProxyConfig(cfg interface{}) (Adapter, error) {
	// 这里后续实现各种协议的解析
	return nil, errors.New("未实现")
}

// BaseAdapter 基础适配器实现
type BaseAdapter struct {
	name       string
	typ        AdapterType
	addr       string
	supportUDP bool
}

// Name 返回名称
func (a *BaseAdapter) Name() string {
	return a.name
}

// Type 返回类型
func (a *BaseAdapter) Type() AdapterType {
	return a.typ
}

// Addr 返回地址
func (a *BaseAdapter) Addr() string {
	return a.addr
}

// SupportUDP 是否支持UDP
func (a *BaseAdapter) SupportUDP() bool {
	return a.supportUDP
}

// SupportWithDialer 是否支持自定义拨号器
func (a *BaseAdapter) SupportWithDialer() bool {
	return false
}

// URLTest 健康检查
func (a *BaseAdapter) URLTest(ctx context.Context, url string) (time.Duration, error) {
	return 0, errors.New("不支持健康检查")
}
