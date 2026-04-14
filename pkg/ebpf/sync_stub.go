//go:build !linux

package ebpf

import (
	"fmt"
	"runtime"

	"github.com/Qing060325/Hades/pkg/core/rules"
)

// RuleSyncer 规则同步器 stub（非 Linux）
type RuleSyncer struct{}

// NewRuleSyncer 创建规则同步器
func NewRuleSyncer(mgr *Manager) *RuleSyncer {
	return &RuleSyncer{}
}

// SyncFromEngine 从规则引擎同步（非 Linux 为空操作）
func (s *RuleSyncer) SyncFromEngine(engine *rules.Engine) error {
	return fmt.Errorf("eBPF 不支持 %s 平台", runtime.GOOS)
}
