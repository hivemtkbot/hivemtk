package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"
)

// ============================================================================
// LLMRoutingController（2026-07-23 重构）
// ----------------------------------------------------------------------------
// 修复缺陷：
//  1. 9 处 context.Background() 全部改用 ctx.Request.Context()（五层架构 ctx 透传）
//  2. URL 路径 :id 与 provider name 混淆 → 改用 :name 路径参数
//  3. TestModel 不再走 Dispatch 临时改写全局路由（已移至 Service.CallProviderForTest）
// ============================================================================

// LLMRoutingController LLM 路由控制器
type LLMRoutingController struct {
	routingService *service.LLMRoutingService
}

// NewLLMRoutingController 创建 LLM 路由控制器
func NewLLMRoutingController(routingService *service.LLMRoutingService) *LLMRoutingController {
	return &LLMRoutingController{routingService: routingService}
}

// ============================================================================
// Provider / Model 管理
// ============================================================================

// ListModels 列出所有 LLM provider
//
// GET /api/llm/models
// 返回：[]LLMProviderInfo
func (c *LLMRoutingController) ListModels(ctx *gin.Context) {
	models := c.routingService.ListModels(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": models})
}

// CreateModel 注册新 provider
//
// POST /api/llm/models
// Body: LLMProviderInfo
func (c *LLMRoutingController) CreateModel(ctx *gin.Context) {
	var info service.LLMProviderInfo
	if err := ctx.ShouldBindJSON(&info); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	if err := c.routingService.CreateModel(ctx.Request.Context(), info); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": info})
}

// GetModel 获取单个 provider
//
// GET /api/llm/models/:name
func (c *LLMRoutingController) GetModel(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	models := c.routingService.ListModels(ctx.Request.Context())
	for _, m := range models {
		if m.Name == name {
			ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": m})
			return
		}
	}
	ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "model not found"})
}

// UpdateModel 更新 provider
//
// PUT /api/llm/models/:name
// Body: LLMProviderInfo（name 字段被忽略，路径 :name 优先）
func (c *LLMRoutingController) UpdateModel(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	var info service.LLMProviderInfo
	if err := ctx.ShouldBindJSON(&info); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	if err := c.routingService.UpdateModel(ctx.Request.Context(), name, info); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": name})
}

// DeleteModel 删除 provider
//
// DELETE /api/llm/models/:name
func (c *LLMRoutingController) DeleteModel(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	if err := c.routingService.DeleteModel(ctx.Request.Context(), name); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": name})
}

// TestModel 测试 provider 连通性（独立路径，不污染全局路由/告警/熔断）
//
// POST /api/llm/models/:name/test
// Body: { "prompt": "...", "timeout_seconds": 60 }
func (c *LLMRoutingController) TestModel(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	var req service.TestModelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// Body 解析失败也允许（仅做最小测试）
		req = service.TestModelRequest{Provider: name}
	}
	req.Provider = name // 强制使用 URL 中的 name
	result, err := c.routingService.TestModel(ctx.Request.Context(), req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// ============================================================================
// 场景路由管理
// ============================================================================

// ListStrategies 列出所有场景路由
//
// GET /api/llm/strategies
// 别名：GET /api/llm/scene-routing（兼容旧前端）
// 返回：[]ScenarioRoute
func (c *LLMRoutingController) ListStrategies(ctx *gin.Context) {
	routes := c.routingService.ListStrategies(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": routes})
}

// UpdateStrategies 批量更新场景路由
//
// PUT /api/llm/strategies
// Body: { "routes": [...], "operator": "...", "trace_id": "...", "commit_msg": "..." }
func (c *LLMRoutingController) UpdateStrategies(ctx *gin.Context) {
	var req service.UpdateStrategiesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	if err := c.routingService.UpdateStrategies(ctx.Request.Context(), req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	// 返回最新快照
	routes := c.routingService.ListStrategies(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": routes})
}

// UpdateSceneRouting 兼容旧接口（PUT /api/llm/scene-routing）
//
// Body 兼容：{ routes: [...] } 或 { scenario: "...", provider: "...", fallbacks: [...] }
func (c *LLMRoutingController) UpdateSceneRouting(ctx *gin.Context) {
	// 尝试解析成 UpdateStrategiesRequest
	var req service.UpdateStrategiesRequest
	if err := ctx.ShouldBindJSON(&req); err == nil && len(req.Routes) > 0 {
		if err := c.routingService.UpdateStrategies(ctx.Request.Context(), req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": req.Routes})
		return
	}
	// 兼容单 route 形式
	var single llm.ScenarioRoute
	if err := ctx.ShouldBindJSON(&single); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	batch := service.UpdateStrategiesRequest{Routes: []llm.ScenarioRoute{single}}
	if err := c.routingService.UpdateStrategies(ctx.Request.Context(), batch); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": single})
}

// ListAuditHistory 列出路由变更审计历史
//
// GET /api/llm/audit?scenario=xxx&limit=50
func (c *LLMRoutingController) ListAuditHistory(ctx *gin.Context) {
	scenario := ctx.Query("scenario")
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	rows, err := c.routingService.ListAuditHistory(ctx.Request.Context(), scenario, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": rows})
}

// ============================================================================
// 用量统计
// ============================================================================

// Stats 进程内实时 provider 维度统计
//
// GET /api/llm/stats
func (c *LLMRoutingController) Stats(ctx *gin.Context) {
	stats := c.routingService.Stats(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
}

// Usage 跨进程历史统计（按 window）
//
// GET /api/llm/usage?window=today|week|month|all
func (c *LLMRoutingController) Usage(ctx *gin.Context) {
	window := ctx.DefaultQuery("window", "all")
	summary, err := c.routingService.Usage(ctx.Request.Context(), window)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "trace_id": logger.TraceIDFromContext(ctx.Request.Context())})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": summary})
}

// CostStats 用量 + 成本统计（前端 cost-stats tab 使用）
//
// GET /api/llm/cost-stats?window=...
// 复用 Usage 接口但返回前端期望的 {monthlyCost, byModel} 形态
func (c *LLMRoutingController) CostStats(ctx *gin.Context) {
	window := ctx.DefaultQuery("window", "month")
	summary, err := c.routingService.Usage(ctx.Request.Context(), window)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	// 转换为前端期望的形态
	out := gin.H{
		"window":        summary.WindowLabel,
		"monthly_cost":  summary.TotalCost,
		"total_tokens":  summary.TotalTokens,
		"total_calls":   summary.TotalCalls,
		"by_scenario":   summary.ByScenario,
		"by_provider":   summary.ByProvider,
		"by_model":      summary.ByProvider, // 前端字段别名
		"enabled_models": summary.EnabledModels,
		"active_models":  summary.ActiveModels,
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}

// FallbackConfig 兜底配置查询（兼容旧前端）
//
// GET /api/llm/fallback
// 返回当前所有场景路由的 fallback 列表（前端 fallback tab 使用）
func (c *LLMRoutingController) FallbackConfig(ctx *gin.Context) {
	routes := c.routingService.ListStrategies(ctx.Request.Context())
	out := make([]gin.H, 0, len(routes))
	for _, r := range routes {
		out = append(out, gin.H{
			"scenario":  r.Scenario,
			"provider":  r.Provider,
			"fallbacks": r.Fallbacks,
			"version":   r.Version,
		})
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": out})
}
