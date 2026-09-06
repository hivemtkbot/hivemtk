package controller

import (
	"hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/errhttp"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ScriptTemplateController 话术模板控制器
type ScriptTemplateController struct {
	templateService *service.ScriptTemplateService
}

// NewScriptTemplateController 创建话术模板控制器实例
func NewScriptTemplateController() *ScriptTemplateController {
	return &ScriptTemplateController{
		templateService: service.NewScriptTemplateService(),
	}
}

// CreateTemplate 创建话术模板
func (c *ScriptTemplateController) CreateTemplate(ctx *gin.Context) {

	userID, _ := ctx.Get("user_id")
	var req service.CreateScriptTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	template, err := c.templateService.CreateTemplate(userID.(uint), &req)
	if errhttp.HandleDBError(ctx, err, "创建话术模板") {
		return
	}

	response.Success(ctx, template, "创建成功")
}

// GetTemplateList 获取话术模板列表
func (c *ScriptTemplateController) GetTemplateList(ctx *gin.Context) {

	category := ctx.Query("category")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.templateService.GetTemplateList(category, page, pageSize)
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

// GetTemplateByID 获取话术模板详情
func (c *ScriptTemplateController) GetTemplateByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的话术 ID")
		return
	}

	template, err := c.templateService.GetTemplateByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, template, "获取成功")
}

// UpdateTemplate 更新话术模板
func (c *ScriptTemplateController) UpdateTemplate(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的话术 ID")
		return
	}

	var req service.UpdateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	template, err := c.templateService.UpdateTemplate(uint(id), &req)
	if errhttp.HandleDBError(ctx, err, "更新话术模板") {
		return
	}

	response.Success(ctx, template, "更新成功")
}

// DeleteTemplate 删除话术模板
func (c *ScriptTemplateController) DeleteTemplate(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的话术 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.templateService.DeleteTemplate(uint(id)), "删除话术模板") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetCategories 获取话术分类
func (c *ScriptTemplateController) GetCategories(ctx *gin.Context) {

	categories, err := c.templateService.GetCategories()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, categories, "获取成功")
}

// SearchTemplates 搜索话术
func (c *ScriptTemplateController) SearchTemplates(ctx *gin.Context) {

	keyword := ctx.Query("keyword")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.templateService.SearchTemplates(keyword, page, pageSize)
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

// GetPublicTemplates 获取公开话术模板
func (c *ScriptTemplateController) GetPublicTemplates(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	templates, total, err := c.templateService.GetPublicTemplates(page, pageSize)
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

// RecommendScript 推荐话术
func (c *ScriptTemplateController) RecommendScript(ctx *gin.Context) {

	var req struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	templates, err := c.templateService.RecommendScript(req.SessionID, req.Message)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, templates, "获取推荐成功")
}

func (c *ScriptTemplateController) SyncToLibrary(ctx *gin.Context) {
	syncSvc := service.NewScriptTemplateSyncService()
	stats, err := syncSvc.SyncToLibrary(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "同步完成")
}
