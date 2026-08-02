package controller

// faq_controller.go FAQ 知识库 L3 Controller
//
// 五层架构归属: L3 API 接入层
// 设计依据: 知识库管理
//
// 接口前缀: /api/faqs
//   GET    /api/faqs                列表查询 (前端 FAQ 管理页面)
//   GET    /api/faqs/:id            详情
//   POST   /api/faqs                新增
//   PUT    /api/faqs/:id            更新
//   DELETE /api/faqs/:id            删除
//   POST   /api/faqs/match          关键词匹配 (Layer1 调试接口)
//   GET    /api/faqs/stats          统计

import (
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// FAQController FAQ 知识库控制器
type FAQController struct {
	svc *service.FAQService
}

// NewFAQController 创建 FAQ 控制器
func NewFAQController(db *gorm.DB) *FAQController {
	return &FAQController{svc: service.NewFAQService(db, nil)}
}

// RegisterRoutes 注册路由
func (c *FAQController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/faqs")
	{
		g.GET("", c.List)
		g.GET("/stats", c.Stats)
		g.GET("/:id", c.Get)
		g.POST("", c.Create)
		g.PUT("/:id", c.Update)
		g.DELETE("/:id", c.Delete)
		g.POST("/match", c.Match)
	}
}

// List 列表查询
func (c *FAQController) List(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	filter := repository.FAQFilter{
		Keyword:  ctx.Query("keyword"),
		Category: ctx.Query("category"),
		Intent:   ctx.Query("intent"),
		Page:     page,
		PageSize: pageSize,
	}
	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		if v, err := strconv.ParseBool(enabledStr); err == nil {
			filter.Enabled = &v
		}
	}
	list, total, err := c.svc.List(ctx.Request.Context(), filter)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// Get 详情
func (c *FAQController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 FAQ ID")
		return
	}
	entry, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "FAQ 不存在")
		return
	}
	response.Success(ctx, entry, "查询成功")
}

// faqCreateReq 创建/更新请求体
type faqCreateReq struct {
	Question   string   `json:"question" binding:"required"`
	Answer     string   `json:"answer" binding:"required"`
	Keywords   []string `json:"keywords"`
	Category   string   `json:"category"`
	Intent     string   `json:"intent"`
	Confidence float64  `json:"confidence"`
	Enabled    *bool    `json:"enabled"`
	// Task 15 强 1对1: AgentID 必填
	AgentID uint `json:"agent_id" binding:"required"`
}

// Create 新增
func (c *FAQController) Create(ctx *gin.Context) {
	var req faqCreateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agentID := req.AgentID
	entry := &model.FAQEntry{
		Question:   req.Question,
		Answer:     req.Answer,
		Keywords:   req.Keywords,
		Category:   req.Category,
		Intent:     req.Intent,
		Confidence: req.Confidence,
		Enabled:    req.Enabled,
		AgentID:    &agentID,
	}
	if err := c.svc.Create(ctx.Request.Context(), entry); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, entry, "创建成功")
}

// Update 更新
func (c *FAQController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 FAQ ID")
		return
	}
	var req faqCreateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agentID := req.AgentID
	entry := &model.FAQEntry{
		Question:   req.Question,
		Answer:     req.Answer,
		Keywords:   req.Keywords,
		Category:   req.Category,
		Intent:     req.Intent,
		Confidence: req.Confidence,
		Enabled:    req.Enabled,
		AgentID:    &agentID,
	}
	if err := c.svc.Update(ctx.Request.Context(), uint(id), entry); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "更新成功")
}

// Delete 删除
func (c *FAQController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 FAQ ID")
		return
	}
	if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "删除成功")
}

// faqMatchReq 关键词匹配请求
//
// Task 15 强 1对1: agent_id 必填 (uint); 不再支持"空 agent = 全局"分支
type faqMatchReq struct {
	Msg     string `json:"msg" binding:"required"`
	TopK    int    `json:"top_k"`
	AgentID uint   `json:"agent_id" binding:"required"` // Task 15 强 1对1: 必填
}

// Match 关键词匹配 (调试接口 + 未来 RAG 引擎调用)
func (c *FAQController) Match(ctx *gin.Context) {
	var req faqMatchReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	// Task 15 强 1对1: agent_id 必填且 > 0
	if req.AgentID == 0 {
		response.Error(ctx, http.StatusBadRequest, "agent_id 必填且 > 0 (Task 15 强 1对1)")
		return
	}
	matches, err := c.svc.MatchByAgent(ctx.Request.Context(), req.AgentID, req.Msg, req.TopK)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, matches, "匹配成功")
}

// Stats 统计
func (c *FAQController) Stats(ctx *gin.Context) {
	total, enabled, err := c.svc.Stats(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"total":   total,
		"enabled": enabled,
	}, "查询成功")
}
