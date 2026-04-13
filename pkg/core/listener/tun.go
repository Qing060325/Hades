// Package listener TUN监听器
package listener

import (
	"context"
	"fmt"
	"net"

	"github.com/hades/hades/internal/config"
	"github.com/hades/hades/pkg/core/adapter"
	"github.com/hades/hades/pkg/core/group"
	"github.com/hades/hades/pkg/core/rules"
)

// TUNListener TUN监听器
type TUNListener struct {
	cfg         *config.TunConfig
	adapterMgr  *adapter.Manager
	ruleEngine  *rules.Engine
	groupMgr    *group.Manager
}

// NewTUNListener 创建TUN监听器
func NewTUNListener(
	cfg *config.TunConfig,
	adapterMgr *adapter.Manager,
	ruleEngine *rules.Engine,
	groupMgr *group.Manager,
) (*TUNListener, error) {
	return &TUNListener{
		cfg:        cfg,
		adapterMgr: adapterMgr,
		ruleEngine: ruleEngine,
		groupMgr:   groupMgr,
	}, nil
}

// Listen 启动TUN监听
func (l *TUNListener) Listen(ctx context.Context) error {
	// TODO: 实现完整的TUN模式
	// 1. 创建TUN设备
	// 2. 配置IP地址和路由
	// 3. 启动数据包处理循环
	return fmt.Errorf("TUN模式尚未完全实现")
}

// Close 关闭TUN监听器
func (l *TUNListener) Close() error {
	return nil
}

// Addr 返回地址
func (l *TUNListener) Addr() net.Addr {
	return nil
}
