// help_center.go 帮助中心控制器（R48 T1）
package controller

import (
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// HelpCenterController 帮助中心控制器
type HelpCenterController struct {
	svc *service.HelpCenterService
}

// NewHelpCenterController 构造
func NewHelpCenterController() *HelpCenterController {
	return &HelpCenterController{svc: service.NewHelpCenterService()}
}

// Categories GET /api/public/help-center/categories（免登录）
func (c *HelpCenterController) Categories(ctx *gin.Context) {
	list, err := c.svc.Categories(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// Articles GET /api/public/help-center/articles?category=&q=
func (c *HelpCenterController) Articles(ctx *gin.Context) {
	list, err := c.svc.Articles(ctx.Request.Context(), ctx.Query("category"), ctx.Query("q"), 50)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// ArticleDetail GET /api/public/help-center/articles/:id
func (c *HelpCenterController) ArticleDetail(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, 400, "无效的文章 ID")
		return
	}
	art, err := c.svc.ArticleDetail(ctx.Request.Context(), id)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, art, "ok")
}

// SetVisibility PATCH /api/knowledge/documents/:id/public-visibility（管理端，登录）
func (c *HelpCenterController) SetVisibility(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, 400, "无效的文档 ID")
		return
	}
	var req struct {
		Visible *bool `json:"visible" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Visible == nil {
		response.Error(ctx, 400, "visible 必填")
		return
	}
	if err := c.svc.SetArticleVisibility(ctx.Request.Context(), id, *req.Visible); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"public_visible": *req.Visible}, "发布状态已更新")
}
