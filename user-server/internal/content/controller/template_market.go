package controller

import (
	"marketing/internal/content/model"
	"marketing/internal/content/service"
	syscontroller "marketing/internal/controller"
	"marketing/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// TemplateMarketController 模板市场控制器
type TemplateMarketController struct {
	marketService *service.TemplateMarketService
}

// NewTemplateMarketController 创建模板市场控制器实例
func NewTemplateMarketController() *TemplateMarketController {
	return &TemplateMarketController{
		marketService: service.NewTemplateMarketService(),
	}
}

// GetTemplateList 获取模板列表
func (c *TemplateMarketController) GetTemplateList(ctx *gin.Context) {
	category := ctx.Query("category")
	templateType := ctx.Query("type")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.marketService.GetTemplateList(category, templateType, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      templates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetTemplateByID 获取模板详情
func (c *TemplateMarketController) GetTemplateByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板 ID")
		return
	}

	template, err := c.marketService.GetTemplateByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, template, "获取成功")
}

// DownloadTemplate 下载模板
func (c *TemplateMarketController) DownloadTemplate(ctx *gin.Context) {

	userID, _ := ctx.Get("user_id")
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板 ID")
		return
	}

	template, err := c.marketService.DownloadTemplate(userID.(uint), uint(id))
	if syscontroller.HandleDBError(ctx, err, "下载模板") {
		return
	}

	response.Success(ctx, template, "下载成功")
}

// GetOfficialTemplates 获取官方模板
func (c *TemplateMarketController) GetOfficialTemplates(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.marketService.GetOfficialTemplates(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      templates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// SearchTemplates 搜索模板
func (c *TemplateMarketController) SearchTemplates(ctx *gin.Context) {
	keyword := ctx.Query("keyword")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.marketService.SearchTemplates(keyword, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      templates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetMyDownloads 获取我的下载
func (c *TemplateMarketController) GetMyDownloads(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	records, total, err := c.marketService.GetMyDownloads(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// CreateTemplate 创建模板
func (c *TemplateMarketController) CreateTemplate(ctx *gin.Context) {

	var template model.MarketTemplate
	if err := ctx.ShouldBindJSON(&template); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	template.IsOfficial = false
	if template.Author == "" {
		template.Author = ""
	}

	if syscontroller.HandleServiceError(ctx, c.marketService.CreateTemplate(&template)) {
		return
	}

	response.Success(ctx, template, "创建成功")
}

// RateTemplate 为模板评分
func (c *TemplateMarketController) RateTemplate(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板 ID")
		return
	}

	var req struct {
		Rating float64 `json:"rating" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	if syscontroller.HandleDBError(ctx, c.marketService.RateTemplate(uint(id), req.Rating), "评分模板") {
		return
	}

	response.Success(ctx, gin.H{
		"template_id": id,
		"rating":      req.Rating,
	}, "评分成功")
}
