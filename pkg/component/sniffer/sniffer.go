// Package sniffer 流量嗅探器
package sniffer

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// Protocol 嗅探协议类型
type Protocol string

const (
	ProtocolTLS  Protocol = "tls"
	ProtocolHTTP Protocol = "http"
	ProtocolQUIC Protocol = "quic"
	ProtocolSTUN Protocol = "stun"
	ProtocolSSH  Protocol = "ssh"
)

// Result 嗅探结果
type Result struct {
	Protocol Protocol
	Domain   string
	ALPN     []string
}

// Sniffer 嗅探器
type Sniffer struct {
	protocols map[Protocol]bool
}

// New 创建嗅探器
func New(protocols []Protocol) *Sniffer {
	s := &Sniffer{protocols: make(map[Protocol]bool)}
	for _, p := range protocols {
		s.protocols[p] = true
	}
	if len(protocols) == 0 {
		s.protocols[ProtocolTLS] = true
		s.protocols[ProtocolHTTP] = true
		s.protocols[ProtocolQUIC] = true
	}
	return s
}

// Sniff 嗅探数据
func (s *Sniffer) Sniff(data []byte) *Result {
	if len(data) < 2 {
		return nil
	}
	if s.protocols[ProtocolTLS] && data[0] == 0x16 && data[1] == 0x03 {
		return sniffTLS(data)
	}
	if s.protocols[ProtocolHTTP] && isHTTP(data) {
		return sniffHTTP(data)
	}
	if s.protocols[ProtocolQUIC] && isQUIC(data) {
		return sniffQUIC(data)
	}
	if s.protocols[ProtocolSSH] && len(data) >= 4 && string(data[:4]) == "SSH-" {
		return &Result{Protocol: ProtocolSSH}
	}
	return nil
}

func isHTTP(data []byte) bool {
	prefixes := []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "CONNECT ", "PATCH "}
	s := string(data)
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func sniffHTTP(data []byte) *Result {
	lines := strings.SplitN(string(data), "\r\n", 10)
	for _, line := range lines[1:] {
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			return &Result{Protocol: ProtocolHTTP, Domain: strings.TrimSpace(line[5:])}
		}
	}
	return nil
}

func sniffTLS(data []byte) *Result {
	if len(data) < 5 {
		return nil
	}
	// TLS Handshake
	if data[0] != 0x16 {
		return nil
	}
	// 解析 ClientHello 寻找 SNI
	domain := parseSNIFromTLS(data)
	if domain == "" {
		return nil
	}
	return &Result{Protocol: ProtocolTLS, Domain: domain}
}

func parseSNIFromTLS(data []byte) string {
	if len(data) < 43 {
		return ""
	}
	// TLS Record Header: 1(content) + 2(version) + 2(length)
	// Handshake: 1(type) + 3(length) + 2(client_version)
	pos := 43 // 跳到 Session ID Length
	if pos >= len(data) {
		return ""
	}
	sessionIDLen := int(data[pos])
	pos += 1 + sessionIDLen
	if pos+2 > len(data) {
		return ""
	}
	// Cipher Suites
	cipherLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + cipherLen
	if pos >= len(data) {
		return ""
	}
	// Compression Methods
	compLen := int(data[pos])
	pos += 1 + compLen
	if pos+2 > len(data) {
		return ""
	}
	// Extensions
	extLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	end := pos + extLen
	for pos+4 <= end && pos+4 <= len(data) {
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4
		if pos+extLen > len(data) {
			return ""
		}
		if extType == 0x0000 { // SNI
			return parseSNIExtension(data[pos : pos+extLen])
		}
		pos += extLen
	}
	return ""
}

func parseSNIExtension(data []byte) string {
	if len(data) < 5 {
		return ""
	}
	// Server Name List Length (2) + Server Name Type (1) + Server Name Length (2)
	nameLen := int(binary.BigEndian.Uint16(data[3:5]))
	if 5+nameLen > len(data) {
		return ""
	}
	return string(data[5 : 5+nameLen])
}

func isQUIC(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	// QUIC long header: first bit set, version follows
	if data[0]&0x80 != 0 {
		// Check for QUIC version
		if len(data) >= 13 {
			version := binary.BigEndian.Uint32(data[1:5])
			return version == 0x00000001 || version == 0x51303433 || version == 0x709A50C4
		}
	}
	return false
}

func sniffQUIC(data []byte) *Result {
	// 解析 QUIC ClientHello 中的 SNI
	// 这是简化实现
	if len(data) < 20 {
		return nil
	}
	// 尝试在数据中查找 SNI
	idx := bytes.Index(data, []byte{0x00, 0x00}) // SNI extension type
	if idx > 0 && idx+5 < len(data) {
		nameLen := int(binary.BigEndian.Uint16(data[idx+3 : idx+5]))
		if idx+5+nameLen <= len(data) {
			domain := string(data[idx+5 : idx+5+nameLen])
			if isValidDomain(domain) {
				return &Result{Protocol: ProtocolQUIC, Domain: domain}
			}
		}
	}
	return nil
}

func isSTUN(data []byte) bool {
	if len(data) < 20 {
		return false
	}
	// STUN magic cookie: 0x2112A442
	return binary.BigEndian.Uint32(data[4:8]) == 0x2112A442
}

func isValidDomain(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return strings.Contains(s, ".")
}
