package controller

import (
	"hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/errhttp"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ABExperimentController A/B 测试控制器
type ABExperimentController struct {
	abService *service.ABExperimentService
}

// NewABExperimentController 创建 A/B 测试控制器实例
func NewABExperimentController() *ABExperimentController {
	return &ABExperimentController{
		abService: service.NewABExperimentService(),
	}
}

// CreateExperiment 创建实验
func (c *ABExperimentController) CreateExperiment(ctx *gin.Context) {

	var req service.CreateExperimentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	experiment, err := c.abService.CreateExperiment(&req)
	if errhttp.HandleDBError(ctx, err, "创建实验") {
		return
	}

	response.Success(ctx, experiment, "创建成功")
}

// GetExperiment 获取实验详情
func (c *ABExperimentController) GetExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	experiment, err := c.abService.GetExperiment(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, experiment, "获取成功")
}

// GetExperimentList 获取实验列表
func (c *ABExperimentController) GetExperimentList(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	experiments, total, err := c.abService.GetExperimentList(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      experiments,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// UpdateExperiment 更新实验
func (c *ABExperimentController) UpdateExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	var req service.CreateExperimentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	experiment, err := c.abService.UpdateExperiment(uint(id), &req)
	if errhttp.HandleDBError(ctx, err, "更新实验") {
		return
	}

	response.Success(ctx, experiment, "更新成功")
}

// DeleteExperiment 删除实验
func (c *ABExperimentController) DeleteExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.abService.DeleteExperiment(uint(id)), "删除实验") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// StartExperiment 启动实验
func (c *ABExperimentController) StartExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.abService.StartExperiment(uint(id)), "启动实验") {
		return
	}

	response.Success(ctx, nil, "启动成功")
}

// PauseExperiment 暂停实验
func (c *ABExperimentController) PauseExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.abService.PauseExperiment(uint(id)), "暂停实验") {
		return
	}

	response.Success(ctx, nil, "暂停成功")
}

// StopExperiment 停止实验
func (c *ABExperimentController) StopExperiment(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.abService.StopExperiment(uint(id)), "停止实验") {
		return
	}

	response.Success(ctx, nil, "停止成功")
}

// GetExperimentResults 获取实验结果
func (c *ABExperimentController) GetExperimentResults(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	results, err := c.abService.GetExperimentResults(uint(id))
	if errhttp.HandleDBError(ctx, err, "获取实验结果") {
		return
	}

	response.Success(ctx, results, "获取成功")
}

// GetConversionEvents 获取转化事件列表
func (c *ABExperimentController) GetConversionEvents(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	events, total, err := c.abService.GetConversionEvents(uint(id), page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      events,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}
