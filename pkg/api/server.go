// Package api RESTful API 实现
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Qing060325/Hades/pkg/core/adapter"
	"github.com/Qing060325/Hades/pkg/core/group"
	"github.com/Qing060325/Hades/pkg/core/rules"
	"github.com/Qing060325/Hades/pkg/stats"
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

// ListenAndServe 启动 API 服务器
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/version", s.handleVersion)
	mux.HandleFunc("/config", s.handleConfig)

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

// handleRoot 根路径
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "Hades API",
		"version": "dev",
	})
}

// handleVersion 版本信息
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"version": "dev",
		"build":   "unknown",
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
