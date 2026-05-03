// Package stats Prometheus 指标收集器
package stats

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusCollector 将 stats.Manager 适配为 Prometheus Collector
type PrometheusCollector struct {
	manager *Manager

	uploadBytes     *prometheus.Desc
	downloadBytes   *prometheus.Desc
	activeConns     *prometheus.Desc
	totalConns      *prometheus.Desc
	proxyUpload     *prometheus.Desc
	proxyDownload   *prometheus.Desc
	proxyConns      *prometheus.Desc
}

// NewPrometheusCollector 创建 Prometheus 收集器
func NewPrometheusCollector(m *Manager) *PrometheusCollector {
	const subsystem = "hades"

	return &PrometheusCollector{
		manager: m,
		uploadBytes: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "upload_bytes_total"),
			"Total upload traffic in bytes",
			nil, nil,
		),
		downloadBytes: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "download_bytes_total"),
			"Total download traffic in bytes",
			nil, nil,
		),
		activeConns: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "active_connections"),
			"Number of currently active connections",
			nil, nil,
		),
		totalConns: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "connections_total"),
			"Total number of connections since startup",
			nil, nil,
		),
		proxyUpload: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "proxy_upload_bytes_total"),
			"Per-proxy upload traffic in bytes",
			[]string{"proxy"}, nil,
		),
		proxyDownload: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "proxy_download_bytes_total"),
			"Per-proxy download traffic in bytes",
			[]string{"proxy"}, nil,
		),
		proxyConns: prometheus.NewDesc(
			prometheus.BuildFQName("", subsystem, "proxy_connections"),
			"Per-proxy active connections",
			[]string{"proxy"}, nil,
		),
	}
}

// Describe 实现 prometheus.Collector 接口
func (c *PrometheusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.uploadBytes
	ch <- c.downloadBytes
	ch <- c.activeConns
	ch <- c.totalConns
	ch <- c.proxyUpload
	ch <- c.proxyDownload
	ch <- c.proxyConns
}

// Collect 实现 prometheus.Collector 接口
func (c *PrometheusCollector) Collect(ch chan<- prometheus.Metric) {
	traffic := c.manager.GetTraffic()

	ch <- prometheus.MustNewConstMetric(
		c.uploadBytes, prometheus.CounterValue, float64(traffic.Upload),
	)
	ch <- prometheus.MustNewConstMetric(
		c.downloadBytes, prometheus.CounterValue, float64(traffic.Download),
	)
	ch <- prometheus.MustNewConstMetric(
		c.activeConns, prometheus.GaugeValue, float64(c.manager.ActiveConnections()),
	)
	ch <- prometheus.MustNewConstMetric(
		c.totalConns, prometheus.CounterValue, float64(c.manager.TotalConnections()),
	)

	// 每代理统计
	allProxyStats := c.manager.AllProxyStats()
	for name, ps := range allProxyStats {
		ch <- prometheus.MustNewConstMetric(
			c.proxyUpload, prometheus.CounterValue, float64(ps.Upload), name,
		)
		ch <- prometheus.MustNewConstMetric(
			c.proxyDownload, prometheus.CounterValue, float64(ps.Download), name,
		)
		ch <- prometheus.MustNewConstMetric(
			c.proxyConns, prometheus.GaugeValue, float64(ps.Connections), name,
		)
	}
}
