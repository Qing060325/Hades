// Package app 应用生命周期管理
package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hades/hades/internal/config"
	"github.com/hades/hades/pkg/api"
	"github.com/hades/hades/pkg/core/adapter"
	"github.com/hades/hades/pkg/core/adapter/hysteria2"
	"github.com/hades/hades/pkg/core/adapter/shadowsocks"
	"github.com/hades/hades/pkg/core/adapter/trojan"
	"github.com/hades/hades/pkg/core/adapter/tuic"
	"github.com/hades/hades/pkg/core/adapter/vless"
	"github.com/hades/hades/pkg/core/adapter/vmess"
	"github.com/hades/hades/pkg/core/adapter/wireguard"
	"github.com/hades/hades/pkg/core/dialer"
	"github.com/hades/hades/pkg/core/dns"
	"github.com/hades/hades/pkg/core/group"
	"github.com/hades/hades/pkg/core/listener"
	"github.com/hades/hades/pkg/core/listener/tun"
	"github.com/hades/hades/pkg/core/rules"
	"github.com/hades/hades/pkg/sniffer"
	"github.com/hades/hades/pkg/stats"
	"github.com/rs/zerolog/log"
)

// App 应用程序
type App struct {
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 核心组件
	adapterManager  *adapter.Manager
	dialerManager   *dialer.Manager
	ruleEngine      *rules.Engine
	dnsClient       *dns.Client
	groupManager    *group.Manager
	listenerManager *listener.Manager
	statsManager    *stats.Manager
	apiServer       *api.Server
	sniffer         *sniffer.Sniffer
}

// New 创建应用实例
func New(cfg *config.Config) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// 初始化组件
	if err := app.initComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("初始化组件失败: %w", err)
	}

	return app, nil
}

// initComponents 初始化组件
func (a *App) initComponents() error {
	var err error

	// 1. 初始化适配器管理器
	a.adapterManager = adapter.NewManager()
	if err := a.initAdapters(); err != nil {
		return fmt.Errorf("初始化适配器失败: %w", err)
	}

	// 2. 初始化拨号器管理器
	a.dialerManager = dialer.NewManager(a.cfg)

	// 3. 初始化 DNS 客户端
	if a.cfg.DNS.Enable {
		a.dnsClient, err = dns.NewClient(&a.cfg.DNS)
		if err != nil {
			return fmt.Errorf("初始化 DNS 客户端失败: %w", err)
		}
	}

	// 4. 初始化规则引擎
	a.ruleEngine = rules.NewEngine(a.cfg.Rules)

	// 5. 初始化代理组管理器
	a.groupManager = group.NewManager(a.adapterManager)
	if err := a.initGroups(); err != nil {
		return fmt.Errorf("初始化代理组失败: %w", err)
	}

	// 6. 初始化嗅探器
	a.sniffer = sniffer.NewSniffer(&a.cfg.Sniffer)

	// 7. 初始化统计管理器
	a.statsManager = stats.NewManager()

	// 8. 初始化监听器管理器
	a.listenerManager = listener.NewManager(a.adapterManager, a.ruleEngine, a.groupManager)

	// 9. 初始化 API 服务器
	if a.cfg.ExternalController != "" {
		a.apiServer = api.NewServer(a.cfg.ExternalController, a.cfg.Secret)
		a.apiServer.SetManagers(a.adapterManager, a.groupManager, a.ruleEngine, a.statsManager)
	}

	return nil
}

// initAdapters 初始化适配器
func (a *App) initAdapters() error {
	// 内置适配器
	a.adapterManager.Add(adapter.NewDirect())
	a.adapterManager.Add(adapter.NewReject())
	a.adapterManager.Add(adapter.NewRejectDrop())

	// 从配置创建代理适配器
	for _, proxyCfg := range a.cfg.Proxies {
		adapt, err := a.createAdapter(&proxyCfg)
		if err != nil {
			log.Warn().Err(err).Str("name", proxyCfg.Name).Str("type", proxyCfg.Type).Msg("创建代理适配器失败")
			continue
		}
		a.adapterManager.Add(adapt)
	}

	return nil
}

// createAdapter 根据配置创建适配器
func (a *App) createAdapter(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	switch cfg.Type {
	case "direct":
		return adapter.NewDirect(), nil
	case "reject":
		return adapter.NewReject(), nil
	case "http":
		return adapter.NewHTTP(&adapter.HTTPOption{
			Name:     cfg.Name,
			Server:   cfg.Server,
			Port:     cfg.Port,
			Password: cfg.Password,
			TLS:      cfg.TLS,
			SNI:      cfg.SNI,
		}), nil
	case "socks5":
		return adapter.NewSOCKS5(&adapter.SOCKS5Option{
			Name:     cfg.Name,
			Server:   cfg.Server,
			Port:     cfg.Port,
			Password: cfg.Password,
			TLS:      cfg.TLS,
			SNI:      cfg.SNI,
			UDP:      cfg.UDP,
		}), nil
	case "ss", "shadowsocks":
		return a.createShadowsocks(cfg)
	case "vmess":
		return a.createVMess(cfg)
	case "vless":
		return a.createVLESS(cfg)
	case "trojan":
		return a.createTrojan(cfg)
	case "hysteria2", "hy2":
		return a.createHysteria2(cfg)
	case "tuic":
		return a.createTUIC(cfg)
	case "wireguard", "wg":
		return a.createWireGuard(cfg)
	default:
		return nil, fmt.Errorf("不支持的代理类型: %s", cfg.Type)
	}
}

func (a *App) createShadowsocks(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	cipher := cfg.Cipher
	if cipher == "" {
		cipher = "aes-256-gcm"
	}
	return shadowsocks.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cipher, cfg.Password)
}

func (a *App) createVMess(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []vmess.Option
	if cfg.TLS {
		opts = append(opts, vmess.WithTLS(cfg.SNI))
	}
	if cfg.Network == "ws" {
		opts = append(opts, vmess.WithWebSocket(cfg.WSPath, cfg.WSHeaders["Host"]))
	}
	return vmess.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.UUID, cfg.AlterID, opts...)
}

func (a *App) createVLESS(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []vless.Option
	if cfg.TLS {
		opts = append(opts, vless.WithTLS(cfg.SNI))
	}
	if cfg.Flow != "" {
		opts = append(opts, vless.WithFlow(cfg.Flow))
	}
	if cfg.Network == "ws" {
		opts = append(opts, vless.WithWebSocket(cfg.WSPath, cfg.WSHeaders["Host"]))
	}
	if cfg.Network == "grpc" {
		opts = append(opts, vless.WithGRPC(cfg.GRPCServiceName))
	}
	return vless.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.UUID, opts...)
}

func (a *App) createTrojan(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []trojan.Option
	if cfg.SNI != "" {
		opts = append(opts, trojan.WithSNI(cfg.SNI))
	}
	if cfg.SkipCertVerify {
		opts = append(opts, trojan.WithSkipCertVerify(true))
	}
	if cfg.Network == "ws" {
		opts = append(opts, trojan.WithWebSocket(cfg.WSPath, cfg.WSHeaders["Host"]))
	}
	if cfg.Network == "grpc" {
		opts = append(opts, trojan.WithGRPC(cfg.GRPCServiceName))
	}
	return trojan.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.Password, opts...)
}

func (a *App) createHysteria2(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []hysteria2.Option

	// SNI
	sni := cfg.SNI
	if sni == "" {
		sni = cfg.Server
	}
	opts = append(opts, hysteria2.WithSNI(sni))

	// 跳过证书验证
	if cfg.SkipCertVerify {
		opts = append(opts, hysteria2.WithSkipCertVerify(true))
	}

	// ALPN
	if len(cfg.ALPN) > 0 {
		opts = append(opts, hysteria2.WithALPN(cfg.ALPN))
	}

	// 带宽
	if cfg.Hysteria2Opts != nil {
		opts = append(opts, hysteria2.WithBandwidth(cfg.Hysteria2Opts.Up, cfg.Hysteria2Opts.Down))
		if cfg.Hysteria2Opts.Obfs != "" {
			opts = append(opts, hysteria2.WithObfs(cfg.Hysteria2Opts.Obfs, cfg.Hysteria2Opts.ObfsPassword))
		}
	}

	return hysteria2.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.Password, opts...)
}

func (a *App) createTUIC(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []tuic.Option

	// SNI
	sni := cfg.SNI
	if sni == "" {
		sni = cfg.Server
	}
	opts = append(opts, tuic.WithSNI(sni))

	// 跳过证书验证
	if cfg.SkipCertVerify {
		opts = append(opts, tuic.WithSkipCertVerify(true))
	}

	// ALPN
	if len(cfg.ALPN) > 0 {
		opts = append(opts, tuic.WithALPN(cfg.ALPN))
	}

	// TUIC 特有配置
	if cfg.TUICOpts != nil {
		opts = append(opts, tuic.WithCongestionController(tuic.CongestionController(cfg.TUICOpts.CongestionController)))
		opts = append(opts, tuic.WithUDPRelayMode(tuic.UDPRelayMode(cfg.TUICOpts.UDPRelayMode)))
		if cfg.TUICOpts.HeartbeatInterval > 0 {
			opts = append(opts, tuic.WithHeartbeatInterval(time.Duration(cfg.TUICOpts.HeartbeatInterval)*time.Millisecond))
		}
		opts = append(opts, tuic.WithZeroRTTHandshake(cfg.TUICOpts.ZeroRTTHandshake))
	}

	return tuic.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.UUID, cfg.Password, opts...)
}

func (a *App) createWireGuard(cfg *config.ProxyConfig) (adapter.Adapter, error) {
	var opts []wireguard.Option

	// MTU
	if cfg.MTU > 0 {
		opts = append(opts, wireguard.WithMTU(cfg.MTU))
	}

	// 预共享密钥
	if cfg.WireGuardOpts != nil {
		if len(cfg.WireGuardOpts.Peers) > 0 {
			peer := cfg.WireGuardOpts.Peers[0]
			opts = append(opts, wireguard.WithPreSharedKey(peer.PreSharedKey))
		}
		opts = append(opts, wireguard.WithAllowedIPs(cfg.WireGuardOpts.AllowedIPs))
	}

	// Reserved
	if len(cfg.Reserved) >= 3 {
		opts = append(opts, wireguard.WithReserved(cfg.Reserved))
	}

	return wireguard.NewAdapter(cfg.Name, cfg.Server, cfg.Port, cfg.PrivateKey, cfg.PublicKey, opts...)
}

// initGroups 初始化代理组
func (a *App) initGroups() error {
	for _, groupCfg := range a.cfg.ProxyGroups {
		g := a.createGroup(&groupCfg)
		if g != nil {
			a.groupManager.Add(g)
		}
	}
	return nil
}

func (a *App) createGroup(cfg *config.ProxyGroupConfig) group.Group {
	// 获取代理列表
	proxies := make([]adapter.Adapter, 0, len(cfg.Proxies))
	for _, name := range cfg.Proxies {
		if adapt := a.adapterManager.Get(name); adapt != nil {
			proxies = append(proxies, adapt)
		}
	}

	if len(proxies) == 0 {
		log.Warn().Str("name", cfg.Name).Msg("代理组无可用代理")
		return nil
	}

	interval := time.Duration(cfg.Interval) * time.Second
	timeout := time.Duration(cfg.Timeout) * time.Millisecond
	tolerance := time.Duration(cfg.Tolerance) * time.Millisecond

	switch cfg.Type {
	case "select":
		return group.NewSelectGroup(cfg.Name, proxies)
	case "url-test":
		return group.NewURLTestGroup(cfg.Name, proxies, cfg.URL, interval, tolerance, timeout)
	case "fallback":
		return group.NewFallbackGroup(cfg.Name, proxies, cfg.URL, interval, timeout)
	case "load-balance":
		return group.NewLoadBalanceGroup(cfg.Name, proxies, group.BalanceRoundRobin)
	default:
		return group.NewSelectGroup(cfg.Name, proxies)
	}
}

// Start 启动应用
func (a *App) Start() error {
	log.Info().Msg("正在启动代理内核...")

	// 启动 DNS 服务
	if a.cfg.DNS.Enable && a.cfg.DNS.Listen != "" {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.dnsClient.Listen(a.ctx, a.cfg.DNS.Listen); err != nil {
				log.Error().Err(err).Msg("DNS 服务异常")
			}
		}()
		log.Info().Str("addr", a.cfg.DNS.Listen).Msg("DNS 服务已启动")
	}

	// 启动监听器
	if err := a.startListeners(); err != nil {
		return fmt.Errorf("启动监听器失败: %w", err)
	}

	// 启动 API 服务器
	if a.apiServer != nil {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.apiServer.ListenAndServe(); err != nil {
				log.Error().Err(err).Msg("API 服务异常")
			}
		}()
		log.Info().Str("addr", a.cfg.ExternalController).Msg("API 服务已启动")
	}

	// 启动代理组健康检查
	a.startHealthChecks()

	log.Info().Msg("代理内核启动完成")
	return nil
}

// startListeners 启动监听器
func (a *App) startListeners() error {
	if a.cfg.MixedPort > 0 {
		addr := fmt.Sprintf("%s:%d", a.cfg.BindAddress, a.cfg.MixedPort)
		if err := a.listenerManager.StartMixedListener(a.ctx, addr, a.cfg.AllowLan); err != nil {
			return fmt.Errorf("启动混合监听器失败: %w", err)
		}
		log.Info().Str("addr", addr).Msg("混合端口监听器已启动")
	}

	if a.cfg.Port > 0 {
		addr := fmt.Sprintf("%s:%d", a.cfg.BindAddress, a.cfg.Port)
		if err := a.listenerManager.StartHTTPListener(a.ctx, addr, a.cfg.AllowLan); err != nil {
			return fmt.Errorf("启动 HTTP 监听器失败: %w", err)
		}
		log.Info().Str("addr", addr).Msg("HTTP 监听器已启动")
	}

	if a.cfg.SocksPort > 0 {
		addr := fmt.Sprintf("%s:%d", a.cfg.BindAddress, a.cfg.SocksPort)
		if err := a.listenerManager.StartSOCKSListener(a.ctx, addr, a.cfg.AllowLan); err != nil {
			return fmt.Errorf("启动 SOCKS 监听器失败: %w", err)
		}
		log.Info().Str("addr", addr).Msg("SOCKS 监听器已启动")
	}

	if a.cfg.Tun.Enable {
		if err := a.startTUN(); err != nil {
			return fmt.Errorf("启动 TUN 模式失败: %w", err)
		}
	}

	return nil
}

// startTUN 启动 TUN 模式
func (a *App) startTUN() error {
	log.Info().Str("stack", a.cfg.Tun.Stack).Msg("正在启动 TUN 模式...")

	tunListener, err := tun.NewTUNListener(&a.cfg.Tun, a.adapterManager, a.ruleEngine, a.groupManager)
	if err != nil {
		return err
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := tunListener.Listen(a.ctx); err != nil {
			log.Error().Err(err).Msg("TUN 监听器异常")
		}
	}()

	log.Info().Msg("TUN 模式已启动")
	return nil
}

// startHealthChecks 启动健康检查
func (a *App) startHealthChecks() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				a.runHealthChecks()
			}
		}
	}()
}

func (a *App) runHealthChecks() {
	groups := a.groupManager.All()
	for _, g := range groups {
		if err := g.HealthCheck(); err != nil {
			log.Debug().Err(err).Str("group", g.Name()).Msg("健康检查失败")
		}
	}
}

// Stop 停止应用
func (a *App) Stop() error {
	log.Info().Msg("正在关闭代理内核...")

	a.cancel()

	if a.apiServer != nil {
		a.apiServer.Shutdown(a.ctx)
	}

	a.wg.Wait()

	if a.listenerManager != nil {
		a.listenerManager.Close()
	}

	log.Info().Msg("代理内核已关闭")
	return nil
}

// Stats 获取统计信息
func (a *App) Stats() *stats.Manager {
	return a.statsManager
}
