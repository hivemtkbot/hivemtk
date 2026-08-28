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


// ---------- K5 高级统计端点（GrowthBook 轻量版） ----------

// GetAdvancedStats GET /api/ab-experiments/:id/stats?method=frequentist|bayesian
func (c *ABExperimentController) GetAdvancedStats(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	method := ctx.DefaultQuery("method", "frequentist")
	var out map[string]any
	switch method {
	case "bayesian":
		out, err = c.abService.GetBayesianTest(uint(id))
	default:
		out, err = c.abService.GetAdvancedStats(uint(id))
	}
	if errhttp.HandleDBError(ctx, err, "查询实验统计") {
		return
	}
	response.Success(ctx, out, "ok")
}

// GetExperimentDiagnostics GET /api/ab-experiments/:id/diagnostics
func (c *ABExperimentController) GetExperimentDiagnostics(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	out, err := c.abService.GetDiagnostics(uint(id))
	if errhttp.HandleDBError(ctx, err, "查询实验诊断") {
		return
	}
	response.Success(ctx, out, "ok")
}

// GetExperimentCUPED GET /api/ab-experiments/:id/cuped
func (c *ABExperimentController) GetExperimentCUPED(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	out, err := c.abService.GetCUPED(uint(id))
	if errhttp.HandleDBError(ctx, err, "查询 CUPED") {
		return
	}
	response.Success(ctx, out, "ok")
}

// PostSequentialTest POST /api/ab-experiments/:id/sequential-test {alpha}
func (c *ABExperimentController) PostSequentialTest(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	var req struct {
		Alpha float64 `json:"alpha"`
	}
	_ = ctx.ShouldBindJSON(&req)
	out, err := c.abService.GetSequentialTest(uint(id), req.Alpha)
	if errhttp.HandleDBError(ctx, err, "序贯检验") {
		return
	}
	response.Success(ctx, out, "ok")
}

// PostBayesianTest POST /api/ab-experiments/:id/bayesian-test
func (c *ABExperimentController) PostBayesianTest(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	out, err := c.abService.GetBayesianTest(uint(id))
	if errhttp.HandleDBError(ctx, err, "贝叶斯检验") {
		return
	}
	response.Success(ctx, out, "ok")
}

// GetResultsWithReach GET /api/ab-experiments/:id/results-with-reach
func (c *ABExperimentController) GetResultsWithReach(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, http.StatusBadRequest, "无效的实验 ID")
		return
	}
	out, err := c.abService.GetResultsWithReach(uint(id))
	if errhttp.HandleDBError(ctx, err, "查询触达聚合结果") {
		return
	}
	response.Success(ctx, out, "ok")
}
