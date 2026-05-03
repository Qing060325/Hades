// Package geodata GeoSite 数据加载器
package geodata

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DomainType 域名匹配类型
type DomainType int

const (
	DomainPlain  DomainType = 0
	DomainDomain DomainType = 1
	DomainFull   DomainType = 3
)

// DomainEntry 域名条目
type DomainEntry struct {
	Type    DomainType
	Value   string
}

// GeoSiteEntry GeoSite 分类条目
type GeoSiteEntry struct {
	CountryCode string
	Domains     []DomainEntry
}

var (
	siteData map[string][]DomainEntry
	dataMu   sync.RWMutex
	loaded   bool
)

// Init 初始化 GeoSite 数据
func Init(dir string) error {
	path := filepath.Join(dir, "geosite.dat")
	return load(path)
}

func load(path string) error {
	dataMu.Lock()
	defer dataMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read geosite %s: %w", path, err)
	}

	// 解析 protobuf 格式的 geosite.dat
	parsed, err := parseGeositeData(data)
	if err != nil {
		return fmt.Errorf("parse geosite: %w", err)
	}
	siteData = parsed
	loaded = true
	return nil
}

// parseGeositeData 解析 geosite.dat (protobuf 格式)
// GeoSite protobuf 结构:
// message GeoSite {
//   repeated GeoSiteEntry entry = 1;
// }
// message GeoSiteEntry {
//   string country_code = 1;
//   repeated Domain domain = 2;
// }
// message Domain {
//   DomainType type = 1;
//   string value = 2;
//   repeated string attribute = 3;
// }
func parseGeositeData(data []byte) (map[string][]DomainEntry, error) {
	result := make(map[string][]DomainEntry)

	pos := 0
	for pos < len(data) {
		// 读取 field tag
		if pos >= len(data) {
			break
		}
		tag, newPos := readVarint(data, pos)
		pos = newPos

		fieldNumber := tag >> 3
		wireType := tag & 0x7

		if fieldNumber == 1 && wireType == 2 { // GeoSiteEntry (length-delimited)
			if pos >= len(data) {
				break
			}
			length, newPos := readVarint(data, pos)
			pos = newPos

			if pos+int(length) > len(data) {
				break
			}
			entryData := data[pos : pos+int(length)]
			pos += int(length)

			// 解析 GeoSiteEntry
			entry, err := parseGeoSiteEntry(entryData)
			if err != nil {
				continue // 跳过无法解析的条目
			}
			if entry.CountryCode != "" {
				result[entry.CountryCode] = entry.Domains
			}
		} else {
			// 跳过未知字段
			pos = skipField(data, pos, wireType)
			if pos < 0 {
				break
			}
		}
	}

	return result, nil
}

// parseGeoSiteEntry 解析单个 GeoSiteEntry
func parseGeoSiteEntry(data []byte) (*GeoSiteEntry, error) {
	entry := &GeoSiteEntry{}

	pos := 0
	for pos < len(data) {
		if pos >= len(data) {
			break
		}
		tag, newPos := readVarint(data, pos)
		pos = newPos

		fieldNumber := tag >> 3
		wireType := tag & 0x7

		switch {
		case fieldNumber == 1 && wireType == 2: // country_code (string)
			if pos >= len(data) {
				return entry, nil
			}
			length, newPos := readVarint(data, pos)
			pos = newPos
			if pos+int(length) > len(data) {
				return entry, nil
			}
			entry.CountryCode = string(data[pos : pos+int(length)])
			pos += int(length)

		case fieldNumber == 2 && wireType == 2: // domain (length-delimited)
			if pos >= len(data) {
				return entry, nil
			}
			length, newPos := readVarint(data, pos)
			pos = newPos
			if pos+int(length) > len(data) {
				return entry, nil
			}
			domainData := data[pos : pos+int(length)]
			pos += int(length)

			domain, err := parseDomainEntry(domainData)
			if err == nil {
				entry.Domains = append(entry.Domains, *domain)
			}

		default:
			pos = skipField(data, pos, wireType)
			if pos < 0 {
				return entry, nil
			}
		}
	}

	return entry, nil
}

// parseDomainEntry 解析单个 Domain 条目
func parseDomainEntry(data []byte) (*DomainEntry, error) {
	domain := &DomainEntry{}

	pos := 0
	for pos < len(data) {
		if pos >= len(data) {
			break
		}
		tag, newPos := readVarint(data, pos)
		pos = newPos

		fieldNumber := tag >> 3
		wireType := tag & 0x7

		switch {
		case fieldNumber == 1 && wireType == 0: // type (varint enum)
			if pos >= len(data) {
				return domain, nil
			}
			val, newPos := readVarint(data, pos)
			pos = newPos
			domain.Type = DomainType(val)

		case fieldNumber == 2 && wireType == 2: // value (string)
			if pos >= len(data) {
				return domain, nil
			}
			length, newPos := readVarint(data, pos)
			pos = newPos
			if pos+int(length) > len(data) {
				return domain, nil
			}
			domain.Value = string(data[pos : pos+int(length)])
			pos += int(length)

		default:
			pos = skipField(data, pos, wireType)
			if pos < 0 {
				return domain, nil
			}
		}
	}

	return domain, nil
}

// readVarint 读取 protobuf varint
func readVarint(data []byte, pos int) (uint64, int) {
	var result uint64
	var shift uint
	for pos < len(data) {
		b := data[pos]
		pos++
		result |= uint64(b&0x7F) << shift
		if b < 0x80 {
			return result, pos
		}
		shift += 7
		if shift >= 64 {
			return 0, pos
		}
	}
	return result, pos
}

// skipField 跳过未知字段
func skipField(data []byte, pos int, wireType int) int {
	switch wireType {
	case 0: // varint
		for pos < len(data) {
			b := data[pos]
			pos++
			if b < 0x80 {
				return pos
			}
		}
		return pos
	case 1: // 64-bit
		return pos + 8
	case 2: // length-delimited
		if pos >= len(data) {
			return -1
		}
		length, newPos := readVarint(data, pos)
		return newPos + int(length)
	case 5: // 32-bit
		return pos + 4
	default:
		return -1
	}
}

// LookupGeoSite 查询域名是否属于指定分类
func LookupGeoSite(domain string, country string) bool {
	dataMu.RLock()
	defer dataMu.RUnlock()

	if !loaded {
		return false
	}

	entries, ok := siteData[country]
	if !ok {
		return false
	}

	domain = strings.ToLower(domain)
	for _, entry := range entries {
		if matchDomain(domain, entry) {
			return true
		}
	}
	return false
}

func matchDomain(domain string, entry DomainEntry) bool {
	value := strings.ToLower(entry.Value)
	switch entry.Type {
	case DomainPlain:
		return domain == value || strings.HasSuffix(domain, "."+value)
	case DomainDomain:
		return domain == value || strings.HasSuffix(domain, "."+value)
	case DomainFull:
		return domain == value
	}
	return false
}

// Download 下载 GeoSite 数据文件
func Download(dir string) error {
	url := "https://cdn.jsdelivr.net/gh/Loyalsoldier/v2ray-rules@release/geosite.dat"
	path := filepath.Join(dir, "geosite.dat")

	if _, err := os.Stat(path); err == nil {
		return nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download geosite: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Close 清理
func Close() {
	dataMu.Lock()
	defer dataMu.Unlock()
	siteData = nil
	loaded = false
}
