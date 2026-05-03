//go:build linux

package dialer

import "syscall"

// MPTCP 协议常量
const (
	// IPPROTO_TCP TCP 协议号
	IPPROTO_TCP = 0x6
	// MPTCP_ENABLED MPTCP 启用选项 (Linux 内核特定)
	MPTCP_ENABLED = 42
)

// setMPTCP 在指定 fd 上启用 MPTCP
// 通过 setsockopt 设置 IPPROTO_TCP 级别的 MPTCP_ENABLED 选项
func setMPTCP(fd int) error {
	return syscall.SetsockoptInt(fd, IPPROTO_TCP, MPTCP_ENABLED, 1)
}
