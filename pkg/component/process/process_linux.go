// Package process Linux 进程检测
package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cache 进程信息缓存
type Cache struct {
	mu    sync.RWMutex
	cache map[string]*Info
	ttl   time.Duration
}

// Info 进程信息
type Info struct {
	Name    string
	Path    string
	UID     int
	PID     int
	Updated time.Time
}

// NewCache 创建进程缓存
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		cache: make(map[string]*Info),
		ttl:   ttl,
	}
	go c.cleanup()
	return c
}

// GetBySocket 通过 socket 信息获取进程
func (c *Cache) GetBySocket(network string, srcIP string, srcPort uint16, dstIP string, dstPort uint16) *Info {
	key := fmt.Sprintf("%s:%s:%d", network, srcIP, srcPort)

	c.mu.RLock()
	if info, ok := c.cache[key]; ok && time.Since(info.Updated) < c.ttl {
		c.mu.RUnlock()
		return info
	}
	c.mu.RUnlock()

	// 从 /proc 查找
	info := findProcessBySocket(network, srcIP, srcPort)
	if info != nil {
		c.mu.Lock()
		c.cache[key] = info
		c.mu.Unlock()
	}
	return info
}

// GetByName 通过进程名查找
func (c *Cache) GetByName(name string) *Info {
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		parts := strings.Split(string(cmdline), "\x00")
		if len(parts) > 0 {
			exe := filepath.Base(parts[0])
			if exe == name {
				exePath, _ := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
				return &Info{
					Name:    exe,
					Path:    exePath,
					PID:     pid,
					Updated: time.Now(),
				}
			}
		}
	}
	return nil
}

func findProcessBySocket(network, srcIP string, srcPort uint16) *Info {
	// 从 /proc/net/tcp 或 /proc/net/udp 查找 inode
	inode := findInode(network, srcIP, srcPort)
	if inode == "" {
		return nil
	}

	// 从 /proc/*/fd 查找匹配的 inode
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		procInode := findProcInode(pid)
		if procInode == inode {
			cmdline, _ := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
			parts := strings.Split(string(cmdline), "\x00")
			exePath, _ := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
			name := ""
			if len(parts) > 0 {
				name = filepath.Base(parts[0])
			}
			return &Info{
				Name:    name,
				Path:    exePath,
				PID:     pid,
				Updated: time.Now(),
			}
		}
	}
	return nil
}

func findInode(network, srcIP string, srcPort uint16) string {
	var procFile string
	if network == "tcp" {
		procFile = "/proc/net/tcp"
	} else {
		procFile = "/proc/net/udp"
	}

	data, err := os.ReadFile(procFile)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// local_address is fields[1]
		addr := fields[1]
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			continue
		}
		port, _ := strconv.ParseUint(parts[1], 16, 16)
		if uint16(port) == srcPort {
			return fields[9] // inode
		}
	}
	return ""
}

func findProcInode(pid int) string {
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, _ := os.ReadDir(fdDir)
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(link, "socket:") {
			return strings.Trim(link, "socket:[]")
		}
	}
	return ""
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		for k, v := range c.cache {
			if time.Since(v.Updated) > c.ttl {
				delete(c.cache, k)
			}
		}
		c.mu.Unlock()
	}
}
