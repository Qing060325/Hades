// Package dns DNS解析模块
package dns

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/config"
	"github.com/rs/zerolog/log"
)

// Client DNS客户端
type Client struct {
	cfg   *config.DNSConfig
	cache *LRUCache

	// Fake-IP
	fakeIPPool *FakeIPPool

	// DNS服务器客户端
	nameservers []Resolver

	mu sync.RWMutex
}

// Resolver DNS解析器接口
type Resolver interface {
	Resolve(ctx context.Context, host string) ([]net.IP, error)
}

// NewClient 创建DNS客户端
func NewClient(cfg *config.DNSConfig) (*Client, error) {
	c := &Client{
		cfg:         cfg,
		cache:       NewLRUCache(4096),
		nameservers: make([]Resolver, 0),
	}

	// 初始化Fake-IP池
	if cfg.EnhancedMode == "fake-ip" {
		c.fakeIPPool = NewFakeIPPool(cfg.FakeIPRange)
	}

	// 初始化DNS服务器
	for _, ns := range cfg.Nameserver {
		r, err := NewResolver(ns)
		if err != nil {
			log.Warn().Err(err).Str("nameserver", ns).Msg("初始化DNS服务器失败")
			continue
		}
		c.nameservers = append(c.nameservers, r)
	}

	return c, nil
}

// Listen 启动DNS服务 (同时监听 UDP 和 TCP)
func (c *Client) Listen(ctx context.Context, addr string) error {
	errCh := make(chan error, 2)

	// 启动 UDP 监听
	go func() {
		errCh <- c.listenUDP(ctx, addr)
	}()

	// 启动 TCP 监听
	go func() {
		errCh <- c.listenTCP(ctx, addr)
	}()

	// 等待任一出错或 context 取消
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
}

// listenUDP 监听 UDP DNS 查询
func (c *Client) listenUDP(ctx context.Context, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve udp addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	log.Info().Str("addr", addr).Str("proto", "udp").Msg("DNS服务已启动")

	buf := make([]byte, 4096) // EDNS0 支持更大的包
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}

			go c.handleDNSQueryUDP(conn, remoteAddr, buf[:n])
		}
	}
}

// listenTCP 监听 TCP DNS 查询
func (c *Client) listenTCP(ctx context.Context, addr string) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return fmt.Errorf("resolve tcp addr: %w", err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("listen tcp: %w", err)
	}
	defer listener.Close()

	log.Info().Str("addr", addr).Str("proto", "tcp").Msg("DNS TCP服务已启动")

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}

		go c.handleDNSConnTCP(conn)
	}
}

// handleDNSConnTCP 处理 TCP DNS 连接
func (c *Client) handleDNSConnTCP(conn *net.TCPConn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// TCP DNS 使用 2 字节长度前缀 (RFC 1035 Section 4.2.2)
	lenBuf := make([]byte, 2)
	if _, err := conn.Read(lenBuf); err != nil {
		return
	}

	queryLen := binary.BigEndian.Uint16(lenBuf)
	if queryLen == 0 || queryLen > 4096 {
		return
	}

	query := make([]byte, queryLen)
	if _, err := conn.Read(query); err != nil {
		return
	}

	// 处理查询
	response := c.processQuery(query)
	if response == nil {
		return
	}

	// TCP 响应也需要长度前缀
	respLen := make([]byte, 2)
	binary.BigEndian.PutUint16(respLen, uint16(len(response)))
	conn.Write(append(respLen, response...))
}

// handleDNSQueryUDP 处理 UDP DNS 查询
func (c *Client) handleDNSQueryUDP(conn *net.UDPConn, remoteAddr *net.UDPAddr, query []byte) {
	response := c.processQuery(query)
	if response != nil {
		conn.WriteToUDP(response, remoteAddr)
	}
}

// processQuery 处理 DNS 查询并返回响应
func (c *Client) processQuery(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}

	queryID := binary.BigEndian.Uint16(query[0:2])
	qdCount := binary.BigEndian.Uint16(query[4:6])
	if qdCount == 0 {
		return nil
	}

	// 解析 Question Section
	offset := 12
	var qname string
	var qtype uint16

	for i := 0; i < int(qdCount); i++ {
		name, newOffset, err := parseDNSName(query, offset)
		if err != nil || newOffset+4 > len(query) {
			return nil
		}
		qname = name
		qtype = binary.BigEndian.Uint16(query[newOffset : newOffset+2])
		offset = newOffset + 4
	}

	// 检查缓存
	if answers, ok := c.cache.Get(qname); ok && len(answers) > 0 {
		return buildDNSResponse(queryID, query, answers, qtype)
	}

	// Fake-IP 模式
	if c.fakeIPPool != nil && qtype == 1 {
		fakeIP := c.fakeIPPool.Get(qname)
		if fakeIP != nil {
			return buildDNSResponse(queryID, query, []net.IP{fakeIP}, qtype)
		}
	}

	// 转发到上游 DNS
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := c.Resolve(ctx, qname)
	if err != nil || len(ips) == 0 {
		return buildDNSErrorResponse(queryID, query, 2)
	}

	return buildDNSResponse(queryID, query, ips, qtype)
}

// parseDNSName 解析 DNS 域名（支持指针压缩）
func parseDNSName(data []byte, offset int) (string, int, error) {
	if offset >= len(data) {
		return "", offset, fmt.Errorf("offset 超出范围")
	}

	var name []byte
	jumpOffset := offset
	jumped := false

	for {
		if offset >= len(data) {
			return "", offset, fmt.Errorf("域名解析越界")
		}

		length := data[offset]

		if length == 0 {
			offset++
			break
		}

		// 指针压缩
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", offset, fmt.Errorf("指针越界")
			}
			pointer := int(binary.BigEndian.Uint16(data[offset:offset+2]) & 0x3FFF)
			if !jumped {
				jumpOffset = offset + 2
			}
			offset = pointer
			jumped = true
			continue
		}

		offset++
		if offset+int(length) > len(data) {
			return "", offset, fmt.Errorf("标签越界")
		}

		if len(name) > 0 {
			name = append(name, '.')
		}
		name = append(name, data[offset:offset+int(length)]...)
		offset += int(length)
	}

	if !jumped {
		return string(name), offset, nil
	}
	return string(name), jumpOffset, nil
}

// buildDNSResponse 构建 DNS 响应报文
func buildDNSResponse(queryID uint16, query []byte, ips []net.IP, qtype uint16) []byte {
	// 响应头
	response := make([]byte, 0, 512)

	// Header
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], queryID)
	binary.BigEndian.PutUint16(header[2:4], 0x8180) // Standard response, recursion available
	binary.BigEndian.PutUint16(header[4:6], 1)       // QDCOUNT
	binary.BigEndian.PutUint16(header[6:8], uint16(len(ips))) // ANCOUNT
	binary.BigEndian.PutUint16(header[8:10], 0)      // NSCOUNT
	binary.BigEndian.PutUint16(header[10:12], 0)     // ARCOUNT
	response = append(response, header...)

	// Question Section (从原始查询中复制)
	if len(query) > 12 {
		// 查找 question 结束位置
		qEnd := 12
		for qEnd < len(query) {
			if query[qEnd] == 0 {
				qEnd += 5 // null + QTYPE(2) + QCLASS(2)
				break
			}
			if query[qEnd]&0xC0 == 0xC0 {
				qEnd += 2
				break
			}
			qEnd += int(query[qEnd]) + 1
		}
		if qEnd <= len(query) {
			response = append(response, query[12:qEnd]...)
		}
	}

	// Answer Section
	for _, ip := range ips {
		ip4 := ip.To4()
		if qtype == 1 && ip4 != nil {
			// A 记录
			answer := make([]byte, 2+2+2+4+2+4)
			// Name (指针指向查询名)
			binary.BigEndian.PutUint16(answer[0:2], 0xC00C)
			// Type A
			binary.BigEndian.PutUint16(answer[2:4], 1)
			// Class IN
			binary.BigEndian.PutUint16(answer[4:6], 1)
			// TTL (300秒)
			binary.BigEndian.PutUint32(answer[6:10], 300)
			// RDLENGTH
			binary.BigEndian.PutUint16(answer[10:12], 4)
			// RDATA
			copy(answer[12:16], ip4)
			response = append(response, answer...)
		} else if qtype == 28 {
			// AAAA 记录
			ip6 := ip.To16()
			if ip6 != nil {
				answer := make([]byte, 2+2+2+4+2+16)
				binary.BigEndian.PutUint16(answer[0:2], 0xC00C)
				binary.BigEndian.PutUint16(answer[2:4], 28)
				binary.BigEndian.PutUint16(answer[4:6], 1)
				binary.BigEndian.PutUint32(answer[6:10], 300)
				binary.BigEndian.PutUint16(answer[10:12], 16)
				copy(answer[12:28], ip6)
				response = append(response, answer...)
			}
		}
	}

	return response
}

// buildDNSErrorResponse 构建 DNS 错误响应
func buildDNSErrorResponse(queryID uint16, query []byte, rcode uint16) []byte {
	response := make([]byte, 0, 512)

	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], queryID)
	binary.BigEndian.PutUint16(header[2:4], 0x8180|rcode) // Response + RCODE
	binary.BigEndian.PutUint16(header[4:6], 1)
	binary.BigEndian.PutUint16(header[6:8], 0)
	binary.BigEndian.PutUint16(header[8:10], 0)
	binary.BigEndian.PutUint16(header[10:12], 0)
	response = append(response, header...)

	// Question Section
	if len(query) > 12 {
		qEnd := 12
		for qEnd < len(query) {
			if query[qEnd] == 0 {
				qEnd += 5
				break
			}
			if query[qEnd]&0xC0 == 0xC0 {
				qEnd += 2
				break
			}
			qEnd += int(query[qEnd]) + 1
		}
		if qEnd <= len(query) {
			response = append(response, query[12:qEnd]...)
		}
	}

	return response
}

// Resolve 解析域名
func (c *Client) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// 检查缓存
	if ips, ok := c.cache.Get(host); ok {
		return ips, nil
	}

	// 使用DNS服务器解析
	for _, r := range c.nameservers {
		ips, err := r.Resolve(ctx, host)
		if err == nil {
			c.cache.Set(host, ips, time.Minute*5)
			return ips, nil
		}
	}

	return nil, fmt.Errorf("解析失败: %s", host)
}

// FakeIP 获取Fake-IP
func (c *Client) FakeIP(host string) net.IP {
	if c.fakeIPPool != nil {
		return c.fakeIPPool.Get(host)
	}
	return nil
}

// LRUCache LRU缓存
type LRUCache struct {
	capacity int
	data     map[string]*lruNode
	head     *lruNode
	tail     *lruNode
	mu       sync.RWMutex
}

type lruNode struct {
	key       string
	ips       []net.IP
	expiresAt time.Time
	prev      *lruNode
	next      *lruNode
}

// NewLRUCache 创建LRU缓存
func NewLRUCache(capacity int) *LRUCache {
	c := &LRUCache{
		capacity: capacity,
		data:     make(map[string]*lruNode),
	}
	// 哨兵节点
	c.head = &lruNode{}
	c.tail = &lruNode{}
	c.head.next = c.tail
	c.tail.prev = c.head
	return c
}

// moveToHead 将节点移到头部
func (c *LRUCache) moveToHead(node *lruNode) {
	// 从当前位置摘除
	node.prev.next = node.next
	node.next.prev = node.prev
	// 插入到头部
	node.next = c.head.next
	node.prev = c.head
	c.head.next.prev = node
	c.head.next = node
}

// removeTail 移除尾部节点
func (c *LRUCache) removeTail() *lruNode {
	node := c.tail.prev
	if node == c.head {
		return nil
	}
	node.prev.next = c.tail
	c.tail.prev = node.prev
	return node
}

// Get 获取缓存
func (c *LRUCache) Get(key string) ([]net.IP, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.data[key]
	if !ok || time.Now().After(node.expiresAt) {
		if ok {
			// 过期，删除
			delete(c.data, key)
			node.prev.next = node.next
			node.next.prev = node.prev
		}
		return nil, false
	}
	c.moveToHead(node)
	return node.ips, true
}

// Set 设置缓存
func (c *LRUCache) Set(key string, ips []net.IP, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.data[key]; ok {
		// 更新已有节点
		node.ips = ips
		node.expiresAt = time.Now().Add(ttl)
		c.moveToHead(node)
		return
	}

	// 新节点
	node := &lruNode{
		key:       key,
		ips:       ips,
		expiresAt: time.Now().Add(ttl),
	}
	c.data[key] = node
	// 插入头部
	node.next = c.head.next
	node.prev = c.head
	c.head.next.prev = node
	c.head.next = node

	// 超出容量，淘汰尾部
	if len(c.data) > c.capacity {
		tail := c.removeTail()
		if tail != nil {
			delete(c.data, tail.key)
		}
	}
}

// FakeIPPool Fake-IP池
type FakeIPPool struct {
	prefix   string
	mu       sync.RWMutex
	hostToIP map[string]net.IP
	ipToHost map[string]string
	counter  uint32
	maxIP    uint32
}

// NewFakeIPPool 创建Fake-IP池
func NewFakeIPPool(rangeStr string) *FakeIPPool {
	return &FakeIPPool{
		prefix:   "198.18.0.",
		hostToIP: make(map[string]net.IP),
		ipToHost: make(map[string]string),
		counter:  1,
		maxIP:    254, // 198.18.0.1 - 198.18.0.254
	}
}

// Get 获取或分配Fake-IP
func (p *FakeIPPool) Get(host string) net.IP {
	p.mu.RLock()
	if ip, ok := p.hostToIP[host]; ok {
		p.mu.RUnlock()
		return ip
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// 再次检查
	if ip, ok := p.hostToIP[host]; ok {
		return ip
	}

	// 上限检查
	if p.counter > p.maxIP {
		p.counter = 1
	}

	// 分配新IP
	ip := net.ParseIP(fmt.Sprintf("%s%d", p.prefix, p.counter))
	p.counter++
	p.hostToIP[host] = ip
	p.ipToHost[ip.String()] = host

	return ip
}

// Lookup 通过IP查找主机名
func (p *FakeIPPool) Lookup(ip string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ipToHost[ip]
}

// udpResolver 标准 UDP DNS 解析器
type udpResolver struct {
	addr string
}

func newUDPResolver(addr string) *udpResolver {
	if !strings.Contains(addr, ":") {
		addr = addr + ":53"
	}
	return &udpResolver{addr: addr}
}

func (r *udpResolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", r.addr)
		},
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, nil
}

// quicResolver DNS-over-QUIC 解析器 (stub)
type quicResolver struct {
	server string
}

func newQUICResolver(server string) *quicResolver {
	addr := strings.TrimPrefix(server, "quic://")
	if !strings.Contains(addr, ":") {
		addr = addr + ":853"
	}
	return &quicResolver{server: addr}
}

func (r *quicResolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// DoQ stub - 回退到标准 DNS
	// TODO: 实现 DNS-over-QUIC (RFC 9250)
	return net.LookupIP(host)
}

// NewResolver 根据地址前缀创建对应协议的解析器
// - "https://" 前缀 → DoH 解析器
// - "tls://" 前缀 → DoT 解析器
// - "quic://" 前缀 → DoQ 解析器 (stub)
// - 纯 IP 地址 → 标准 UDP DNS 解析器
func NewResolver(addr string) (Resolver, error) {
	switch {
	case strings.HasPrefix(addr, "https://"):
		return NewDoHResolver(addr), nil
	case strings.HasPrefix(addr, "tls://"):
		return NewDoTResolver(addr), nil
	case strings.HasPrefix(addr, "quic://"):
		return newQUICResolver(addr), nil
	default:
		// 纯 IP 或 host:port → UDP DNS
		return newUDPResolver(addr), nil
	}
}
