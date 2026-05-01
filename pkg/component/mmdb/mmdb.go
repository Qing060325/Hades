// Package mmdb MaxMind DB 读取器
package mmdb

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

var (
	defaultReader *maxminddb.Reader
	readerMu      sync.RWMutex
)

// Init 初始化 MMDB
func Init(path string) error {
	readerMu.Lock()
	defer readerMu.Unlock()

	if defaultReader != nil {
		defaultReader.Close()
	}

	reader, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("open mmdb %s: %w", path, err)
	}
	defaultReader = reader
	return nil
}

// LookupCountry 查询 IP 所属国家代码
func LookupCountry(addr netip.Addr) string {
	readerMu.RLock()
	defer readerMu.RUnlock()

	if defaultReader == nil {
		return ""
	}

	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"registered_country"`
	}

	ip := net.IP(addr.AsSlice())
	if err := defaultReader.Lookup(ip, &record); err != nil {
		return ""
	}
	if record.Country.ISOCode != "" {
		return record.Country.ISOCode
	}
	return record.RegisteredCountry.ISOCode
}

// LookupASN 查询 IP 所属 ASN
func LookupASN(addr netip.Addr) uint {
	readerMu.RLock()
	defer readerMu.RUnlock()

	if defaultReader == nil {
		return 0
	}

	var record struct {
		ASN uint `maxminddb:"autonomous_system_number"`
	}

	ip := net.IP(addr.AsSlice())
	if err := defaultReader.Lookup(ip, &record); err != nil {
		return 0
	}
	return record.ASN
}

// Download 下载 MMDB 文件
func Download(dir string) error {
	url := "https://cdn.jsdelivr.net/gh/Loyalsoldier/geoip@release/Country.mmdb"
	path := filepath.Join(dir, "Country.mmdb")

	if _, err := os.Stat(path); err == nil {
		return nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download mmdb: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Close 关闭 reader
func Close() {
	readerMu.Lock()
	defer readerMu.Unlock()
	if defaultReader != nil {
		defaultReader.Close()
		defaultReader = nil
	}
}
