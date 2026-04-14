//go:build linux

// Package ebpf 提供 Hades 的 eBPF 加速数据包转发功能。
//
// 使用 Linux TC (Traffic Control) hook 在内核层拦截网络流量：
//   - 匹配 DIRECT 规则的 IP-CIDR 流量在内核直接放行
//   - 匹配 REJECT 规则的流量在内核直接丢弃
//   - 其余流量正常传递到用户态代理
//
// 要求：Linux 内核 5.6+, CAP_NET_ADMIN 权限
// 编译：运行 `go generate ./pkg/ebpf/` 编译 BPF C 程序
package ebpf

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/rs/zerolog/log"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target bpfel,bpfeb hadesTC ./hades_tc.c

// ─── BPF Map 类型定义 ──────────────────────────────────────

// cidr4Key IPv4 CIDR LPM Trie key
type cidr4Key struct {
	Prefixlen uint32
	Addr      uint32
}

// cidr4Value IPv4 CIDR 规则值
type cidr4Value struct {
	Action uint32
}

// cidr6Key IPv6 CIDR LPM Trie key
type cidr6Key struct {
	Prefixlen uint32
	Addr      [4]uint32
}

// cidr6Value IPv6 CIDR 规则值
type cidr6Value struct {
	Action uint32
}

// statsKey 统计计数器 key
type statsKey struct {
	Action uint32
}

// statsValue 统计计数器 value
type statsValue struct {
	Packets uint64
	Bytes   uint64
}

// ─── 公共类型 ──────────────────────────────────────────────

// Action 表示 eBPF 规则的动作
type Action uint32

const (
	ActionDirect Action = 1 // 直接放行，不经过用户态
	ActionReject Action = 2 // 直接丢弃
	ActionProxy  Action = 3 // 交由用户态代理处理
)

// CIDR4Rule IPv4 CIDR 规则
type CIDR4Rule struct {
	Prefix netip.Prefix
	Action Action
}

// CIDR6Rule IPv6 CIDR 规则
type CIDR6Rule struct {
	Prefix netip.Prefix
	Action Action
}

// Stats 流量统计
type Stats struct {
	Packets uint64
	Bytes   uint64
}

// ─── Manager ───────────────────────────────────────────────

// Manager eBPF 管理器
type Manager struct {
	collection *ebpf.Collection

	// BPF Map 引用
	directCIDR4 *ebpf.Map
	directCIDR6 *ebpf.Map
	statsMap    *ebpf.Map

	ifaceName string
	ifIndex   int
	attached  bool
}

// NewManager 创建 eBPF 管理器
func NewManager() *Manager {
	return &Manager{}
}

// Load 加载 eBPF 程序
//
// 查找预编译的 BPF ELF 对象文件并加载。
// ELF 文件可由以下方式生成：
//   1. 运行 `go generate ./pkg/ebpf/` （需要 clang/llvm 工具链）
//   2. 在 CI 中预编译并嵌入二进制
//
// 加载失败时返回错误，调用方应降级到用户态转发
func (m *Manager) Load() error {
	// 移除内存锁定限制
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("移除内存锁定限制失败: %w", err)
	}

	// 查找 ELF 文件：搜索多个可能的路径
	elfPath := findBPFElf()
	if elfPath == "" {
		return fmt.Errorf("未找到预编译的 eBPF ELF 文件，请先运行 go generate ./pkg/ebpf/")
	}

	log.Info().Str("path", elfPath).Msg("[eBPF] 正在加载 ELF 文件...")

	// 从 ELF 文件加载 CollectionSpec
	spec, err := ebpf.LoadCollectionSpec(elfPath)
	if err != nil {
		return fmt.Errorf("加载 eBPF CollectionSpec 失败: %w", err)
	}

	// 创建 Collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建 eBPF Collection 失败: %w", err)
	}
	m.collection = coll

	// 获取 Map 引用
	m.directCIDR4 = coll.Maps["direct_cidr4"]
	m.directCIDR6 = coll.Maps["direct_cidr6"]
	m.statsMap = coll.Maps["stats"]

	if m.directCIDR4 == nil || m.directCIDR6 == nil || m.statsMap == nil {
		coll.Close()
		return fmt.Errorf("eBPF Map 引用获取失败")
	}

	log.Info().Msg("[eBPF] eBPF 程序加载成功")
	return nil
}

// findBPFElf 查找预编译的 BPF ELF 文件
func findBPFElf() string {
	// 搜索路径（优先级从高到低）
	candidates := []string{
		// 1. 与当前二进制同目录
		"/etc/hades/hades_tc.o",
		// 2. Go embed 或 bpf2go 生成的路径
		"hades_tc_bpfel.o",
		// 3. 相对于可执行文件的路径
		"ebpf/hades_tc_bpfel.o",
	}

	// 获取可执行文件所在目录
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append([]string{
			filepath.Join(exeDir, "hades_tc_bpfel.o"),
			filepath.Join(exeDir, "ebpf", "hades_tc_bpfel.o"),
		}, candidates...)
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// Attach 将 eBPF 程序附加到网络接口的 TC ingress
func (m *Manager) Attach(ifName string, ifIndex int) error {
	if m.collection == nil {
		return fmt.Errorf("eBPF 程序未加载")
	}

	if m.attached {
		log.Warn().Str("iface", m.ifaceName).Msg("[eBPF] 已附加，跳过")
		return nil
	}

	// 获取 TC ingress 程序
	prog := m.collection.Programs["hades_tc_ingress"]
	if prog == nil {
		return fmt.Errorf("未找到 hades_tc_ingress 程序")
	}

	// 使用 tc 命令附加 BPF 程序
	if err := attachTCProgram(ifName, prog); err != nil {
		return fmt.Errorf("附加 TC 程序失败: %w", err)
	}

	m.ifaceName = ifName
	m.ifIndex = ifIndex
	m.attached = true

	log.Info().
		Str("iface", ifName).
		Int("ifindex", ifIndex).
		Msg("[eBPF] TC ingress 程序已附加")
	return nil
}

// Detach 从网络接口分离 eBPF 程序
func (m *Manager) Detach() error {
	if !m.attached {
		return nil
	}

	if err := detachTCProgram(m.ifaceName); err != nil {
		log.Warn().Err(err).Msg("[eBPF] 分离 TC 程序失败")
	}

	m.attached = false
	log.Info().Str("iface", m.ifaceName).Msg("[eBPF] TC 程序已分离")
	return nil
}

// Close 关闭 eBPF 管理器
func (m *Manager) Close() error {
	if err := m.Detach(); err != nil {
		return err
	}
	if m.collection != nil {
		m.collection.Close()
		m.collection = nil
	}
	return nil
}

// UpdateCIDR4Rules 更新 IPv4 CIDR 规则
// 使用 LPM Trie 实现最长前缀匹配
func (m *Manager) UpdateCIDR4Rules(rules []CIDR4Rule) error {
	if m.directCIDR4 == nil {
		return fmt.Errorf("direct_cidr4 Map 未初始化")
	}

	// 清空现有规则
	clearMap(m.directCIDR4)

	for _, rule := range rules {
		key := cidr4Key{
			Prefixlen: uint32(rule.Prefix.Bits()),
			Addr:      ipToUint32(rule.Prefix.Addr()),
		}
		val := cidr4Value{
			Action: uint32(rule.Action),
		}

		if err := m.directCIDR4.Put(&key, &val); err != nil {
			log.Warn().
				Str("cidr", rule.Prefix.String()).
				Err(err).
				Msg("[eBPF] 添加 IPv4 CIDR 规则失败")
		}
	}

	log.Info().Int("count", len(rules)).Msg("[eBPF] IPv4 CIDR 规则已更新")
	return nil
}

// UpdateCIDR6Rules 更新 IPv6 CIDR 规则
func (m *Manager) UpdateCIDR6Rules(rules []CIDR6Rule) error {
	if m.directCIDR6 == nil {
		return fmt.Errorf("direct_cidr6 Map 未初始化")
	}

	clearMap(m.directCIDR6)

	for _, rule := range rules {
		key := cidr6Key{
			Prefixlen: uint32(rule.Prefix.Bits()),
		}
		addr := rule.Prefix.Addr().As16()
		for i := 0; i < 4; i++ {
			key.Addr[i] = binary.LittleEndian.Uint32(addr[i*4 : (i+1)*4])
		}

		val := cidr6Value{
			Action: uint32(rule.Action),
		}

		if err := m.directCIDR6.Put(&key, &val); err != nil {
			log.Warn().
				Str("cidr", rule.Prefix.String()).
				Err(err).
				Msg("[eBPF] 添加 IPv6 CIDR 规则失败")
		}
	}

	log.Info().Int("count", len(rules)).Msg("[eBPF] IPv6 CIDR 规则已更新")
	return nil
}

// GetStats 获取流量统计
func (m *Manager) GetStats() (map[Action]Stats, error) {
	if m.statsMap == nil {
		return nil, fmt.Errorf("stats Map 未初始化")
	}

	result := make(map[Action]Stats)

	for _, action := range []Action{ActionDirect, ActionReject, ActionProxy} {
		key := statsKey{Action: uint32(action)}
		var val statsValue
		if err := m.statsMap.Lookup(&key, &val); err == nil {
			result[action] = Stats{
				Packets: val.Packets,
				Bytes:   val.Bytes,
			}
		}
	}

	return result, nil
}

// IsAttached 检查是否已附加
func (m *Manager) IsAttached() bool {
	return m.attached
}

// InterfaceName 返回附加的网络接口名
func (m *Manager) InterfaceName() string {
	return m.ifaceName
}

// ─── 辅助函数 ──────────────────────────────────────────────

// ipToUint32 将 IPv4 地址转换为网络字节序 uint32
func ipToUint32(addr netip.Addr) uint32 {
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:])
}

// clearMap 清空 BPF Map
func clearMap(m *ebpf.Map) {
	var key []byte
	var val []byte
	iter := m.Iterate()
	for iter.Next(&key, &val) {
		_ = m.Delete(key)
	}
}
