package controller

// help_center.go 帮助中心控制器（R48 T1）

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
	return &HelpCenterController{svc: service.NewHelpCenterServiceFromGlobal()}
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

// Search GET /api/public/help-center/search?keyword=xxx&limit=10
// [P0-FIX B] 公开门户搜索端点：走 ILIKE 标题 + knowledge_chunks 正文关联查询，免登录
func (c *HelpCenterController) Search(ctx *gin.Context) {
	keyword := ctx.Query("keyword")
	limit := 20
	if l := ctx.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if keyword == "" {
		response.Success(ctx, gin.H{"list": []any{}, "total": 0}, "ok")
		return
	}
	list, err := c.svc.Search(ctx.Request.Context(), keyword, limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// ArticleDetail GET /api/public/help-center/articles/:id（含 views 自增）
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
	c.svc.IncArticleViews(ctx.Request.Context(), id)
	response.Success(ctx, art, "ok")
}

// SetStatus PATCH /api/knowledge/documents/:id/help-center-status {status}
func (c *HelpCenterController) SetStatus(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, 400, "无效的文档 ID")
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "status 必填")
		return
	}
	if err := c.svc.SetArticleStatus(ctx.Request.Context(), id, req.Status); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"help_center_status": req.Status}, "状态已更新")
}

// TopArticles GET /api/knowledge/help-center/top?limit=10（效果统计）
func (c *HelpCenterController) TopArticles(ctx *gin.Context) {
	limit := 10
	if l := ctx.Query("limit"); l != "" {
		if n, err2 := strconv.Atoi(l); err2 == nil {
			limit = n
		}
	}
	list, err := c.svc.TopArticles(ctx.Request.Context(), limit)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// RetrievalTest POST /api/knowledge/help-center/retrieval-test {product_id,query,top_k}
func (c *HelpCenterController) RetrievalTest(ctx *gin.Context) {
	var req struct {
		ProductID string `json:"product_id"`
		Query     string `json:"query" binding:"required"`
		TopK      int    `json:"top_k"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "query 必填")
		return
	}
	res, err := c.svc.RetrievalTest(ctx.Request.Context(), req.ProductID, req.Query, req.TopK)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, "ok")
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
