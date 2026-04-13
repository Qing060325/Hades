// Package tun 网络栈实现
package tun

import (
	"context"
	"sync"
)

// GVisorStack gVisor 网络栈（占位实现）
type GVisorStack struct {
	device   Device
	listener *TunListener
	mu       sync.Mutex
}

// newGVisorStack 创建 gVisor 网络栈（占位实现）
func newGVisorStack(device Device, listener *TunListener) (*GVisorStack, error) {
	return &GVisorStack{
		device:   device,
		listener: listener,
	}, nil
}

// Start 启动网络栈
func (s *GVisorStack) Start(ctx context.Context, device Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	go s.readLoop(ctx)

	return nil
}

// Stop 停止网络栈
func (s *GVisorStack) Stop() error {
	return nil
}

// readLoop 读取数据包循环
func (s *GVisorStack) readLoop(ctx context.Context) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := s.device.Read(buf)
			if err != nil {
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				go s.handlePacket(data)
			}
		}
	}
}

// handlePacket 处理数据包
func (s *GVisorStack) handlePacket(data []byte) {
	if len(data) < 20 {
		return
	}

	s.listener.HandlePacket(data)
}

// SystemStack 系统网络栈
type SystemStack struct {
	device   Device
	listener *TunListener
}

// newSystemStack 创建系统网络栈
func newSystemStack(device Device, listener *TunListener) (*SystemStack, error) {
	return &SystemStack{
		device:   device,
		listener: listener,
	}, nil
}

// Start 启动
func (s *SystemStack) Start(ctx context.Context, device Device) error {
	go s.readLoop(ctx)
	return nil
}

// Stop 停止
func (s *SystemStack) Stop() error {
	return nil
}

// readLoop 读取循环
func (s *SystemStack) readLoop(ctx context.Context) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := s.device.Read(buf)
			if err != nil {
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				go s.listener.HandlePacket(data)
			}
		}
	}
}

// MixedStack 混合网络栈（占位实现）
type MixedStack struct {
	device   Device
	listener *TunListener
}

// newMixedStack 创建混合网络栈
func newMixedStack(device Device, listener *TunListener) (*MixedStack, error) {
	return &MixedStack{
		device:   device,
		listener: listener,
	}, nil
}

// Start 启动
func (s *MixedStack) Start(ctx context.Context, device Device) error {
	go s.readLoop(ctx)
	return nil
}

// Stop 停止
func (s *MixedStack) Stop() error {
	return nil
}

// readLoop 读取循环
func (s *MixedStack) readLoop(ctx context.Context) {
	buf := make([]byte, 65536)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := s.device.Read(buf)
			if err != nil {
				continue
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				go s.listener.HandlePacket(data)
			}
		}
	}
}
