package controller

// sop_template_controller.go SOP 模板 L3 Controller
//
// 五层架构归属: L3 API 接入层
// 设计依据: 2026-07-31 P1-A 知识库管理
//
// 接口前缀: /api/sop-templates
//   GET    /api/sop-templates       列表查询 (前端 SOP 模板管理页面)
//   GET    /api/sop-templates/:id   详情
//   POST   /api/sop-templates       新增
//   PUT    /api/sop-templates/:id   更新
//   DELETE /api/sop-templates/:id   删除
//   POST   /api/sop-templates/match 按 (intent, stage) 匹配 (Layer1 调试接口)

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

// SOPTemplateController SOP 模板控制器
type SOPTemplateController struct {
	svc *service.SOPTemplateService
}

// NewSOPTemplateController 创建 SOP 模板控制器
func NewSOPTemplateController(db *gorm.DB) *SOPTemplateController {
	return &SOPTemplateController{
		svc: service.NewSOPTemplateService(db, nil),
	}
}

// RegisterRoutes 注册路由
func (c *SOPTemplateController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/sop-templates")
	{
		g.GET("", c.List)
		g.GET("/:id", c.Get)
		g.POST("", c.Create)
		g.PUT("/:id", c.Update)
		g.DELETE("/:id", c.Delete)
		g.POST("/match", c.Match)
	}
}

// List 列表查询
func (c *SOPTemplateController) List(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	filter := repository.SOPTemplateFilter{
		Keyword:  ctx.Query("keyword"),
		Intent:   ctx.Query("intent"),
		Stage:    ctx.Query("stage"),
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
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// Get 详情
func (c *SOPTemplateController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP 模板 ID")
		return
	}
	tpl, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "SOP 模板不存在")
		return
	}
	response.Success(ctx, tpl, "查询成功")
}

// sopTemplateCreateReq 创建/更新请求体
type sopTemplateCreateReq struct {
	Name       string  `json:"name" binding:"required"`
	Intent     string  `json:"intent" binding:"required"`
	Stage      string  `json:"stage" binding:"required"`
	Template   string  `json:"template" binding:"required"`
	Vars       string  `json:"vars"`
	Priority   int     `json:"priority"`
	Confidence float64 `json:"confidence"`
	Enabled    *bool   `json:"enabled"`
}

// Create 新增
func (c *SOPTemplateController) Create(ctx *gin.Context) {
	var req sopTemplateCreateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	tpl := &model.SOPTemplate{
		Name:       req.Name,
		Intent:     req.Intent,
		Stage:      req.Stage,
		Template:   req.Template,
		Vars:       req.Vars,
		Priority:   req.Priority,
		Confidence: req.Confidence,
		Enabled:    req.Enabled,
	}
	if err := c.svc.Create(ctx.Request.Context(), tpl); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, tpl, "创建成功")
}

// Update 更新
func (c *SOPTemplateController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP 模板 ID")
		return
	}
	var req sopTemplateCreateReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	tpl := &model.SOPTemplate{
		Name:       req.Name,
		Intent:     req.Intent,
		Stage:      req.Stage,
		Template:   req.Template,
		Vars:       req.Vars,
		Priority:   req.Priority,
		Confidence: req.Confidence,
		Enabled:    req.Enabled,
	}
	if err := c.svc.Update(ctx.Request.Context(), uint(id), tpl); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "更新成功")
}

// Delete 删除
func (c *SOPTemplateController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP 模板 ID")
		return
	}
	if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "删除成功")
}

// sopTemplateMatchReq 匹配请求
type sopTemplateMatchReq struct {
	Intent string `json:"intent" binding:"required"`
	Stage  string `json:"stage"`
	TopK   int    `json:"top_k"`
}

// Match 按 (intent, stage) 匹配 (Layer1 调试接口)
func (c *SOPTemplateController) Match(ctx *gin.Context) {
	var req sopTemplateMatchReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	matches, err := c.svc.MatchByIntentStage(ctx.Request.Context(), req.Intent, req.Stage)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if req.TopK > 0 && len(matches) > req.TopK {
		matches = matches[:req.TopK]
	}
	response.Success(ctx, matches, "匹配成功")
}
