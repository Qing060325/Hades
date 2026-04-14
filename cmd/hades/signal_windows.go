//go:build windows
// +build windows

package main

import (
	"os"
	"os/signal"
)

// setupShutdownSignals 设置 Windows 系统的关闭信号
func setupShutdownSignals(sigChan chan os.Signal) {
	// Windows 不支持 SIGTERM，只使用 os.Interrupt (Ctrl+C)
	signal.Notify(sigChan, os.Interrupt)
}

// getShutdownMessage 获取关闭信号的消息
func getShutdownMessage(sig os.Signal) string {
	return sig.String()
}
