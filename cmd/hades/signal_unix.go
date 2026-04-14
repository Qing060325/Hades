//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// setupShutdownSignals 设置 Unix 系统的关闭信号
func setupShutdownSignals(sigChan chan os.Signal) {
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
}

// getShutdownMessage 获取关闭信号的消息
func getShutdownMessage(sig os.Signal) string {
	return sig.String()
}
