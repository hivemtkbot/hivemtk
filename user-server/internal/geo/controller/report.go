package controller

import (
	"net/http"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// ReportController GEO 报表控制器。
type ReportController struct {
	svc *service.ReportService
}

// NewReportController 构造报表控制器。
func NewReportController(svc *service.ReportService) *ReportController {
	return &ReportController{svc: svc}
}

// GetReport 获取 GEO 汇总报表
// GET /geo/reports/summary
func (c *ReportController) GetReport(ctx *gin.Context) {
	startDate := ctx.Query("start_date")
	endDate := ctx.Query("end_date")

	result, err := c.svc.GetReport(ctx.Request.Context(), startDate, endDate)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取汇总报表失败")
		return
	}
	response.Success(ctx, result, "获取汇总报表成功")
}

// GetROI 获取 ROI 分析
// GET /geo/reports/roi
func (c *ReportController) GetROI(ctx *gin.Context) {
	provider := ctx.Query("provider")
	model := ctx.Query("model")
	startDate := ctx.Query("start_date")
	endDate := ctx.Query("end_date")

	result, err := c.svc.GetROI(ctx.Request.Context(), provider, model, startDate, endDate)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取 ROI 分析失败")
		return
	}
	response.Success(ctx, result, "获取 ROI 分析成功")
}

// GetAPICosts 获取 API 调用成本
// GET /geo/reports/api-costs
func (c *ReportController) GetAPICosts(ctx *gin.Context) {
	result, err := c.svc.GetAPICosts(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取 API 成本失败")
		return
	}
	response.Success(ctx, result, "获取 API 成本成功")
}
