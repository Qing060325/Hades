//go:build windows
// +build windows

package api

import (
	"os"
)

// terminateProcess 发送终止信号给进程（Windows 版本）
// Windows 不支持 SIGTERM，使用 Kill 代替
func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
