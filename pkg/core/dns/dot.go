// Package dns DNS-over-TLS (DoT) 解析器 - RFC 7858
package dns

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// DoTResolver DNS-over-TLS 解析器
type DoTResolver struct {
	server   string
	tlsConfig *tls.Config
}

// NewDoTResolver 创建 DoT 解析器
// server 示例: "tls://8.8.8.4:853" 或 "tls://1.1.1.1"
func NewDoTResolver(server string) *DoTResolver {
	// 去掉 tls:// 前缀
	addr := strings.TrimPrefix(server, "tls://")
	// 添加默认端口
	if !strings.Contains(addr, ":") {
		addr = addr + ":853"
	}

	return &DoTResolver{
		server: addr,
		tlsConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

// Resolve 通过 DoT 解析域名
func (r *DoTResolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// 并发查询 A 和 AAAA
	type result struct {
		ips []net.IP
		err error
	}

	ch := make(chan result, 2)

	go func() {
		ips, err := r.queryType(ctx, host, 1) // A
		ch <- result{ips, err}
	}()

	go func() {
		ips, err := r.queryType(ctx, host, 28) // AAAA
		ch <- result{ips, err}
	}()

	var allIPs []net.IP
	for i := 0; i < 2; i++ {
		res := <-ch
		if res.err == nil {
			allIPs = append(allIPs, res.ips...)
		}
	}

	if len(allIPs) == 0 {
		return nil, fmt.Errorf("DoT: no records for %s", host)
	}

	return allIPs, nil
}

// queryType 查询指定类型的 DNS 记录
func (r *DoTResolver) queryType(ctx context.Context, host string, qtype uint16) ([]net.IP, error) {
	// 构建 DNS 查询
	query, err := buildDoTQuery(host, qtype)
	if err != nil {
		return nil, err
	}

	// 建立 TLS 连接
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", r.server, r.tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("DoT dial: %w", err)
	}
	defer conn.Close()

	// 设置超时
	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetDeadline(deadline)
	} else {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	// DoT 使用 TCP 长度前缀 (RFC 1035 Section 4.2.2)
	lengthPrefix := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthPrefix, uint16(len(query)))

	_, err = conn.Write(append(lengthPrefix, query...))
	if err != nil {
		return nil, fmt.Errorf("DoT write: %w", err)
	}

	// 读取响应长度
	_, err = conn.Read(lengthPrefix)
	if err != nil {
		return nil, fmt.Errorf("DoT read length: %w", err)
	}

	respLen := binary.BigEndian.Uint16(lengthPrefix)
	if respLen == 0 || respLen > 4096 {
		return nil, fmt.Errorf("DoT: invalid response length %d", respLen)
	}

	// 读取响应
	resp := make([]byte, respLen)
	_, err = conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("DoT read response: %w", err)
	}

	return parseDoTResponse(resp)
}

// buildDoTQuery 构建 DNS 查询报文
func buildDoTQuery(host string, qtype uint16) ([]byte, error) {
	// 与 DoH 使用相同的报文格式
	query := make([]byte, 12)
	// ID = 0x1234 (任意)
	query[0] = 0x12
	query[1] = 0x34
	// Flags: standard query, RD=1
	query[2] = 0x01
	query[3] = 0x00
	// QDCOUNT = 1
	query[4] = 0
	query[5] = 1

	host = strings.TrimSuffix(host, ".")
	parts := strings.Split(host, ".")
	for _, part := range parts {
		if len(part) > 63 {
			return nil, fmt.Errorf("label too long: %s", part)
		}
		query = append(query, byte(len(part)))
		query = append(query, []byte(part)...)
	}
	query = append(query, 0) // root label

	// QTYPE
	query = append(query, byte(qtype>>8), byte(qtype))
	// QCLASS = IN (1)
	query = append(query, 0, 1)

	return query, nil
}

// parseDoTResponse 解析 DoT 响应
func parseDoTResponse(data []byte) ([]net.IP, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("response too short")
	}

	ancount := uint16(data[6])<<8 | uint16(data[7])
	var ips []net.IP

	// 跳过 Question section
	offset := 12
	qdcount := uint16(data[4])<<8 | uint16(data[5])
	for i := uint16(0); i < qdcount; i++ {
		for offset < len(data) {
			if data[offset] == 0 {
				offset++
				break
			}
			if data[offset]&0xC0 == 0xC0 {
				offset += 2
				break
			}
			offset += int(data[offset]) + 1
		}
		offset += 4
	}

	// 解析 Answer section
	for i := uint16(0); i < ancount && offset < len(data); i++ {
		for offset < len(data) {
			if data[offset] == 0 {
				offset++
				break
			}
			if data[offset]&0xC0 == 0xC0 {
				offset += 2
				break
			}
			offset += int(data[offset]) + 1
		}

		if offset+10 > len(data) {
			break
		}

		ansType := uint16(data[offset])<<8 | uint16(data[offset+1])
		rdlength := uint16(data[offset+8])<<8 | uint16(data[offset+9])
		offset += 10

		if offset+int(rdlength) > len(data) {
			break
		}

		switch ansType {
		case 1:
			if rdlength == 4 {
				ips = append(ips, net.IP(append([]byte{}, data[offset:offset+4]...)))
			}
		case 28:
			if rdlength == 16 {
				ips = append(ips, net.IP(append([]byte{}, data[offset:offset+16]...)))
			}
		}

		offset += int(rdlength)
	}

	return ips, nil
}
