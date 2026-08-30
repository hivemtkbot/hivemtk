// Package controller - 智能路由 & 技能匹配（G2）
package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// SmartRouterController 智能路由 API
type SmartRouterController struct {
	svc *service.SmartRouter
}

// NewSmartRouterController 创建实例
func NewSmartRouterController() *SmartRouterController {
	return &SmartRouterController{
		svc: service.NewSmartRouter(),
	}
}

// SelectAgent POST /api/smart-router/select
// 请求体: {"intent": "refund", "skills_needed": ["refund", "vip"]}
func (c *SmartRouterController) SelectAgent(ctx *gin.Context) {
	var req service.SmartRouteRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	result, err := c.svc.SelectAgent(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		// 无候选坐席（全离线或满载），返回空让上层降级
		response.Success(ctx, gin.H{"agent": nil, "message": "无可用坐席，请降级到 round-robin"}, "ok")
		return
	}
	response.Success(ctx, result, "路由完成")
}
