// Package controller - Agent 绩效归因（G5）
package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// AgentAttributionController 绩效归因 API
type AgentAttributionController struct {
	svc *service.AgentAttributionService
}

// NewAgentAttributionController 创建实例
func NewAgentAttributionController() *AgentAttributionController {
	return &AgentAttributionController{
		svc: service.NewAgentAttributionService(),
	}
}

// GetPerformance GET /api/agent-attribution/performance?period_days=7&agent_id=0
func (c *AgentAttributionController) GetPerformance(ctx *gin.Context) {
	var q service.PerformanceQuery

	if v := ctx.Query("period_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.PeriodDays = n
		}
	}
	if v := ctx.Query("agent_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			q.AgentID = uint(n)
		}
	}

	perfs, err := c.svc.GetPerformance(ctx.Request.Context(), &q)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, perfs, "获取成功")
}
