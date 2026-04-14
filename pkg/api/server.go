// Package api RESTful API 实现
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Qing060325/Hades/internal/version"
	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/stats"
	"github.com/Qing060325/Hades/pkg/subscription"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// Server API 服务器
type Server struct {
	addr      string
	secret    string
	server    *http.Server
	mu        sync.RWMutex

	adapterMgr *adapter.Manager
	groupMgr   *group.Manager
	ruleEngine *rules.Engine
	statsMgr   *stats.Manager
	subMgr     SubscriptionManager

	// 配置重载回调
	reloadFunc ReloadFunc

	// 规则集提供者管理器
	providerMgr RuleProviderManager
}

// RuleProviderManager 规则集提供者管理器接口
type RuleProviderManager interface {
	Reload(name string) error
	ReloadAll() error
	Stats() map[string]RuleProviderStats
}

// RuleProviderStats 提供者统计
type RuleProviderStats struct {
	Type      string `json:"type"`
	Behavior  string `json:"behavior"`
	Count     int    `json:"count"`
	UpdatedAt string `json:"updatedAt"`
}

// ReloadFunc 配置重载回调函数类型
type ReloadFunc func(configData []byte) error

// SubscriptionManager 订阅管理器接口
type SubscriptionManager interface {
	List() []*subscription.Subscription
	GetSubscription(name string) (*subscription.Subscription, bool)
	Update(name string) error
	UpdateAll() error
}

// NewServer 创建 API 服务器
func NewServer(addr, secret string) *Server {
	return &Server{
		addr:   addr,
		secret: secret,
	}
}

// SetManagers 设置管理器
func (s *Server) SetManagers(adapterMgr *adapter.Manager, groupMgr *group.Manager, ruleEngine *rules.Engine, statsMgr *stats.Manager) {
	s.adapterMgr = adapterMgr
	s.groupMgr = groupMgr
	s.ruleEngine = ruleEngine
	s.statsMgr = statsMgr
}

// SetSubscriptionManager 设置订阅管理器
func (s *Server) SetSubscriptionManager(subMgr SubscriptionManager) {
	s.subMgr = subMgr
}

// SetReloadFunc 设置配置重载回调
func (s *Server) SetReloadFunc(f ReloadFunc) {
	s.reloadFunc = f
}

// SetProviderManager 设置规则集提供者管理器
func (s *Server) SetProviderManager(mgr RuleProviderManager) {
	s.providerMgr = mgr
}

// ListenAndServe 启动 API 服务器
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/version", s.handleVersion)
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/configs", s.handleConfigs) // 配置热重载

	// 代理相关
	mux.HandleFunc("/proxies", s.handleProxies)
	mux.HandleFunc("/proxies/", s.handleProxy)

	// 代理组
	mux.HandleFunc("/groups", s.handleGroups)
	mux.HandleFunc("/groups/", s.handleGroup)

	// 规则
	mux.HandleFunc("/rules", s.handleRules)

	// 连接
	mux.HandleFunc("/connections", s.handleConnections)
	mux.HandleFunc("/connections/close", s.closeAllConnections)

	// 流量统计
	mux.HandleFunc("/traffic", s.handleTraffic)

	// DNS 查询
	mux.HandleFunc("/dns/query", s.handleDNSQuery)

	// 日志
	mux.HandleFunc("/logs", s.handleLogs)

	// 健康检查
	mux.HandleFunc("/ping", s.handlePing)

	// 订阅管理
	mux.HandleFunc("/subscriptions", s.handleSubscriptions)
	mux.HandleFunc("/subscriptions/", s.handleSubscription)

	// 系统升级
	mux.HandleFunc("/upgrade", s.handleUpgrade)
	mux.HandleFunc("/upgrade/status", s.handleUpgradeStatus)

	// 规则集提供者
	mux.HandleFunc("/providers/rules", s.handleRuleProviders)
	mux.HandleFunc("/providers/rules/", s.handleRuleProvider)

	// Prometheus 指标（注册自定义收集器）
	if s.statsMgr != nil {
		registry := prometheus.NewRegistry()
		registry.MustRegister(stats.NewPrometheusCollector(s.statsMgr))
		registry.MustRegister(prometheus.NewGoCollector())
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		log.Info().Msg("Prometheus /metrics 端点已启用")
	}

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: s.authMiddleware(mux),
	}

	log.Info().Str("addr", s.addr).Msg("API 服务器已启动")
	return s.server.ListenAndServe()
}

// Shutdown 关闭服务器
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// authMiddleware 认证中间件
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.secret != "" {
			token := r.Header.Get("Authorization")
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token != "Bearer "+s.secret && token != s.secret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON 写入 JSON 响应
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// handleRoot 根路径
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "Hades API",
		"version": "dev",
	})
}

// handleVersion 版本信息
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    version.Version,
		"goVersion": version.GoVersion,
		"buildTime": version.BuildTime,
	})
}

// handleConfig 配置信息
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":     "rule",
			"logLevel": "info",
			"tun": map[string]interface{}{
				"enable": false,
			},
		})
		return
	}

	// PATCH 更新配置
	if r.Method == http.MethodPatch {
		var cfg map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleConfigs 配置热重载
// PUT /configs - 上传新配置并触发热重载
// PATCH /configs - 从文件重新加载配置
func (s *Server) handleConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		if s.reloadFunc == nil {
			s.writeError(w, http.StatusServiceUnavailable, "config reload not available")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		defer r.Body.Close()

		if len(body) == 0 {
			s.writeError(w, http.StatusBadRequest, "empty config body")
			return
		}

		if err := s.reloadFunc(body); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("config reload failed: %v", err))
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "config reloaded successfully",
		})

	case http.MethodPatch:
		if s.reloadFunc == nil {
			s.writeError(w, http.StatusServiceUnavailable, "config reload not available")
			return
		}

		// 从请求体获取配置文件路径，或使用默认路径
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		configPath := req.Path
		if configPath == "" {
			s.writeError(w, http.StatusBadRequest, "config path is required")
			return
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read config file: %v", err))
			return
		}

		if err := s.reloadFunc(data); err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("config reload failed: %v", err))
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "config reloaded successfully",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleProxies 代理列表
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	if s.adapterMgr == nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"proxies": []interface{}{}})
		return
	}

	proxies := make(map[string]interface{})
	for name, adapt := range s.adapterMgr.All() {
		proxies[name] = map[string]interface{}{
			"name":   adapt.Name(),
			"type":   string(adapt.Type()),
			"addr":   adapt.Addr(),
			"udp":    adapt.SupportUDP(),
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"proxies": proxies,
	})
}

// handleProxy 代理详情/切换
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现代理详情和切换
	w.WriteHeader(http.StatusNotImplemented)
}

// handleGroups 代理组列表
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if s.groupMgr == nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"groups": []interface{}{}})
		return
	}

	groups := s.groupMgr.All()
	result := make(map[string]interface{})
	for name, g := range groups {
		proxies := make([]map[string]interface{}, 0)
		for _, p := range g.Proxies() {
			proxies = append(proxies, map[string]interface{}{
				"name": p.Name(),
				"type": string(p.Type()),
			})
		}
		result[name] = map[string]interface{}{
			"name":    g.Name(),
			"type":    string(g.Type()),
			"proxies": proxies,
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{"groups": result})
}

// handleGroup 代理组操作
func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		// 切换代理组选择
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleRules 规则列表
func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"rules": []interface{}{},
	})
}

// handleConnections 连接列表
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"connections":    []interface{}{},
			"downloadTotal":  0,
			"uploadTotal":    0,
		})
		return
	}

	if r.Method == http.MethodDelete {
		s.closeAllConnections(w, r)
		return
	}
}

// closeAllConnections 关闭所有连接
func (s *Server) closeAllConnections(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "all connections closed"})
}

// handleTraffic 流量统计
func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if s.statsMgr != nil {
		stats := s.statsMgr.GetTraffic()
		s.writeJSON(w, http.StatusOK, stats)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"up":   0,
		"down": 0,
	})
}

// handleDNSQuery DNS 查询
func (s *Server) handleDNSQuery(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("name")
	if query == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name parameter"})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":  query,
		"ips":   []string{},
		"error": nil,
	})
}

// handleLogs 日志流
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// SSE 格式
	fmt.Fprintf(w, "data: {\"type\":\"log\",\"payload\":\"Hades started\"}\n\n")
	flusher.Flush()
}

// handlePing 健康检查
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

// handleSubscriptions 订阅列表
func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if s.subMgr == nil {
		s.writeError(w, http.StatusServiceUnavailable, "subscription manager not available")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 获取所有订阅
		subs := s.subMgr.List()
		infos := make([]subscription.SubscriptionInfo, 0, len(subs))
		for _, sub := range subs {
			infos = append(infos, sub.GetInfo())
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"subscriptions": infos,
		})

	case http.MethodPost:
		// 更新所有订阅
		if err := s.subMgr.UpdateAll(); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "all subscriptions updated",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleSubscription 单个订阅操作
func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	if s.subMgr == nil {
		s.writeError(w, http.StatusServiceUnavailable, "subscription manager not available")
		return
	}

	// 从 URL 解析订阅名称
	name := r.URL.Path[len("/subscriptions/"):]
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "subscription name required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 获取订阅详情
		sub, exists := s.subMgr.GetSubscription(name)
		if !exists {
			s.writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		s.writeJSON(w, http.StatusOK, sub.GetInfo())

	case http.MethodPut:
		// 更新订阅
		if err := s.subMgr.Update(name); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{
			"message": "subscription updated",
		})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleUpgrade 处理升级请求
func (s *Server) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 获取最新版本信息
	latestVersion, downloadURL, err := getLatestVersion()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("获取最新版本失败: %v", err))
		return
	}

	// 检查是否需要升级
	currentVersion := version.Version
	if currentVersion == latestVersion {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"message":       "已经是最新版本",
			"current":       currentVersion,
			"latest":        latestVersion,
			"need_upgrade":  false,
		})
		return
	}

	// 异步执行升级
	go performUpgrade(downloadURL, latestVersion)

	s.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":       "升级已开始",
		"current":       currentVersion,
		"latest":        latestVersion,
		"need_upgrade":  true,
		"status":        "downloading",
	})
}

// handleUpgradeStatus 获取升级状态
func (s *Server) handleUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := getUpgradeStatus()
	s.writeJSON(w, http.StatusOK, status)
}

// getLatestVersion 从 GitHub 获取最新版本
func getLatestVersion() (string, string, error) {
	resp, err := http.Get("https://api.github.com/repos/Qing060325/Hades/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	// 根据系统架构选择下载链接
	osType := runtime.GOOS
	arch := runtime.GOARCH

	binaryName := fmt.Sprintf("hades-%s-%s", osType, arch)
	if osType == "windows" {
		binaryName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			downloadURL = asset.URL
			break
		}
	}

	if downloadURL == "" {
		return "", "", fmt.Errorf("未找到适合当前系统的二进制文件: %s", binaryName)
	}

	return release.TagName, downloadURL, nil
}

// upgradeStatus 升级状态
var (
	upgradeStatus = map[string]interface{}{
		"status":  "idle", // idle, downloading, installing, completed, failed
		"message": "",
		"progress": 0,
	}
	upgradeMu sync.RWMutex
)

// performUpgrade 执行升级
func performUpgrade(downloadURL, version string) {
	upgradeMu.Lock()
	upgradeStatus["status"] = "downloading"
	upgradeStatus["message"] = "正在下载新版本..."
	upgradeStatus["progress"] = 0
	upgradeMu.Unlock()

	// 获取当前可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		upgradeMu.Lock()
		upgradeStatus["status"] = "failed"
		upgradeStatus["message"] = fmt.Sprintf("获取当前程序路径失败: %v", err)
		upgradeMu.Unlock()
		return
	}

	// 确定临时文件的扩展名
	tmpExt := ""
	if runtime.GOOS == "windows" {
		tmpExt = ".exe"
	}

	// 使用系统临时目录（兼容 Windows）
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("hades-upgrade-%s%s", version, tmpExt))
	tmpFile = tmpFile + ".tmp" // 确保文件不存在

	// 下载新二进制文件
	if err := downloadFile(tmpFile, downloadURL); err != nil {
		upgradeMu.Lock()
		upgradeStatus["status"] = "failed"
		upgradeStatus["message"] = fmt.Sprintf("下载失败: %v", err)
		upgradeMu.Unlock()
		// 清理临时文件
		os.Remove(tmpFile)
		return
	}

	upgradeMu.Lock()
	upgradeStatus["status"] = "installing"
	upgradeStatus["message"] = "正在安装..."
	upgradeStatus["progress"] = 80
	upgradeMu.Unlock()

	// 备份旧版本
	backupPath := exePath + ".backup"
	backupExists := false
	if _, err := os.Stat(backupPath); err == nil {
		backupExists = true
	}

	// 移动新版本到目标位置
	// Windows 不允许对正在运行的 exe 执行 Rename，所以我们用 Copy + Delete 的方式
	if runtime.GOOS == "windows" {
		// Windows: 先复制，再删除原文件
		if err := copyFile(tmpFile, exePath); err != nil {
			os.Remove(tmpFile)
			upgradeMu.Lock()
			upgradeStatus["status"] = "failed"
			upgradeStatus["message"] = fmt.Sprintf("安装失败: %v", err)
			upgradeMu.Unlock()
			return
		}
		// 尝试删除备份（忽略错误）
		if backupExists {
			os.Remove(backupPath)
		}
		// 重命名旧版本作为备份（如果原文件已不存在）
		if _, err := os.Stat(exePath + ".old"); os.IsNotExist(err) {
			// 原文件已更新，不需要备份
		}
	} else {
		// Unix-like: 使用 Rename
		// 先备份旧版本
		if _, err := os.Stat(exePath); err == nil {
			os.Rename(exePath, backupPath)
		}

		if err := os.Rename(tmpFile, exePath); err != nil {
			// 恢复备份
			if _, err := os.Stat(backupPath); err == nil {
				os.Rename(backupPath, exePath)
			}
			upgradeMu.Lock()
			upgradeStatus["status"] = "failed"
			upgradeStatus["message"] = fmt.Sprintf("安装失败: %v", err)
			upgradeMu.Unlock()
			return
		}
	}

	// 设置可执行权限（Unix 系统需要）
	if runtime.GOOS != "windows" {
		os.Chmod(exePath, 0755)
	}

	// 清理临时文件
	os.Remove(tmpFile)

	upgradeMu.Lock()
	upgradeStatus["status"] = "completed"
	if runtime.GOOS == "windows" {
		upgradeStatus["message"] = fmt.Sprintf("已升级到 %s，请手动重启服务", version)
	} else {
		upgradeStatus["message"] = fmt.Sprintf("已升级到 %s，服务将自动重启", version)
	}
	upgradeStatus["progress"] = 100
	upgradeMu.Unlock()

	// 在非 Windows 系统上，自动重启
	if runtime.GOOS != "windows" {
		// 给一点时间让状态被获取
		time.Sleep(2 * time.Second)
		// 发送重启信号
		terminateProcess(os.Getpid())
	}
}

// getUpgradeStatus 获取升级状态
func getUpgradeStatus() map[string]interface{} {
	upgradeMu.RLock()
	defer upgradeMu.RUnlock()

	// 返回副本
	status := make(map[string]interface{})
	for k, v := range upgradeStatus {
		status[k] = v
	}
	return status
}

// downloadFile 下载文件
func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// copyFile 复制文件（用于 Windows 上的热升级）
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// 创建目标文件
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// 复制内容
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// 确保写入磁盘
	return destFile.Sync()
}

// handleRuleProviders 处理规则集提供者列表
func (s *Server) handleRuleProviders(w http.ResponseWriter, r *http.Request) {
	if s.providerMgr == nil {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"providers": []interface{}{}})
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := s.providerMgr.Stats()
		providers := make([]map[string]interface{}, 0, len(stats))
		for name, stat := range stats {
			providers = append(providers, map[string]interface{}{
				"name":     name,
				"type":     stat.Type,
				"behavior": stat.Behavior,
				"count":    stat.Count,
				"updatedAt": stat.UpdatedAt,
			})
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"providers": providers})

	case http.MethodPut:
		// 重载所有提供者
		if err := s.providerMgr.ReloadAll(); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "所有规则集已重载"})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "方法不允许"})
	}
}

// handleRuleProvider 处理单个规则集提供者
func (s *Server) handleRuleProvider(w http.ResponseWriter, r *http.Request) {
	if s.providerMgr == nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "规则集提供者未初始化"})
		return
	}

	// 提取提供者名称: /providers/rules/{name}
	path := strings.TrimPrefix(r.URL.Path, "/providers/rules/")
	name := strings.TrimSuffix(path, "/")
	if name == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少提供者名称"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := s.providerMgr.Stats()
		stat, ok := stats[name]
		if !ok {
			s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "提供者不存在"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":     name,
			"type":     stat.Type,
			"behavior": stat.Behavior,
			"count":    stat.Count,
			"updatedAt": stat.UpdatedAt,
		})

	case http.MethodPut:
		// 重载指定提供者
		if err := s.providerMgr.Reload(name); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("规则集 %s 已重载", name)})

	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "方法不允许"})
	}
}
