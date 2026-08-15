// 指标 HTTP 中间件（2026-08-15 M3-P1-E3）
//
// 功能：
//   - 自动采集每个 HTTP 请求的耗时、状态码、路径、方法的指标
//   - 暴露 /metrics 端点供 Prometheus 抓取
//   - 默认排除 /metrics 自身（避免自抓取递归）
//
// 用法：
//   r.Use(middleware.MetricsMiddleware())
//   r.GET("/metrics", gin.WrapH(metrics.Handler()))
package middleware

import (
	"strconv"
	"time"

	"hivemtk-user/internal/pkg/metrics"

	"github.com/gin-gonic/gin"
)

// MetricsConfig 指标中间件配置
type MetricsConfig struct {
	ExcludePaths []string
	Namespace string
	BucketsHistogram []float64
}

// DefaultMetricsConfig 默认配置
var DefaultMetricsConfig = MetricsConfig{
	ExcludePaths: []string{
		"/metrics",
		"/healthz",
		"/api/health",
	},
	Namespace: "http",
	BucketsHistogram: []float64{
		1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000,
	},
}

var (
	httpRequestsTotal  *metrics.CounterVec
	httpRequestLatency *metrics.HistogramVec
	httpRequestsActive *metrics.GaugeVec
	metricsOnce        bool
)

func initHTTPMetrics(buckets []float64) {
	if metricsOnce {
		return
	}
	metricsOnce = true
	httpRequestsTotal = metrics.NewCounter(
		"http_requests_total",
		"Total HTTP requests by method, path, status",
		[]string{"method", "path", "status"},
	)
	httpRequestLatency = metrics.NewHistogram(
		"http_request_duration_ms",
		"HTTP request duration in ms",
		[]string{"method", "path"},
		buckets,
	)
	httpRequestsActive = metrics.NewGauge(
		"http_requests_in_flight",
		"Current in-flight HTTP requests",
		[]string{"method"},
	)
}

// normalizePath 路径归一化（避免高基数标签）
//
// 将 :id 这种动态段替换为 :param，避免每个 ID 创建一个时间序列
func normalizePath(c *gin.Context) string {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
		if len(path) > 64 {
			path = path[:64]
		}
	}
	return path
}

// MetricsMiddleware 指标采集中间件
func MetricsMiddleware(cfg ...MetricsConfig) gin.HandlerFunc {
	var c MetricsConfig
	if len(cfg) > 0 {
		c = cfg[0]
	} else {
		c = DefaultMetricsConfig
	}
	if len(c.BucketsHistogram) == 0 {
		c.BucketsHistogram = DefaultMetricsConfig.BucketsHistogram
	}
	initHTTPMetrics(c.BucketsHistogram)

	exclude := make(map[string]bool, len(c.ExcludePaths))
	for _, p := range c.ExcludePaths {
		exclude[p] = true
	}

	return func(ctx *gin.Context) {
		path := normalizePath(ctx)
		if exclude[path] {
			ctx.Next()
			return
		}

		method := ctx.Request.Method

		httpRequestsActive.WithLabel(method).Inc()
		defer httpRequestsActive.WithLabel(method).Dec()

		start := time.Now()
		ctx.Next()
		elapsed := time.Since(start)

		status := strconv.Itoa(ctx.Writer.Status())
		httpRequestsTotal.WithLabel(method, path, status).Inc()
		httpRequestLatency.WithLabel(method, path).Observe(float64(elapsed.Milliseconds()))
	}
}


