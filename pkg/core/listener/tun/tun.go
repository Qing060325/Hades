// Package tun TUN 设备抽象层（跨平台）
package tun

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sync"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/rs/zerolog/log"
)

// Device TUN 设备接口
type Device interface {
	Name() string
	Close() error
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	MTU() int
}

// Stack 网络栈接口
type Stack interface {
	Start(ctx context.Context, device Device) error
	Stop() error
}

// TunConfig TUN 配置
type TunConfig struct {
	Enable             bool
	Stack              string // system / gvisor / mixed
	AutoRoute          bool
	StrictRoute        bool
	MTU                int
	DNSHijack          []string
	AutoDetectInterface bool
}

// TunListener TUN 监听器
type TunListener struct {
	cfg        *config.TunConfig
	device     Device
	stack      Stack

	adapterMgr *adapter.Manager
	ruleEngine *rules.Engine
	groupMgr   *group.Manager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
}

// NewTUNListener 创建 TUN 监听器
func NewTUNListener(
	cfg *config.TunConfig,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) (*TunListener, error) {
	return &TunListener{
		cfg:        cfg,
		adapterMgr: adapterMgr,
		ruleEngine: ruleEngine,
		groupMgr:   groupMgr,
	}, nil
}

// Listen 启动 TUN 监听
func (l *TunListener) Listen(ctx context.Context) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}

	l.ctx, l.cancel = context.WithCancel(ctx)
	l.mu.Unlock()

	log.Info().Str("stack", l.cfg.Stack).Msg("[TUN] 正在创建 TUN 设备...")

	// 创建 TUN 设备
	device, err := createDevice(l.cfg)
	if err != nil {
		return fmt.Errorf("创建 TUN 设备失败: %w", err)
	}
	l.device = device

	log.Info().Str("name", device.Name()).Str("mtu", fmt.Sprintf("%d", device.MTU())).Msg("[TUN] TUN 设备已创建")

	// 创建网络栈
	l.stack, err = createStack(l.cfg.Stack, device, l)
	if err != nil {
		device.Close()
		return fmt.Errorf("创建网络栈失败: %w", err)
	}

	log.Info().Msg("[TUN] 网络栈已创建")

	// 启动网络栈
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		if err := l.stack.Start(l.ctx, device); err != nil {
			log.Error().Err(err).Msg("[TUN] 网络栈异常")
		}
	}()

	// 自动路由
	if l.cfg.AutoRoute {
		go l.setupAutoRoute()
	}

	log.Info().Msg("[TUN] TUN 模式已启动")
	<-l.ctx.Done()

	// 关闭
	l.stack.Stop()
	device.Close()

	log.Info().Msg("[TUN] TUN 模式已关闭")
	return nil
}

// Close 关闭 TUN 监听器
func (l *TunListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	if l.cancel != nil {
		l.cancel()
	}

	l.wg.Wait()
	return nil
}

// setupAutoRoute 设置自动路由
func (l *TunListener) setupAutoRoute() {
	// TODO: 实现自动路由设置
	// Linux: 使用 ip route 命令
	// macOS: 使用 route 命令
	// Windows: 使用 netsh 命令
}

// HandlePacket 处理数据包
func (l *TunListener) HandlePacket(data []byte) {
	// 解析 IP 数据包
	metadata, _, err := parsePacket(data)
	if err != nil {
		return
	}

	// DNS 劫持检查
	if isDNSHijack(metadata, l.cfg.DNSHijack) {
		// TODO: 处理 DNS 劫持
		return
	}

	// 选择适配器
	adapt := l.selectAdapter(metadata)
	if adapt == nil {
		return
	}

	log.Debug().
		Str("source", metadata.SourceAddress()).
		Str("target", metadata.DestinationAddress()).
		Str("adapter", adapt.Name()).
		Msg("[TUN] 处理数据包")

	// 转发数据包
	// TODO: 实现完整的数据包转发
}

// selectAdapter 选择适配器
func (l *TunListener) selectAdapter(metadata *adapter.Metadata) adapter.Adapter {
	if l.ruleEngine != nil {
		if name := l.ruleEngine.Match(metadata); name != "" {
			if adapt := l.adapterMgr.Get(name); adapt != nil {
				return adapt
			}
		}
	}

	if l.groupMgr != nil {
		if g := l.groupMgr.Get("proxy"); g != nil {
			return g.Select(metadata)
		}
	}

	return l.adapterMgr.Get("DIRECT")
}

// createDevice 创建 TUN 设备（跨平台）
func createDevice(cfg *config.TunConfig) (Device, error) {
	switch runtime.GOOS {
	case "linux":
		return createLinuxDevice(cfg)
	case "darwin":
		return createDarwinDevice(cfg)
	case "windows":
		return createWindowsDevice(cfg)
	default:
		return nil, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// createStack 创建网络栈
func createStack(stackType string, device Device, listener *TunListener) (Stack, error) {
	switch stackType {
	case "gvisor":
		return newGVisorStack(device, listener)
	case "system":
		return newSystemStack(device, listener)
	case "mixed":
		return newMixedStack(device, listener)
	default:
		return newGVisorStack(device, listener)
	}
}

// isDNSHijack 检查是否需要 DNS 劫持
func isDNSHijack(metadata *adapter.Metadata, hijacks []string) bool {
	if metadata.DstPort != 53 {
		return false
	}
	for _, hijack := range hijacks {
		if hijack == "any:53" || hijack == fmt.Sprintf("%s:53", metadata.DstIP.String()) {
			return true
		}
	}
	return false
}

// parsePacket 解析 IP 数据包
func parsePacket(data []byte) (*adapter.Metadata, []byte, error) {
	if len(data) < 20 {
		return nil, nil, fmt.Errorf("数据包太短")
	}

	metadata := &adapter.Metadata{
		Network: "tcp",
		Type:    adapter.MetadataTypeTUN,
	}

	// 判断 IPv4/IPv6
	if data[0]>>4 == 4 {
		// IPv4
		metadata.SrcIP, _ = netip.ParseAddr(net.IP(data[12:16]).String())
		metadata.DstIP, _ = netip.ParseAddr(net.IP(data[16:20]).String())
		metadata.SrcPort = uint16(data[20])<<8 | uint16(data[21])
		metadata.DstPort = uint16(data[22])<<8 | uint16(data[23])
		metadata.Network = parseIPProtocol(data[9])
		return metadata, data[20:], nil
	} else if data[0]>>4 == 6 {
		// IPv6
		metadata.SrcIP, _ = netip.ParseAddr(net.IP(data[8:24]).String())
		metadata.DstIP, _ = netip.ParseAddr(net.IP(data[24:40]).String())
		metadata.SrcPort = uint16(data[40])<<8 | uint16(data[41])
		metadata.DstPort = uint16(data[42])<<8 | uint16(data[43])
		metadata.Network = parseIPProtocol(data[6])
		return metadata, data[40:], nil
	}

	return nil, nil, fmt.Errorf("未知的 IP 版本")
}

func parseIPProtocol(proto byte) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 1:
		return "icmp"
	default:
		return "unknown"
	}
}

// createLinuxDevice Linux TUN 设备创建
func createLinuxDevice(cfg *config.TunConfig) (Device, error) {
	return nil, fmt.Errorf("Linux TUN 设备尚未实现")
}

// createDarwinDevice Darwin TUN 设备创建
func createDarwinDevice(cfg *config.TunConfig) (Device, error) {
	return nil, fmt.Errorf("Darwin TUN 设备尚未实现")
}

// createWindowsDevice Windows TUN 设备创建
func createWindowsDevice(cfg *config.TunConfig) (Device, error) {
	return nil, fmt.Errorf("Windows TUN 设备尚未实现")
}
