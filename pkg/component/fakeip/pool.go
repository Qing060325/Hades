// Package fakeip Fake-IP DNS 模式
package fakeip

import (
	"encoding/binary"
	"net"
	"sync"
)

// Pool Fake-IP 地址池
type Pool struct {
	mu       sync.RWMutex
	ipRange  net.IPNet
	current  uint32
	start    uint32
	end      uint32
	mapping  map[uint32]string // ip -> domain
	reverse  map[string]uint32 // domain -> ip
	gateway  net.IP
}

// NewPool 创建 Fake-IP 池
func NewPool(cidr string) (*Pool, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ip := ipNet.IP.To4()
	if ip == nil {
		return nil, err
	}

	startIP := binary.BigEndian.Uint32(ip)
	mask := binary.BigEndian.Uint32(ipNet.Mask)
	start := startIP + 1       // 跳过网络地址
	end := (startIP | ^mask) - 1 // 跳过广播地址

	return &Pool{
		ipRange: *ipNet,
		current: start,
		start:   start,
		end:     end,
		mapping: make(map[uint32]string),
		reverse: make(map[string]uint32),
	}, nil
}

// Get 获取域名的 Fake-IP
func (p *Pool) Get(domain string) net.IP {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 已存在映射
	if ip, ok := p.reverse[domain]; ok {
		return uint32ToIP(ip)
	}

	// 分配新 IP
	ip := p.allocate()
	p.mapping[ip] = domain
	p.reverse[domain] = ip
	return uint32ToIP(ip)
}

// Lookup 查找 Fake-IP 对应的域名
func (p *Pool) Lookup(ip net.IP) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ipUint := binary.BigEndian.Uint32(ip.To4())
	domain, ok := p.mapping[ipUint]
	return domain, ok
}

// IsFakeIP 检查是否是 Fake-IP
func (p *Pool) IsFakeIP(ip net.IP) bool {
	ipUint := binary.BigEndian.Uint32(ip.To4())
	return ipUint >= p.start && ipUint <= p.end
}

func (p *Pool) allocate() uint32 {
	for {
		ip := p.current
		p.current++
		if p.current > p.end {
			p.current = p.start
		}
		if _, used := p.mapping[ip]; !used {
			return ip
		}
	}
}

func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}
