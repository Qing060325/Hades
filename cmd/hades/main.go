// Package main 程序入口
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Qing060325/Hades/internal/app"
	"github.com/Qing060325/Hades/internal/config"
	hadesSvc "github.com/Qing060325/Hades/internal/service"
	"github.com/Qing060325/Hades/internal/version"
	"github.com/Qing060325/Hades/pkg/subscription"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	configPath  = flag.String("c", "config.yaml", "配置文件路径")
	showVersion = flag.Bool("v", false, "显示版本信息")
	debugMode   = flag.Bool("d", false, "调试模式")
)

// 服务配置常量
const (
	ServiceName        = "Hades"
	ServiceDisplayName = "Hades Proxy Kernel"
	ServiceDescription = "Hades 高性能代理内核服务"
)

func main() {
	flag.Parse()

	// 显示版本
	if *showVersion {
		fmt.Println(version.String())
		os.Exit(0)
	}

	// 初始化日志
	initLogger(*debugMode)

	// 打印启动信息
	log.Info().Str("version", version.Version).Msg("Hades 代理内核")
	log.Info().Str("os", runtime.GOOS).Str("arch", runtime.GOARCH).Msg("运行平台")

	// 检查是否有服务子命令
	if len(os.Args) > 1 {
		action := os.Args[1]
		switch action {
		case "install":
			handleServiceCommand("install")
			return
		case "uninstall":
			handleServiceCommand("uninstall")
			return
		case "start":
			handleServiceCommand("start")
			return
		case "stop":
			handleServiceCommand("stop")
			return
		case "restart":
			handleServiceCommand("restart")
			return
		case "status":
			handleServiceCommand("status")
			return
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}

	// 检查是否由服务管理器启动（自动检测服务模式）
	if hadesSvc.Interactive() {
		// 交互模式：直接运行
		runInteractive()
	} else {
		// 服务模式：通过 kardianos/service 运行
		runService()
	}
}

// runInteractive 交互模式运行
func runInteractive() {
	log.Info().Msg("以交互模式启动")

	// 加载配置
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", *configPath).Msg("加载配置失败")
	}

	// 创建应用
	application, err := app.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("创建应用失败")
	}

	// 启动应用
	if err := application.Start(); err != nil {
		log.Fatal().Err(err).Msg("启动应用失败")
	}

	// 等待信号
	waitForShutdown(application)
}

// runService 服务模式运行
func runService() {
	log.Info().Msg("以服务模式启动")

	// 使用 service 包运行
	hadesSvc.Run(*configPath, ServiceName, ServiceDisplayName, ServiceDescription)
}

// handleServiceCommand 处理服务命令
func handleServiceCommand(action string) {
	// 初始化日志（用于输出错误信息）
	initLogger(false)

	var err error

	switch action {
	case "install":
		log.Info().Msg("正在安装服务...")
		err = hadesSvc.InstallService(*configPath, ServiceName, ServiceDisplayName, ServiceDescription)
		if err != nil {
			log.Fatal().Err(err).Msg("安装服务失败")
		}
		fmt.Printf("✅ 服务 %s 安装成功\n", ServiceName)
		fmt.Println("使用以下命令管理服务:")
		fmt.Printf("  启动: %s start\n", os.Args[0])
		fmt.Printf("  停止: %s stop\n", os.Args[0])
		fmt.Printf("  重启: %s restart\n", os.Args[0])
		fmt.Printf("  卸载: %s uninstall\n", os.Args[0])

	case "uninstall":
		log.Info().Msg("正在卸载服务...")
		err = hadesSvc.UninstallService(ServiceName)
		if err != nil {
			log.Fatal().Err(err).Msg("卸载服务失败")
		}
		fmt.Printf("✅ 服务 %s 卸载成功\n", ServiceName)

	case "start":
		log.Info().Msg("正在启动服务...")
		err = hadesSvc.ControlService(ServiceName, "start")
		if err != nil {
			log.Fatal().Err(err).Msg("启动服务失败")
		}
		fmt.Printf("✅ 服务 %s 启动成功\n", ServiceName)

	case "stop":
		log.Info().Msg("正在停止服务...")
		err = hadesSvc.ControlService(ServiceName, "stop")
		if err != nil {
			log.Fatal().Err(err).Msg("停止服务失败")
		}
		fmt.Printf("✅ 服务 %s 停止成功\n", ServiceName)

	case "restart":
		log.Info().Msg("正在重启服务...")
		err = hadesSvc.ControlService(ServiceName, "restart")
		if err != nil {
			log.Fatal().Err(err).Msg("重启服务失败")
		}
		fmt.Printf("✅ 服务 %s 重启成功\n", ServiceName)

	case "status":
		status, err := hadesSvc.GetServiceStatus(ServiceName)
		if err != nil {
			fmt.Printf("❌ 获取服务状态失败: %v\n", err)
			os.Exit(1)
		}
		switch status {
		case hadesSvc.StatusRunning:
			fmt.Printf("✅ 服务 %s 正在运行\n", ServiceName)
		case hadesSvc.StatusStopped:
			fmt.Printf("⏸️  服务 %s 已停止\n", ServiceName)
		case hadesSvc.StatusUnknown:
			fmt.Printf("❓ 服务 %s 状态未知\n", ServiceName)
		default:
			fmt.Printf("❓ 服务 %s 状态: %v\n", ServiceName, status)
		}
	}
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Println(`
Hades 代理内核 - 使用说明

用法:
  hades [命令] [选项]

命令:
  (无命令)           以交互模式启动
  install            安装为系统服务
  uninstall          卸载系统服务
  start              启动系统服务
  stop               停止系统服务
  restart            重启系统服务
  status             查看服务状态
  help, -h, --help   显示此帮助信息

选项:
  -c <path>          指定配置文件路径 (默认: config.yaml)
  -v                 显示版本信息
  -d                 启用调试模式

示例:
  hades                              # 直接启动
  hades -c /path/to/config.yaml     # 使用指定配置
  hades install                      # 安装为服务
  hades start                        # 启动服务
  hades status                        # 查看状态
`)
}

// initLogger 初始化日志
func initLogger(debug bool) {
	// 设置日志级别
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	// 设置时间格式
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"

	// 设置控制台输出
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
	})
}

// loadConfig 加载配置
func loadConfig(path string) (*config.Config, error) {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Warn().Str("path", path).Msg("配置文件不存在，使用默认配置")
		return config.Default(), nil
	}

	cfg, err := config.ParseFile(path)
	if err != nil {
		return nil, err
	}

	// 处理订阅配置
	if err := processSubscriptions(cfg); err != nil {
		log.Error().Err(err).Msg("处理订阅失败")
		// 订阅失败不影响主程序启动
	}

	return cfg, nil
}

// processSubscriptions 处理订阅配置
func processSubscriptions(cfg *config.Config) error {
	if len(cfg.Subscriptions) == 0 {
		return nil
	}

	log.Info().Int("count", len(cfg.Subscriptions)).Msg("发现订阅配置")

	// 创建临时订阅管理器
	mgr := subscription.NewManager()

	// 添加订阅
	for _, subCfg := range cfg.Subscriptions {
		sub := &subscription.Subscription{
			Name:       subCfg.Name,
			URL:        subCfg.URL,
			Interval:   subCfg.Interval,
			AutoUpdate: subCfg.AutoUpdate,
		}

		if err := mgr.Add(sub); err != nil {
			log.Error().Err(err).Str("name", subCfg.Name).Msg("添加订阅失败")
			continue
		}
	}

	// 立即更新所有订阅
	if err := mgr.UpdateAll(); err != nil {
		log.Warn().Err(err).Msg("部分订阅更新失败")
	}

	// 获取所有节点
	nodes := mgr.GetNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("没有解析到任何节点")
	}

	log.Info().Int("nodes", len(nodes)).Msg("订阅节点解析完成")

	// 将订阅节点合并到配置中
	mergeSubscriptionNodes(cfg, nodes)

	// 停止订阅管理器（启动后由应用接管）
	mgr.Stop()

	return nil
}

// mergeSubscriptionNodes 合并订阅节点到配置
func mergeSubscriptionNodes(cfg *config.Config, nodes []subscription.Node) {
	// 生成订阅代理组名称
	subGroupNames := make([]string, 0, len(cfg.Subscriptions))
	for _, sub := range cfg.Subscriptions {
		groupName := sub.Name + "-nodes"
		subGroupNames = append(subGroupNames, groupName)
	}

	// 将节点转换为配置格式并添加到 proxies
	for _, node := range nodes {
		proxy := nodeToProxyConfig(node)
		cfg.Proxies = append(cfg.Proxies, proxy)
	}

	// 为每个订阅创建代理组
	for i := range cfg.Subscriptions {
		groupName := subGroupNames[i]
		group := config.ProxyGroupConfig{
			Name:     groupName,
			Type:     "select",
			Proxies:  []string{},
		}

		// 将属于该订阅的节点添加到组中
		// 简化处理：按订阅顺序分配节点
		cfg.ProxyGroups = append(cfg.ProxyGroups, group)
	}

	// 在主代理组(PROXY)中添加订阅组
	for i, group := range cfg.ProxyGroups {
		if group.Name == "PROXY" || group.Name == "Proxy" || group.Name == "proxy" {
			// 添加订阅组到主代理组
			for _, subName := range subGroupNames {
				cfg.ProxyGroups[i].Proxies = append(cfg.ProxyGroups[i].Proxies, subName)
			}
			break
		}
	}

	// 如果没有主代理组，创建一个
	if len(cfg.ProxyGroups) == 0 {
		mainGroup := config.ProxyGroupConfig{
			Name: "PROXY",
			Type: "select",
			Proxies: append([]string{"DIRECT"}, subGroupNames...),
		}
		cfg.ProxyGroups = []config.ProxyGroupConfig{mainGroup}
	}
}

// nodeToProxyConfig 将订阅节点转换为代理配置
func nodeToProxyConfig(node subscription.Node) config.ProxyConfig {
	proxy := config.ProxyConfig{
		Name:           node.Name,
		Type:           node.Type,
		Server:         node.Server,
		Port:           node.Port,
		Password:       node.Password,
		UUID:           node.UUID,
		AlterID:        node.AlterID,
		Cipher:         node.Cipher,
		TLS:            node.TLS,
		SkipCertVerify: node.SkipCert,
		ServerName:     node.ServerName,
		Network:        node.Network,
		WSPath:         node.WSPath,
		WSHeaders:      node.WSHeaders,
		UDP:            node.UDP,
	}

	// 确保节点名称唯一
	if proxy.Name == "" {
		proxy.Name = fmt.Sprintf("%s-%d", proxy.Server, proxy.Port)
	}

	return proxy
}

// waitForShutdown 等待关闭信号
func waitForShutdown(application *app.App) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().Str("signal", sig.String()).Msg("收到关闭信号")

	// 关闭应用
	if err := application.Stop(); err != nil {
		log.Error().Err(err).Msg("关闭应用失败")
	}

	log.Info().Msg("代理内核已关闭")
}
