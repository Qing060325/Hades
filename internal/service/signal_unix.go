//go:build !windows
// +build !windows

package service

import (
	"os"
	"os/signal"
	"syscall"
)

// setupServiceSignals 设置 Unix 系统的服务信号
func setupServiceSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
}
