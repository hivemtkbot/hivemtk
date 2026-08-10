package controller

import (
	"hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/errhttp"

	"github.com/gin-gonic/gin"
)

// AIContentController AI内容控制器
type AIContentController struct {
	contentService  *service.AIContentService
	templateService *service.PromptTemplateService
}

// NewAIContentController 创建AI内容控制器实例
func NewAIContentController() *AIContentController {
	return &AIContentController{
		contentService:  service.NewAIContentService(db.GetDB()),
		templateService: service.NewPromptTemplateService(db.GetDB()),
	}
}

// GenerateContent 生成内容
func (c *AIContentController) GenerateContent(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	uid := convertToUint(userID)

	var req service.GenerateContentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	result, err := c.contentService.GenerateContent(ctx.Request.Context(), uid, &req)
	if errhttp.HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, result, "生成成功")
}

// CreateHistory 创建生成历史记录（不调用外部 AI 服务，直接保存到数据库）
func (c *AIContentController) CreateHistory(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	uid := convertToUint(userID)

	var req service.CreateHistoryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	record, err := c.contentService.CreateHistory(uid, &req)
	if errhttp.HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, record, "创建成功")
}

// GetGenerationHistory 获取生成历史
func (c *AIContentController) GetGenerationHistory(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	uid := convertToUint(userID)
	if uid == 0 {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	filters := make(map[string]any)
	if recordType := ctx.Query("type"); recordType != "" {
		filters["type"] = recordType
	}
	if isSaved := ctx.Query("is_saved"); isSaved != "" {
		filters["is_saved"] = isSaved == "true"
	}
	if isFavorite := ctx.Query("is_favorite"); isFavorite != "" {
		filters["is_favorite"] = isFavorite == "true"
	}

	records, total, err := c.contentService.GetGenerationHistory(uid, page, pageSize, filters)
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

// GetRecordByID 获取生成记录详情
func (c *AIContentController) GetRecordByID(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid := convertToUint(userID)
	if uid == 0 {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}

	record, err := c.contentService.GetRecordByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, record, "获取成功")
}

// SaveRecord 保存生成记录
func (c *AIContentController) SaveRecord(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.contentService.SaveRecord(uint(id)), "保存记录") {
		return
	}

	response.Success(ctx, nil, "保存成功")
}

// FavoriteRecord 收藏生成记录
func (c *AIContentController) FavoriteRecord(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}

	var req struct {
		IsFavorite bool `json:"is_favorite"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		req.IsFavorite = true
	}

	if errhttp.HandleDBError(ctx, c.contentService.FavoriteRecord(uint(id), req.IsFavorite), "收藏记录") {
		return
	}

	response.Success(ctx, nil, "操作成功")
}

// RateRecord 评分生成记录
func (c *AIContentController) RateRecord(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}

	var req struct {
		Rating int `json:"rating" binding:"required,min=1,max=5"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请提供1-5的评分")
		return
	}

	if errhttp.HandleDBError(ctx, c.contentService.RateRecord(uint(id), req.Rating), "评分记录") {
		return
	}

	response.Success(ctx, nil, "评分成功")
}

// DeleteRecord 删除生成记录
func (c *AIContentController) DeleteRecord(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的记录ID")
		return
	}

	if errhttp.HandleServiceError(ctx, c.contentService.DeleteRecord(uint(id))) {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetTemplates 获取模板列表
func (c *AIContentController) GetTemplates(ctx *gin.Context) {
	templateType := ctx.Query("type")

	templates, err := c.templateService.GetTemplates(templateType)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, templates, "获取成功")
}

// GetTemplateByID 获取模板详情
func (c *AIContentController) GetTemplateByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板ID")
		return
	}

	template, err := c.templateService.GetTemplateByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, template, "获取成功")
}

// CreateTemplate 创建模板
func (c *AIContentController) CreateTemplate(ctx *gin.Context) {

	var req service.CreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	template, err := c.templateService.CreateTemplate(&req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, template, "创建成功")
}

// UpdateTemplate 更新模板
func (c *AIContentController) UpdateTemplate(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板ID")
		return
	}

	var req service.CreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	template, err := c.templateService.UpdateTemplate(uint(id), &req)
	if errhttp.HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, template, "更新成功")
}

// DeleteTemplate 删除模板
func (c *AIContentController) DeleteTemplate(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板ID")
		return
	}

	if errhttp.HandleServiceError(ctx, c.templateService.DeleteTemplate(uint(id))) {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetTemplateTypes 获取模板类型列表
func (c *AIContentController) GetTemplateTypes(ctx *gin.Context) {
	types := c.templateService.GetTemplateTypes()
	response.Success(ctx, types, "获取成功")
}

// Helper function
func convertToUint(v any) uint {
	switch val := v.(type) {
	case uint:
		return val
	case int:
		return uint(val)
	case int64:
		return uint(val)
	case float64:
		return uint(val)
	default:
		return 0
	}
}
