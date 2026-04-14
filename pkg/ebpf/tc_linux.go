//go:build linux

package ebpf

import (
	"fmt"
	"os/exec"

	"github.com/cilium/ebpf"
)

// attachTCProgram 将 BPF 程序附加到网络接口的 TC ingress
// 使用 tc 命令通过 netlink 操作
func attachTCProgram(ifName string, prog *ebpf.Program) error {
	// 1. 添加 clsact qdisc（如果不存在）
	addQdisc := exec.Command("tc", "qdisc", "add", "dev", ifName, "clsact")
	_ = addQdisc.Run() // 忽略错误（可能已存在）

	// 2. 获取程序 fd
	fd := prog.FD()

	// 3. 附加 BPF 程序到 ingress
	// 使用 tc filter 命令
	addFilter := exec.Command("tc", "filter", "add", "dev", ifName,
		"ingress", "bpf", "direct-action",
		"fd", fmt.Sprintf("%d", fd))
	if output, err := addFilter.CombinedOutput(); err != nil {
		return fmt.Errorf("tc filter add 失败: %w, output: %s", err, string(output))
	}

	return nil
}

// detachTCProgram 从网络接口分离 TC BPF 程序
func detachTCProgram(ifName string) error {
	// 删除所有 ingress filter
	delFilter := exec.Command("tc", "filter", "del", "dev", ifName, "ingress")
	_ = delFilter.Run() // 忽略错误

	// 删除 clsact qdisc
	delQdisc := exec.Command("tc", "qdisc", "del", "dev", ifName, "clsact")
	_ = delQdisc.Run() // 忽略错误

	return nil
}
