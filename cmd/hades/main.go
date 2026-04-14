// Package main 程序入口
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Qing060325/Hades/internal/app"
	"github.com/Qing060325/Hades/internal/config"
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
	log.Info().Str("version", version.Version).Msg("启动代理内核")

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
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().Str("signal", sig.String()).Msg("收到关闭信号")

	// 关闭应用
	if err := application.Stop(); err != nil {
		log.Error().Err(err).Msg("关闭应用失败")
	}

	log.Info().Msg("代理内核已关闭")
}
