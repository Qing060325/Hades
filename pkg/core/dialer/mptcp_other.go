//go:build !linux

package dialer

// setMPTCP 在非 Linux 平台上 MPTCP 不可用，返回 nil（静默忽略）
func setMPTCP(fd int) error {
	// MPTCP 仅在 Linux 5.6+ 内核上受支持
	// 非 Linux 平台静默忽略此选项
	return nil
}
