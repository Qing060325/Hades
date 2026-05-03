// Package dns DNS-over-HTTPS (DoH) 解析器 - RFC 8484
package dns

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DoHResolver DNS-over-HTTPS 解析器
type DoHResolver struct {
	url    string
	client *http.Client
}

// NewDoHResolver 创建 DoH 解析器
// url 示例: "https://dns.alidns.com/dns-query" 或 "https://cloudflare-dns.com/dns-query"
func NewDoHResolver(url string) *DoHResolver {
	return &DoHResolver{
		url: strings.TrimSuffix(url, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 5 * time.Second,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Resolve 通过 DoH 解析域名
func (r *DoHResolver) Resolve(ctx context.Context, host string) ([]net.IP, error) {
	// 构建 DNS 查询报文 (A + AAAA)
	queryA, err := buildDoHQuery(host, 1) // Type A
	if err != nil {
		return nil, fmt.Errorf("build A query: %w", err)
	}

	queryAAAA, err := buildDoHQuery(host, 28) // Type AAAA
	if err != nil {
		return nil, fmt.Errorf("build AAAA query: %w", err)
	}

	var allIPs []net.IP

	// 并发查询 A 和 AAAA
	type result struct {
		ips []net.IP
		err error
	}

	ch := make(chan result, 2)

	go func() {
		ips, err := r.doQuery(ctx, queryA)
		ch <- result{ips, err}
	}()

	go func() {
		ips, err := r.doQuery(ctx, queryAAAA)
		ch <- result{ips, err}
	}()

	for i := 0; i < 2; i++ {
		res := <-ch
		if res.err == nil {
			allIPs = append(allIPs, res.ips...)
		}
	}

	if len(allIPs) == 0 {
		return nil, fmt.Errorf("DoH: no records for %s", host)
	}

	return allIPs, nil
}

// doQuery 执行单个 DoH 查询
func (r *DoHResolver) doQuery(ctx context.Context, query []byte) ([]net.IP, error) {
	// RFC 8484: 使用 GET + base64url 编码
	encoded := base64.RawURLEncoding.EncodeToString(query)
	reqURL := r.url + "?dns=" + encoded

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		// GET 失败则尝试 POST
		return r.doPostQuery(ctx, query)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return r.doPostQuery(ctx, query)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, err
	}

	return parseDoHResponse(body)
}

// doPostQuery 使用 POST 方法发送 DoH 查询
func (r *DoHResolver) doPostQuery(ctx context.Context, query []byte) ([]net.IP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, strings.NewReader(string(query)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH POST returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, err
	}

	return parseDoHResponse(body)
}

// buildDoHQuery 构建 DNS 查询报文
func buildDoHQuery(host string, qtype uint16) ([]byte, error) {
	// 简单的 DNS 查询报文构建
	// Header
	query := make([]byte, 12)
	// ID = 0 (由 DoH 服务器分配)
	query[0] = 0
	query[1] = 0
	// Flags: standard query, RD=1
	query[2] = 0x01
	query[3] = 0x00
	// QDCOUNT = 1
	query[4] = 0
	query[5] = 1
	// ANCOUNT, NSCOUNT, ARCOUNT = 0
	query[6] = 0
	query[7] = 0
	query[8] = 0
	query[9] = 0
	query[10] = 0
	query[11] = 0

	// Question section
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

// parseDoHResponse 解析 DoH 响应报文，提取 IP 地址
func parseDoHResponse(data []byte) ([]net.IP, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("response too short")
	}

	// 解析 Header
	ancount := uint16(data[6])<<8 | uint16(data[7])

	var ips []net.IP

	// 跳过 Question section
	offset := 12
	qdcount := uint16(data[4])<<8 | uint16(data[5])
	for i := uint16(0); i < qdcount; i++ {
		// 跳过 QNAME
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
		offset += 4 // QTYPE + QCLASS
	}

	// 解析 Answer section
	for i := uint16(0); i < ancount && offset < len(data); i++ {
		// 跳过 NAME (可能包含指针)
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

		// TYPE
		ansType := uint16(data[offset])<<8 | uint16(data[offset+1])
		// CLASS (跳过)
		// TTL (跳过)
		// RDLENGTH
		rdlength := uint16(data[offset+8])<<8 | uint16(data[offset+9])
		offset += 10

		if offset+int(rdlength) > len(data) {
			break
		}

		switch ansType {
		case 1: // A record
			if rdlength == 4 {
				ips = append(ips, net.IP(append([]byte{}, data[offset:offset+4]...)))
			}
		case 28: // AAAA record
			if rdlength == 16 {
				ips = append(ips, net.IP(append([]byte{}, data[offset:offset+16]...)))
			}
		}

		offset += int(rdlength)
	}

	return ips, nil
}
