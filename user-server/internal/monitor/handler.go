package monitor

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/response"
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

// nodeHealthHandler 按渠道 × 节点聚合：响应时间(avg/p95) + 异常率（多渠道特性）。
func nodeHealthHandler(c *gin.Context) {
	nh, err := NodeHealthByChannel(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, gin.H{"nodes": nh, "window": nodeHealthWindow.String()}, "success")
}

// latencyHandler 按渠道端到端时延（上报接入 → 送达确认）。
func latencyHandler(c *gin.Context) {
	l, err := LifecycleLatencyByChannel(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Success(c, l, "success")
}

// lifecycleHandler 还原某会话/某轮的业务链路节点（入参/出参/响应时间/预期/异常 + 节点间时延）。
// 查询参数：conversation_id（会话级）或 trace_id（轮次级），可选 limit。
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
	// 前端按「扁平节点行」渲染：把每轮的次级节点拍平，节点已携带 trace_id/conversation_id。
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

// traceTreeHandler 取任意一条消息 / 单轮对话的完整链路明细（生命周期节点 + agent 多轮 + 多工具）。
// 查询参数：trace_id（轮次级）> msg_id（反查）> conversation_id（最近一轮）。
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
