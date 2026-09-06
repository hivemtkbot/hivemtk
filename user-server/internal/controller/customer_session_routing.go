package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AssignSession 分配会话
func (c *CustomerSessionController) AssignSession(ctx *gin.Context) {
	var req service.AssignSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.AssignSession(ctx.Request.Context(), &req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
}

// AutoAssignSession 自动分配会话
func (c *CustomerSessionController) AutoAssignSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	if err := c.sessionService.AutoAssign(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
}

// TransferSession 转接会话
func (c *CustomerSessionController) TransferSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		NewAgentID uint `json:"new_agent_id" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.TransferSession(ctx.Request.Context(), uint(id), req.NewAgentID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "转接成功")
}
