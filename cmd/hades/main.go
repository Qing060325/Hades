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

	return config.ParseFile(path)
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
