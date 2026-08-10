package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SessionTagController 会话标签控制器
type SessionTagController struct {
	tagService *service.SessionTagService
}

// NewSessionTagController 创建会话标签控制器实例
func NewSessionTagController() *SessionTagController {
	return &SessionTagController{
		tagService: service.NewSessionTagService(),
	}
}

// GetTags 获取标签列表
func (c *SessionTagController) GetTags(ctx *gin.Context) {
	tags, err := c.tagService.GetTags(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, tags, "获取成功")
}

// CreateTag 创建标签
func (c *SessionTagController) CreateTag(ctx *gin.Context) {
	var req service.CreateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	tag, err := c.tagService.CreateTag(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, tag, "创建成功")
}

// UpdateTag 更新标签
func (c *SessionTagController) UpdateTag(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的标签ID")
		return
	}

	var req service.CreateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	tag, err := c.tagService.UpdateTag(ctx.Request.Context(), uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, tag, "更新成功")
}

// DeleteTag 删除标签
func (c *SessionTagController) DeleteTag(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的标签ID")
		return
	}

	if err := c.tagService.DeleteTag(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}
