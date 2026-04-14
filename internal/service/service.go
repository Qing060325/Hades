// Package service 跨平台服务管理
package service

import (
	"os"

	"github.com/Qing060325/Hades/internal/app"
	"github.com/Qing060325/Hades/internal/config"
	svc "github.com/kardianos/service"
	"github.com/rs/zerolog/log"
)

// 全局日志器
var logger svc.Logger

// HadesService Hades 服务实现
type HadesService struct {
	configPath string
	app        *app.App
	stopCh     chan struct{}
}

// Start 实现 service.Interface 的 Start 方法
func (s *HadesService) Start(svcService svc.Service) error {
	s.stopCh = make(chan struct{})

	// 启动后台运行
	go s.run()

	return nil
}

// run 实际运行逻辑
func (s *HadesService) run() {
	// 设置服务停止回调
	if logger != nil {
		logger.Info("正在启动 Hades 服务...")
	}

	// 加载配置
	cfg, err := config.ParseFile(s.configPath)
	if err != nil {
		if logger != nil {
			logger.Error(err)
		}
		log.Fatal().Err(err).Str("path", s.configPath).Msg("加载配置失败")
		return
	}

	// 创建应用
	application, err := app.New(cfg)
	if err != nil {
		if logger != nil {
			logger.Error(err)
		}
		log.Fatal().Err(err).Msg("创建应用失败")
		return
	}
	s.app = application

	// 启动应用
	if err := application.Start(); err != nil {
		if logger != nil {
			logger.Error(err)
		}
		log.Fatal().Err(err).Msg("启动应用失败")
		return
	}

	if logger != nil {
		logger.Info("Hades 服务已启动")
	}
	log.Info().Msg("Hades 服务已启动")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	setupServiceSignals(sigChan)

	select {
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("收到停止信号")
	case <-s.stopCh:
		log.Info().Msg("收到服务停止请求")
	}

	// 停止应用
	if err := s.app.Stop(); err != nil {
		log.Error().Err(err).Msg("停止应用失败")
	}

	log.Info().Msg("Hades 服务已停止")
}

// Stop 实现 service.Interface 的 Stop 方法
func (s *HadesService) Stop(svcService svc.Service) error {
	if s.app != nil {
		if err := s.app.Stop(); err != nil {
			log.Error().Err(err).Msg("停止应用失败")
			return err
		}
	}
	return nil
}

// Run 以服务模式或交互模式运行 Hades
func Run(configPath string, serviceName string, serviceDisplayName string, serviceDescription string) {
	// 创建服务配置
	svcConfig := &svc.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
	}

	hadesService := &HadesService{
		configPath: configPath,
	}

	// 创建服务实例
	svcInstance, err := svc.New(hadesService, svcConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("创建服务失败")
	}

	// 创建日志器
	logger, err = svcInstance.Logger(nil)
	if err != nil {
		log.Fatal().Err(err).Msg("创建服务日志失败")
	}

	// 运行服务
	if err := svcInstance.Run(); err != nil {
		logger.Error(err)
		log.Fatal().Err(err).Msg("服务运行失败")
	}
}

// InstallService 安装服务
func InstallService(configPath string, serviceName string, serviceDisplayName string, serviceDescription string) error {
	svcConfig := &svc.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
	}

	s := &HadesService{configPath: configPath}

	svcInstance, err := svc.New(s, svcConfig)
	if err != nil {
		return err
	}

	return svcInstance.Install()
}

// UninstallService 卸载服务
func UninstallService(serviceName string) error {
	svcConfig := &svc.Config{
		Name: serviceName,
	}

	svcInstance, err := svc.New(&HadesService{}, svcConfig)
	if err != nil {
		return err
	}

	return svcInstance.Uninstall()
}

// ServiceStatus 服务状态
type ServiceStatus int

const (
	StatusUnknown ServiceStatus = iota
	StatusRunning
	StatusStopped
)

// GetServiceStatus 获取服务状态
func GetServiceStatus(serviceName string) (ServiceStatus, error) {
	svcConfig := &svc.Config{
		Name: serviceName,
	}

	svcInstance, err := svc.New(&HadesService{}, svcConfig)
	if err != nil {
		return StatusUnknown, err
	}

	status, err := svcInstance.Status()
	if err != nil {
		return StatusUnknown, err
	}

	switch status {
	case svc.StatusRunning:
		return StatusRunning, nil
	case svc.StatusStopped:
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}

// ControlService 控制服务 (start/stop/restart)
func ControlService(serviceName string, action string) error {
	svcConfig := &svc.Config{
		Name: serviceName,
	}

	svcInstance, err := svc.New(&HadesService{}, svcConfig)
	if err != nil {
		return err
	}

	switch action {
	case "start":
		return svcInstance.Start()
	case "stop":
		return svcInstance.Stop()
	case "restart":
		if err := svcInstance.Stop(); err != nil {
			return err
		}
		return svcInstance.Start()
	default:
		return nil
	}
}

// Interactive 检测是否在交互模式运行
func Interactive() bool {
	return svc.Interactive()
}
