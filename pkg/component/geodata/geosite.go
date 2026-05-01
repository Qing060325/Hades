// Package geodata GeoSite 数据加载器
package geodata

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	siteData, err = parseGeositeData(data)
	if err != nil {
		return fmt.Errorf("parse geosite: %w", err)
	}
	loaded = true
	return nil
}

// parseGeositeData 解析 geosite.dat (protobuf 格式)
func parseGeositeData(data []byte) (map[string][]DomainEntry, error) {
	// 简化的 protobuf 解析
	// 实际应使用 proto.Unmarshal，这里先提供基础实现
	result := make(map[string][]DomainEntry)
	_ = data // TODO: 实现完整 protobuf 解析
	return result, nil
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

	for _, entry := range entries {
		if matchDomain(domain, entry) {
			return true
		}
	}
	return false
}

func matchDomain(domain string, entry DomainEntry) bool {
	switch entry.Type {
	case DomainPlain:
		return domain == entry.Value || len(domain) > len(entry.Value) &&
			domain[len(domain)-len(entry.Value)-1:] == "."+entry.Value
	case DomainDomain:
		return domain == entry.Value || len(domain) > len(entry.Value) &&
			domain[len(domain)-len(entry.Value)-1:] == "."+entry.Value
	case DomainFull:
		return domain == entry.Value
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
