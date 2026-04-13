// Package sniffer 流量嗅探模块
package sniffer

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hades/hades/internal/config"
	"github.com/hades/hades/pkg/core/adapter"
)

// Sniffer 嗅探器
type Sniffer struct {
	cfg    *config.SnifferConfig
	enable bool
}

// NewSniffer 创建嗅探器
func NewSniffer(cfg *config.SnifferConfig) *Sniffer {
	return &Sniffer{
		cfg:    cfg,
		enable: cfg.Enable,
	}
}

// PeekConnection 嗅探连接
func (s *Sniffer) PeekConnection(conn net.Conn) (*adapter.Metadata, error) {
	if !s.enable {
		return nil, nil
	}

	// 读取前几个字节来判断协议
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Time{})

	data := buf[:n]

	// 判断协议类型
	var metadata *adapter.Metadata

	if s.isTLS(data) {
		metadata = s.peekTLS(data)
	} else if s.isHTTP(data) {
		metadata = s.peekHTTP(data)
	} else if s.isQUIC(data) {
		metadata = s.peekQUIC(data)
	}

	// 将读取的数据放回连接
	if n > 0 && metadata != nil {
		return metadata, nil
	}

	// 创建新连接，将已读数据作为缓冲
	return nil, nil
}

// isTLS 检测 TLS 协议
func (s *Sniffer) isTLS(data []byte) bool {
	if len(data) < 5 {
		return false
	}

	// TLS record: ContentType(1) + Version(2) + Length(2)
	// ContentType: 20=ChangeCipherSpec, 21=Alert, 22=Handshake, 23=Application
	contentType := data[0]
	if contentType < 20 || contentType > 23 {
		return false
	}

	// 版本检查: TLS 1.0=0x0301, TLS 1.1=0x0302, TLS 1.2=0x0303, TLS 1.3=0x0303
	version := uint16(data[1])<<8 | uint16(data[2])
	if version < 0x0301 || version > 0x0304 {
		return false
	}

	// 检查端口（可选）
	port := s.getPortFromConfig("TLS")
	if port == 0 {
		return true
	}

	return true
}

// isHTTP 检测 HTTP 协议
func (s *Sniffer) isHTTP(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// HTTP 方法: GET, POST, PUT, DELETE, HEAD, OPTIONS, PATCH, CONNECT
	methods := [][]byte{
		[]byte("GET "),
		[]byte("POST "),
		[]byte("PUT "),
		[]byte("DELETE "),
		[]byte("HEAD "),
		[]byte("OPTIONS "),
		[]byte("PATCH "),
		[]byte("CONNECT "),
	}

	for _, method := range methods {
		if bytes.HasPrefix(data, method) {
			return true
		}
	}

	return false
}

// isQUIC 检测 QUIC 协议
func (s *Sniffer) isQUIC(data []byte) bool {
	if len(data) < 5 {
		return false
	}

	// QUIC Long Header: 1开头的第一个字节
	// 格式: 1 | Form(1) | Fixed Bit(1) | Long Packet Type(2) | Reserved(2) | Packet Number Length(2)
	if data[0]&0x80 == 0 {
		return false
	}

	// 版本检查
	version := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
	// QUIC v1 = 0x00000001, QUIC v2 = 0x6b3343cf
	if version == 0x00000001 || version == 0x6b3343cf {
		return true
	}

	return false
}

// peekTLS 嗅探 TLS 连接
func (s *Sniffer) peekTLS(data []byte) *adapter.Metadata {
	metadata := &adapter.Metadata{}

	if len(data) < 43 {
		return metadata
	}

	// 解析 ClientHello
	// data[0] = ContentType (22 = Handshake)
	// data[1:3] = Version
	// data[3:5] = Length
	// data[5] = HandshakeType (1 = ClientHello)
	// data[6:9] = HandshakeLength (3 bytes)
	// data[9:11] = ClientHello Version
	// data[11:43] = Random (32 bytes)
	// data[43] = SessionID Length

	offset := 43
	if offset >= len(data) {
		return metadata
	}

	// SessionID Length
	sessionIDLen := int(data[offset])
	offset++

	// Skip SessionID
	offset += sessionIDLen

	if offset+2 >= len(data) {
		return metadata
	}

	// CipherSuites Length
	cipherSuitesLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2 + cipherSuitesLen

	if offset+1 >= len(data) {
		return metadata
	}

	// Compression Methods Length
	compressionLen := int(data[offset])
	offset++

	// Skip Compression Methods
	offset += compressionLen

	if offset+1 >= len(data) {
		return metadata
	}

	// Extensions Length
	extensionsLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2

	extensionsEnd := offset + extensionsLen

	// 解析扩展
	for offset+4 <= extensionsEnd && offset+4 <= len(data) {
		extType := uint16(data[offset])<<8 | uint16(data[offset+1])
		extLen := uint16(data[offset+2])<<8 | uint16(data[offset+3])
		offset += 4

		if offset+int(extLen) > len(data) {
			break
		}

		extData := data[offset : offset+int(extLen)]

		// SNI 扩展 (type = 0)
		if extType == 0 {
			sni := parseSNIExtension(extData)
			if sni != "" {
				metadata.Host = sni
				break
			}
		}

		offset += int(extLen)
	}

	return metadata
}

// parseSNIExtension 解析 SNI 扩展
func parseSNIExtension(data []byte) string {
	if len(data) < 5 {
		return ""
	}

	// Server Name List Length
	// data[0:2] = SNIList Length
	// data[2] = Name Type (0 = hostname)
	// data[3:5] = Name Length
	nameType := data[2]
	if nameType != 0 {
		return ""
	}

	nameLen := int(data[3])<<8 | int(data[4])
	if 5+nameLen > len(data) {
		return ""
	}

	return string(data[5 : 5+nameLen])
}

// peekHTTP 嗅探 HTTP 连接
func (s *Sniffer) peekHTTP(data []byte) *adapter.Metadata {
	metadata := &adapter.Metadata{}

	// 解析 HTTP 请求行
	reader := bufio.NewReader(bytes.NewReader(data))
	reqLine, err := reader.ReadString('\n')
	if err != nil {
		return metadata
	}

	reqLine = strings.TrimSpace(reqLine)
	parts := strings.Split(reqLine, " ")
	if len(parts) < 2 {
		return metadata
	}

	// 解析 Host header
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}

		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			host := strings.TrimSpace(line[5:])
			metadata.Host = host
			break
		}
	}

	return metadata
}

// peekQUIC 嗅探 QUIC 连接
func (s *Sniffer) peekQUIC(data []byte) *adapter.Metadata {
	// QUIC 连接暂时不嗅探 Host
	return nil
}

// getPortFromConfig 从配置获取端口
func (s *Sniffer) getPortFromConfig(protocol string) int {
	if s.cfg == nil {
		return 0
	}

	ports, ok := s.cfg.Sniff[protocol]
	if !ok {
		return 0
	}

	for range ports.Ports {
		// 简化实现，直接返回第一个端口
		return 0
	}

	return 0
}

// IsForceDomain 检查是否强制嗅探
func (s *Sniffer) IsForceDomain(host string) bool {
	if s.cfg == nil {
		return false
	}

	for _, domain := range s.cfg.ForceDomain {
		if matchDomain(domain, host) {
			return true
		}
	}
	return false
}

// IsSkipDomain 检查是否跳过嗅探
func (s *Sniffer) IsSkipDomain(host string) bool {
	if s.cfg == nil {
		return false
	}

	for _, domain := range s.cfg.SkipDomain {
		if matchDomain(domain, host) {
			return true
		}
	}
	return false
}

// matchDomain 域名匹配
func matchDomain(pattern, host string) bool {
	if strings.HasPrefix(pattern, "+.") {
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix) || host == suffix[1:]
	}
	return host == pattern
}

// domainRegex 域名正则
var domainRegex = regexp.MustCompile(`[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)+`)

// PeekConn 嗅探连接（静态方法）
func PeekConn(conn net.Conn) (*adapter.Metadata, []byte, error) {
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	n, err := conn.Read(buf)
	if err != nil {
		return nil, nil, err
	}
	conn.SetReadDeadline(time.Time{})

	data := buf[:n]

	var metadata *adapter.Metadata

	// 判断协议
	sniffer := &Sniffer{}
	if sniffer.isTLS(data) {
		metadata = sniffer.peekTLS(data)
	} else if sniffer.isHTTP(data) {
		metadata = sniffer.peekHTTP(data)
	}

	return metadata, data, nil
}

// _ = sync, context, regexp import
var _ = sync.Mutex{}
var _ = context.Background
var _ = regexp.MustCompile
