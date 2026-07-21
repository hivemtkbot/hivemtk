package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LLMProviderController LLM Provider 降级管理控制器（M-1 P1 占位）
//
// TODO: 完整实现 health/circuit/policy 字段
// 当前仅返回 501，让路由可达、build 通过
type LLMProviderController struct{}

func NewLLMProviderController() *LLMProviderController {
	return &LLMProviderController{}
}

// GetHealth 查询所有 provider 健康度
func (c *LLMProviderController) GetHealth(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error": "LLM Provider health endpoint not yet implemented",
		"code":  "NOT_IMPLEMENTED",
	})
}

// GetProviderHealth 查询单个 provider 健康度
func (c *LLMProviderController) GetProviderHealth(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error":    "LLM Provider health endpoint not yet implemented",
		"code":     "NOT_IMPLEMENTED",
		"provider": ctx.Param("provider"),
	})
}

// ResetCircuit 重置 provider 熔断器（支持单个或全部）
func (c *LLMProviderController) ResetCircuit(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error": "Reset circuit not yet implemented",
		"code":  "NOT_IMPLEMENTED",
	})
}

// GetPolicy 查询降级策略
func (c *LLMProviderController) GetPolicy(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error": "Policy endpoint not yet implemented",
		"code":  "NOT_IMPLEMENTED",
	})
}

// UpdatePolicy 更新降级策略
func (c *LLMProviderController) UpdatePolicy(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error": "Policy endpoint not yet implemented",
		"code":  "NOT_IMPLEMENTED",
	})
}
