package controller

import (
	syscontroller "marketing/internal/controller"
	"marketing/internal/ops/model"
	"marketing/internal/ops/service"
	"marketing/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ChurnPredictionController 流失预警控制器
type ChurnPredictionController struct {
	churnService *service.ChurnPredictionService
}

// NewChurnPredictionController 创建流失预警控制器实例
func NewChurnPredictionController() *ChurnPredictionController {
	return &ChurnPredictionController{
		churnService: service.NewChurnPredictionService(),
	}
}

// GetChurnPrediction 获取用户流失预测
func (c *ChurnPredictionController) GetChurnPrediction(ctx *gin.Context) {

	userID := ctx.Query("user_id")
	if userID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少用户 ID 参数")
		return
	}

	prediction, err := c.churnService.GetChurnPrediction(userID)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, prediction, "获取成功")
}

// GetChurnPredictions 获取流失预测列表
func (c *ChurnPredictionController) GetChurnPredictions(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	predictions, total, err := c.churnService.GetChurnPredictions(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      predictions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetHighRiskUsers 获取高风险用户列表
func (c *ChurnPredictionController) GetHighRiskUsers(ctx *gin.Context) {

	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))

	users, err := c.churnService.GetHighRiskUsers(limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, users, "获取成功")
}

// GetChurnWarnings 获取流失预警列表
func (c *ChurnPredictionController) GetChurnWarnings(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	warnings, total, err := c.churnService.GetChurnWarnings(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      warnings,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetUnhandledWarnings 获取未处理的流失预警
func (c *ChurnPredictionController) GetUnhandledWarnings(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	warnings, total, err := c.churnService.GetUnhandledWarnings(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      warnings,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// MarkWarningHandled 标记预警为已处理
func (c *ChurnPredictionController) MarkWarningHandled(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的预警 ID")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	// 获取处理用户 ID
	userID, _ := ctx.Get("user_id")
	handledBy, _ := userID.(uint)

	if syscontroller.HandleDBError(ctx, c.churnService.MarkWarningHandled(uint(id), handledBy, req.Note), "标记预警") {
		return
	}

	response.Success(ctx, nil, "标记成功")
}

// InterveneUser 对流失预警用户进行干预
// 入参 JSON: { warning_id: 1, intervention_type: "discount" }
func (c *ChurnPredictionController) InterveneUser(ctx *gin.Context) {
	var req struct {
		WarningID        uint   `json:"warning_id" binding:"required"`
		InterventionType string `json:"intervention_type" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	// 获取处理用户 ID
	userID, _ := ctx.Get("user_id")
	handledBy, _ := userID.(uint)

	if syscontroller.HandleDBError(ctx, c.churnService.InterveneWarning(req.WarningID, handledBy, req.InterventionType), "干预预警") {
		return
	}

	response.Success(ctx, gin.H{
		"warning_id":        req.WarningID,
		"intervention_type": req.InterventionType,
	}, "干预成功")
}

// GetModelConfig 获取模型配置
func (c *ChurnPredictionController) GetModelConfig(ctx *gin.Context) {

	config, err := c.churnService.GetModelConfig()
	if syscontroller.HandleDBError(ctx, err, "获取模型配置") {
		return
	}

	response.Success(ctx, config, "获取成功")
}

// SaveModelConfig 保存模型配置
func (c *ChurnPredictionController) SaveModelConfig(ctx *gin.Context) {

	var config model.ChurnModelConfig
	if err := ctx.ShouldBindJSON(&config); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	if syscontroller.HandleDBError(ctx, c.churnService.SaveModelConfig(&config), "保存模型配置") {
		return
	}

	response.Success(ctx, config, "保存成功")
}

// GetChurnStatistics 获取流失统计
//
// 同类修复：start_date / end_date 为必填参数，缺失时返回 400
// （不允许用默认值/mock 数据兜底，符合"不允许使用模拟数据"硬约束）。
// 前端页面初次加载由前端主动传入最近 30 天日期。
func (c *ChurnPredictionController) GetChurnStatistics(ctx *gin.Context) {

	startDate := ctx.Query("start_date")
	endDate := ctx.Query("end_date")

	if startDate == "" || endDate == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少必要参数：start_date / end_date")
		return
	}

	stats, err := c.churnService.GetChurnStatistics(startDate, endDate)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, stats, "获取成功")
}

// GetRiskDistribution 获取风险分布
func (c *ChurnPredictionController) GetRiskDistribution(ctx *gin.Context) {

	distribution, err := c.churnService.GetRiskDistribution()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, distribution, "获取成功")
}
