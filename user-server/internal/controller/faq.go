package controller


import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// FAQController FAQ 知识库控制器
type FAQController struct {
	svc *service.FAQService
}

// NewFAQController 创建 FAQ 控制器
func NewFAQController() *FAQController {
	return &FAQController{svc: service.NewFAQServiceDefault()}
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

// List godoc
// @Summary      FAQ 列表
// @Description  按关键词/分类/意图分页查询 FAQ 词条
// @Tags         FAQ
// @Produce      json
// @Security     BearerAuth
// @Param        keyword  query  string  false  "关键词"
// @Param        category query  string  false  "分类"
// @Param        intent   query  string  false  "意图编码"
// @Param        enabled  query  bool    false  "是否启用"
// @Param        page     query  int     false  "页码"  default(1)
// @Param        page_size query int    false  "每页"  default(20)
// @Success      200  {object}  response.Response  "成功"
// @Router       /api/faqs [get]
func (c *FAQController) List(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	filter := dto.FAQFilter{
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

// Get godoc
// @Summary      FAQ 详情
// @Description  根据 ID 返回 FAQ 词条完整内容
// @Tags         FAQ
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "FAQ ID"
// @Success      200  {object}  response.Response  "成功"
// @Failure      404  {object}  response.Response  "未找到"
// @Router       /api/faqs/{id} [get]
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
	AgentID uint `json:"agent_id" binding:"required"`
}

// Create godoc
// @Summary      新增 FAQ
// @Description  创建一个 FAQ 词条并绑定到指定智能体
// @Tags         FAQ
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  faqCreateReq  true  "FAQ 内容"
// @Success      200   {object}  response.Response  "创建成功"
// @Router       /api/faqs [post]
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

// Update godoc
// @Summary      更新 FAQ
// @Description  更新 FAQ 词条内容或绑定
// @Tags         FAQ
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path  int            true  "FAQ ID"
// @Param        body  body  faqCreateReq   true  "更新内容"
// @Success      200   {object}  response.Response  "更新成功"
// @Router       /api/faqs/{id} [put]
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
	AgentID uint   `json:"agent_id" binding:"required"` 
}

// Match 关键词匹配 (调试接口 + 未来 RAG 引擎调用)
func (c *FAQController) Match(ctx *gin.Context) {
	var req faqMatchReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
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

