package router

import (
	"context"
	"net/http"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/app"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)


// setupToolDebugRoutes 注册工具链调试与可观测 API 路由
//
// 调用方：router.Setup() 的 auth 路由组
func setupToolDebugRoutes(auth *gin.RouterGroup) {
	auth.GET("/agent/tools/list", handleToolList)
	auth.GET("/agent/tools/get", handleToolGet)
	auth.POST("/agent/tools/execute", handleToolExecute)
	auth.GET("/agent/tools/stats", handleToolStats)
	auth.GET("/agent/tools/audit", handleToolAudit)
	auth.GET("/agent/tools/cost", handleToolCost)
	auth.POST("/agent/tools/circuit/reset", handleToolCircuitReset)
	auth.GET("/agent/tools/providers", handleToolProviders)
}


// handleToolList 列出所有已注册工具
//
// 响应：
//
//	{
//	  "success": true,
//	  "total": 41,
//	  "tools": [
//	    {"name":"customer.search","category":"customer","description":"...","parameters":{...}}
//	  ]
//	}
func handleToolList(c *gin.Context) {
	registry := tooluse.GetGlobalRegistry()
	if registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "tool registry not initialized"})
		return
	}
	category := c.Query("category")
	var tools []tooluse.Tool
	if category != "" {
		tools = registry.ListByCategory(tooluse.ToolCategory(category))
	} else {
		tools = registry.List()
	}
	out := make([]gin.H, 0, len(tools))
	for _, t := range tools {
		out = append(out, gin.H{
			"name":        t.Name(),
			"category":    string(t.Category()),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"total":   len(out),
		"tools":   out,
	})
}


// handleToolGet 按 name 查询单个工具详情
//
// 查询参数：?name=customer.search
func handleToolGet(c *gin.Context) {
	registry := tooluse.GetGlobalRegistry()
	if registry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "tool registry not initialized"})
		return
	}
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "name query parameter required"})
		return
	}
	tool, err := registry.Get(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"tool": gin.H{
			"name":        tool.Name(),
			"category":    string(tool.Category()),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	})
}


// toolExecuteRequest 执行工具请求体
type toolExecuteRequest struct {
	ToolName string         `json:"tool_name" binding:"required"`
	Args     map[string]any `json:"args"`
	AgentID    string `json:"agent_id"`
	SessionID  string `json:"session_id"`
	CustomerID string `json:"customer_id"`
	Source     string `json:"source"` 
}

// handleToolExecute 执行单个工具
//
// 复用全局 ToolExecutor，走完整 8 层装饰器链
// （权限 → 限流 → 熔断 → 参数校验 → 重试 → 超时 → 审计 → 计费）
//
// 安全控制：
//   - 必须走 auth 路由组（已含 JWT 鉴权）
//   - 单次单工具调用，不暴露批量
//   - 全局 TokenBucket 限流（20 QPS / 突发 50）兜底
//   - 30s 超时兜底
func handleToolExecute(c *gin.Context) {
	exec := tooluse.GetGlobalExecutor()
	if exec == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "tool executor not initialized"})
		return
	}
	var req toolExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}
	if req.Args == nil {
		req.Args = map[string]any{}
	}
	toolCtx := &tooluse.ToolContext{
		AgentID:    req.AgentID,
		SessionID:  req.SessionID,
		CustomerID: req.CustomerID,
		Source:     req.Source,
		AuditTrace: "manual:" + time.Now().Format("20060102T150405"),
	}
	if toolCtx.Source == "" {
		toolCtx.Source = "manual"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()

	router := app.GetGlobalToolRouter()
	var routeResult tooluse.RouteResult
	if router != nil {
		routeResult = router.Route(ctx, req.ToolName, req.Args, toolCtx)
	} else {
		tr, execErr := exec.ExecuteByName(ctx, req.ToolName, req.Args)
		routeResult = tooluse.RouteResult{Result: tr, Err: execErr}
	}

	logger.Infof("[tool-debug] manual execute tool=%s success=%v duration_ms=%d circuit_open=%v caller=%s",
		req.ToolName, routeResult.Result.Success, routeResult.Result.Timing.DurationMs, routeResult.CircuitOpen, toolCtx.AuditTrace)

	respErr := routeResult.Result.Error
	if respErr == "" && routeResult.Err != nil {
		respErr = routeResult.Err.Error()
	}
	resp := gin.H{
		"success":     routeResult.Result.Success,
		"tool_result": routeResult.Result.Data,
		"error":       respErr,
	}
	if routeResult.CircuitOpen {
		resp["circuit_open"] = true
	}
	if routeResult.RateLimit {
		resp["rate_limited"] = true
	}
	c.JSON(http.StatusOK, resp)
}


// handleToolStats 获取 ToolRouter 全局统计
//
// 响应包含：
//   - router_stats: ToolRouter 统计（TotalCalls/SuccessCalls/FailedCalls/RateLimitedCalls/CircuitOpenCalls/TotalCost）
//   - registry_total: 注册中心工具总数
//   - executor_available: 当前可用工具数（排除 disabled）
func handleToolStats(c *gin.Context) {
	registry := tooluse.GetGlobalRegistry()
	exec := tooluse.GetGlobalExecutor()
	router := app.GetGlobalToolRouter()

	resp := gin.H{
		"success":            true,
		"registry_total":     0,
		"executor_available": 0,
	}
	if registry != nil {
		resp["registry_total"] = registry.Count()
	}
	if exec != nil {
		resp["executor_available"] = len(exec.ListAvailableTools())
	}
	if router != nil {
		resp["router_stats"] = router.GetStats()
	} else {
		resp["router_stats"] = nil
		resp["router_warning"] = "ToolRouter not initialized"
	}
	c.JSON(http.StatusOK, resp)
}


// handleToolAudit 获取最近 N 条审计日志
//
// 查询参数：
//
//	?limit=100  返回条数（默认 100，最大 1000）
//	?tool=xxx   按工具名过滤
func handleToolAudit(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := atoiSafe(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	toolName := c.Query("tool")

	exec := tooluse.GetGlobalExecutor()
	if exec == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "tool executor not initialized"})
		return
	}

	memLogger := app.GetGlobalMemoryAuditLogger()
	if memLogger == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"entries": []any{},
			"warning": "memory audit logger not accessible (may be replaced by DB-backed implementation)",
			"total":   0,
		})
		return
	}
	entries := memLogger.Entries()
	out := make([]tooluse.AuditEntry, 0, limit)
	for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := entries[i]
		if toolName != "" && e.ToolName != toolName {
			continue
		}
		out = append(out, e)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"total":   len(out),
		"entries": out,
	})
}


// handleToolCost 获取计费统计
func handleToolCost(c *gin.Context) {
	memTracker := app.GetGlobalMemoryCostTracker()
	if memTracker == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"stats":   []any{},
			"warning": "memory cost tracker not accessible",
			"total":   0,
		})
		return
	}
	stats := memTracker.Stats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"total":   len(stats),
		"stats":   stats,
	})
}


// toolCircuitResetRequest 重置熔断请求体
type toolCircuitResetRequest struct {
	ToolName string `json:"tool_name" binding:"required"`
}

// handleToolCircuitReset 重置指定工具的熔断状态
//
// 当某工具因连续失败被熔断后，运维可手动调用此端点立即解除熔断
func handleToolCircuitReset(c *gin.Context) {
	router := app.GetGlobalToolRouter()
	if router == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "tool router not initialized"})
		return
	}
	var req toolCircuitResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.ToolName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "tool_name required"})
		return
	}
	router.ResetCircuit(req.ToolName)
	logger.Infof("[tool-debug] circuit reset tool=%s by caller=%s", req.ToolName, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"tool_name": req.ToolName,
		"message":   "circuit breaker reset",
	})
}


// atoiSafe 安全字符串转 int
func atoiSafe(s string) (int, error) {
	if s == "" {
		return 0, errInvalidInteger
	}
	var n int
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errInvalidInteger
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

// errInvalidInteger 非法整数错误（避免引入 strconv 包开销）
var errInvalidInteger = &simpleError{"invalid integer"}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }


// handleToolProviders 列出所有 ToolProvider 及其装配状态
//
// 响应：
//
//	{
//	  "success": true,
//	  "total_providers": 5,
//	  "results": [
//	    {
//	      "provider_name": "reach",
//	      "category": "reach",
//	      "description": "多渠道触达工具（...）",
//	      "registered_tools": ["reach.sms.send", ...],
//	      "tool_count": 20,
//	      "skipped": false,
//	      "duration_ms": 12000000
//	    }
//	  ]
//	}
func handleToolProviders(c *gin.Context) {
	if app.GetGlobalProviderRegistry() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "provider registry not initialized",
		})
		return
	}
	results := app.GetGlobalProviderRegistry().Results()
	providers := app.GetGlobalProviderRegistry().ListProviders()

	providerInfo := make([]gin.H, 0, len(providers))
	resultMap := make(map[string]tooluse.ProviderRegistrationResult, len(results))
	for _, r := range results {
		resultMap[r.ProviderName] = r
	}

	for _, p := range providers {
		r, hasResult := resultMap[p.Name()]
		info := gin.H{
			"provider_name": p.Name(),
			"category":      string(p.Category()),
			"description":   p.Description(),
		}
		if hasResult {
			info["registered_tools"] = r.RegisteredTools
			info["skipped_tools"] = r.SkippedTools
			info["tool_count"] = r.ToolCount
			info["skipped"] = r.Skipped
			info["skipped_reason"] = r.SkippedReason
			info["err"] = r.Err
			info["duration_ms"] = r.Duration.Milliseconds()
		} else {
			info["registered_tools"] = []string{}
			info["tool_count"] = 0
			info["note"] = "no registration result (registered after last RegisterAll)"
		}
		providerInfo = append(providerInfo, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"total_providers":  len(providers),
		"registered_count": app.GetGlobalProviderRegistry().Count(),
		"results":          providerInfo,
	})
}

