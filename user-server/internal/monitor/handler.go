package monitor

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 在已有路由组（已挂 InitGuard 私域鉴权）上注册监控端点。
// 私域部署，沿用 bridge 的鉴权模型：无需前端 JWT，账号以 channel+account_id 自证。
//
// 注意：UI 已迁至 user-web 前端（src/views/system/TraceMonitor.vue），
// 本包仅暴露 JSON 数据接口，由前端调用渲染。
func RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/monitor/health", healthHandler)
	rg.GET("/monitor/anomalies", anomaliesHandler)
	rg.GET("/monitor/node-health", nodeHealthHandler)
	rg.GET("/monitor/latency", latencyHandler)
	rg.GET("/monitor/lifecycle", lifecycleHandler)
	rg.GET("/monitor/traces", tracesHandler)
	rg.GET("/monitor/trace-tree", traceTreeHandler)
}

func healthHandler(c *gin.Context) {
	h, err := HealthOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, h, "success")
}

func anomaliesHandler(c *gin.Context) {
	a, err := Anomalies(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, a, "success")
}

func nodeHealthHandler(c *gin.Context) {
	nh, err := NodeHealthByChannel(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, gin.H{"nodes": nh, "window": nodeHealthWindow.String()}, "success")
}

func latencyHandler(c *gin.Context) {
	l, err := LifecycleLatencyByChannel(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, l, "success")
}

func lifecycleHandler(c *gin.Context) {
	conv := c.Query("conversation_id")
	tid := c.Query("trace_id")
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	lc, err := Lifecycle(c.Request.Context(), conv, tid, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	flat := make([]LifecycleNode, 0)
	for _, round := range lc {
		flat = append(flat, round.Nodes...)
	}
	response.Success(c, flat, "success")
}

func tracesHandler(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	ts, err := Traces(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, ts, "success")
}

func traceTreeHandler(c *gin.Context) {
	tid := c.Query("trace_id")
	conv := c.Query("conversation_id")
	msg := c.Query("msg_id")
	tree, err := TraceTree(c.Request.Context(), tid, conv, msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, tree, "success")
}
