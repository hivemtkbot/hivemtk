package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ManageSmartRouterController 管理端智能路由控制器
type ManageSmartRouterController struct {
	svc *service.SmartRouter
}

// NewManageSmartRouterController 构造
func NewManageSmartRouterController() *ManageSmartRouterController {
	return &ManageSmartRouterController{svc: service.NewSmartRouter()}
}

// MatchAgent POST /api/manage/smart-router/match
// body: {"intent_type": "refund", "skills_needed": ["refund", "vip"]}
func (c *ManageSmartRouterController) MatchAgent(ctx *gin.Context) {
	var req struct {
		IntentType   string   `json:"intent_type"`
		SkillsNeeded []string `json:"skills_needed"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "请求参数错误："+err.Error())
		return
	}
	svcReq := &service.SmartRouteRequest{
		Intent:       req.IntentType,
		SkillsNeeded: req.SkillsNeeded,
	}
	result, err := c.svc.SelectAgent(ctx.Request.Context(), svcReq)
	if HandleServiceError(ctx, err) {
		return
	}
	if result == nil {
		response.Success(ctx, gin.H{"agent": nil, "message": "无可用坐席"}, "ok")
		return
	}
	response.Success(ctx, result, "路由完成")
}
