package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
)

// LLMProviderController LLM Provider 降级管理控制器（6 个端点真实实现，经 service 中转 ProviderFailover）
type LLMProviderController struct {
	failoverSvc *service.LLMFailoverService
}

// NewLLMProviderController 创建 LLM Provider 控制器
//
// failoverSvc 未就绪时返回 503（依赖未就绪）；就绪时调用真实 ProviderFailover API。
func NewLLMProviderController(failoverSvc *service.LLMFailoverService) *LLMProviderController {
	return &LLMProviderController{failoverSvc: failoverSvc}
}

// guardFailover 检查 failover 是否就绪，未就绪返回 503
func (c *LLMProviderController) guardFailover(ctx *gin.Context) bool {
	if !c.failoverSvc.Ready() {
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
	healths := c.failoverSvc.GetAllHealth()
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
	h := c.failoverSvc.GetHealth(name)
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
	_ = ctx.ShouldBindJSON(&body) 
	if body.Name == "" {
		all := c.failoverSvc.GetAllHealth()
		reset := 0
		for _, h := range all {
			if c.failoverSvc.ResetCircuit(h.ProviderName) {
				reset++
			}
		}
		ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"reset": reset, "total": len(all)}})
		return
	}
	ok := c.failoverSvc.ResetCircuit(body.Name)
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"name": body.Name, "reset": ok}})
}

// GetPolicy 查询降级策略
//
// GET /api/llm-routings/policy
func (c *LLMProviderController) GetPolicy(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	policy := c.failoverSvc.LoadPolicy(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": policy})
}

// UpdatePolicy 更新降级策略
//
// PUT /api/llm-routings/policy
// Body: LLMFailoverPolicy JSON
func (c *LLMProviderController) UpdatePolicy(ctx *gin.Context) {
	if !c.guardFailover(ctx) {
		return
	}
	var policy service.LLMFailoverPolicy
	if err := ctx.ShouldBindJSON(&policy); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid body: " + err.Error()})
		return
	}
	c.failoverSvc.ApplyPolicy(policy)
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
	res, err := c.failoverSvc.ResolveRoute(body.Scenario, body.CanaryKey)
	if errors.Is(err, service.ErrLLMDispatcherNotReady) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "dispatcher not initialized"})
		return
	}
	if errors.Is(err, service.ErrLLMScenarioRouteNotFound) {
		ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "scenario route not found"})
		return
	}
	resp := gin.H{
		"scenario":  body.Scenario,
		"provider":  res.Provider,
		"fallbacks": res.Fallbacks,
		"version":   res.Version,
		"weight":    res.Weight,
		"is_canary": res.IsCanary,
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
}

