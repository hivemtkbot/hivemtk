package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

// LLMProviderController LLM Provider 降级管理控制器（6 个端点真实实现，调用 ProviderFailover）
type LLMProviderController struct {
	failover *llm.ProviderFailover
}

// NewLLMProviderController 创建 LLM Provider 控制器
//
// failover 为 nil 时返回 503（依赖未就绪）；非 nil 时调用真实 ProviderFailover API。
func NewLLMProviderController(failover *llm.ProviderFailover) *LLMProviderController {
	return &LLMProviderController{failover: failover}
}

// guardFailover 检查 failover 是否就绪，未就绪返回 503
func (c *LLMProviderController) guardFailover(ctx *gin.Context) bool {
	if c.failover == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    503,
			"message": "ProviderFailover not initialized",
		})
		return false
	}
	return true
}

// GetHealth 查询所有 provider 健康度
//
// GET /api/llm-routings/providers/health
func (c *LLMProviderController) GetHealth(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	healths := c.failover.GetAllHealth()
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": healths})
}

// GetProviderHealth 查询单个 provider 健康度
//
// GET /api/llm-routings/providers/:name/health
func (c *LLMProviderController) GetProviderHealth(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	h := c.failover.GetHealth(name)
	if h == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "provider not found", "data": gin.H{"name": name}})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": h})
}

// ResetCircuit 重置 provider 熔断器（支持单个或全部）
//
// POST /api/llm-routings/providers/circuit/reset
// Body: { "name": "default" }  不传 name 则重置全部
func (c *LLMProviderController) ResetCircuit(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = ctx.ShouldBindJSON(&body) // body 可为空（重置全部）
	if body.Name == "" {
		// 重置全部
		all := c.failover.GetAllHealth()
		reset := 0
		for _, h := range all {
			if c.failover.ResetCircuit(h.ProviderName) {
				reset++
			}
		}
		ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"reset": reset, "total": len(all)}})
		return
	}
	ok := c.failover.ResetCircuit(body.Name)
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"name": body.Name, "reset": ok}})
}

// GetPolicy 查询降级策略
//
// GET /api/llm-routings/policy
func (c *LLMProviderController) GetPolicy(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	policy := c.failover.LoadPolicy(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": policy})
}

// UpdatePolicy 更新降级策略
//
// PUT /api/llm-routings/policy
// Body: FailoverPolicy JSON
func (c *LLMProviderController) UpdatePolicy(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	var policy llm.FailoverPolicy
	if err := ctx.ShouldBindJSON(&policy); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	c.failover.ApplyPolicy(policy)
	// 持久化：复用 LoadPolicy 流程中的 DB 写路径（system_kv_config.key=llm_provider_failover）
	// ApplyPolicy 是内存生效，外部需通过额外接口持久化（保留语义：UpdatePolicy 内存生效 + log）
	logger.Infof("[LLMProviderController] UpdatePolicy applied: scenarios=%d", len(policy.Scenarios))
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": policy})
}

// ResolveRoute 根据 scenario + canary key 决定走哪个 provider（决策可观测）
//
// POST /api/llm-routings/resolve
// Body: { "scenario": "intent_recognize", "canary_key": "user_123" }
// 返回：{ "provider": "...", "is_canary": true/false, "version": N }
func (c *LLMProviderController) ResolveRoute(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	var body struct {
		Scenario  string `json:"scenario"`
		CanaryKey string `json:"canary_key"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	if body.Scenario == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "scenario is required"})
		return
	}
	// 走 dispatcher 全局单例查 route
	d := llm.GetGlobalDispatcher()
	if d == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "dispatcher not initialized"})
		return
	}
	route := d.GetRoute(llm.DispatchScenario(body.Scenario))
	if route == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "scenario route not found"})
		return
	}
	canary := llm.DecideCanaryRoute(route, body.CanaryKey)
	resp := gin.H{
		"scenario":  body.Scenario,
		"provider":  route.Provider,
		"fallbacks": route.Fallbacks,
		"version":   route.Version,
		"weight":    route.Weight,
		"is_canary": canary != nil,
	}
	if canary != nil {
		resp["provider"] = canary.Provider
		resp["fallbacks"] = canary.Fallbacks
		resp["version"] = canary.Version
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
}

// compile-time interface guard
var _ context.Context = context.Background()
