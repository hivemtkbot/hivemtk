package controller

import (
	"hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/errhttp"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// MarketingFlowController 营销流程控制器
type MarketingFlowController struct {
	flowService *service.MarketingFlowService
}

// NewMarketingFlowController 创建营销流程控制器实例
func NewMarketingFlowController() *MarketingFlowController {
	return &MarketingFlowController{
		flowService: service.NewMarketingFlowService(),
	}
}

// CreateFlow 创建流程
func (c *MarketingFlowController) CreateFlow(ctx *gin.Context) {

	userID, _ := ctx.Get("user_id")
	var req service.CreateFlowRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	flow, err := c.flowService.CreateFlow(userID.(uint), &req)
	if errhttp.HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, flow, "创建成功")
}

// GetFlowList 获取流程列表
func (c *MarketingFlowController) GetFlowList(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	flows, total, err := c.flowService.GetFlowList(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      flows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetFlowByID 获取流程详情
func (c *MarketingFlowController) GetFlowByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	flow, err := c.flowService.GetFlowByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, flow, "获取成功")
}

// UpdateFlow 更新流程
func (c *MarketingFlowController) UpdateFlow(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	var req service.UpdateFlowRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	flow, err := c.flowService.UpdateFlow(uint(id), &req)
	if errhttp.HandleDBError(ctx, err, "更新营销流程") {
		return
	}

	response.Success(ctx, flow, "更新成功")
}

// DeleteFlow 删除流程
func (c *MarketingFlowController) DeleteFlow(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.flowService.DeleteFlow(uint(id)), "删除营销流程") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// ActivateFlow 激活流程
func (c *MarketingFlowController) ActivateFlow(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.flowService.ActivateFlow(uint(id)), "激活营销流程") {
		return
	}

	response.Success(ctx, nil, "激活成功")
}

// PauseFlow 暂停流程
func (c *MarketingFlowController) PauseFlow(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.flowService.PauseFlow(uint(id)), "暂停营销流程") {
		return
	}

	response.Success(ctx, nil, "暂停成功")
}

// StopFlow 停止流程
func (c *MarketingFlowController) StopFlow(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.flowService.StopFlow(uint(id)), "停止营销流程") {
		return
	}

	response.Success(ctx, nil, "停止成功")
}

// GetExecutionList 获取执行记录列表
func (c *MarketingFlowController) GetExecutionList(ctx *gin.Context) {

	flowIDStr := ctx.Param("id")
	flowID, err := strconv.ParseUint(flowIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	executions, total, err := c.flowService.GetExecutionList(uint(flowID), page, pageSize)
	if errhttp.HandleDBError(ctx, err, "获取执行记录") {
		return
	}

	response.Success(ctx, gin.H{
		"list":      executions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetExecutionStats 获取执行统计
func (c *MarketingFlowController) GetExecutionStats(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的流程 ID")
		return
	}

	stats, err := c.flowService.GetExecutionStats(uint(id))
	if errhttp.HandleDBError(ctx, err, "获取执行统计") {
		return
	}

	response.Success(ctx, stats, "获取成功")
}
