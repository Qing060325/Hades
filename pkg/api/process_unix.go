//go:build !windows
// +build !windows

package api

import (
	"os"
	"syscall"
)

// terminateProcess 发送终止信号给进程（Unix 版本）
func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
