//go:build windows
// +build windows

package service

import (
	"os"
	"os/signal"
)

// setupServiceSignals 设置 Windows 系统的服务信号
func setupServiceSignals(sigChan chan os.Signal) {
	// Windows 不支持 SIGTERM，只使用 os.Interrupt
	signal.Notify(sigChan, os.Interrupt)
}
