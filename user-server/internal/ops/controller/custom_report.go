package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/errhttp"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// CustomReportController 自定义报表控制器
type CustomReportController struct {
	reportService *service.CustomReportService
}

// NewCustomReportController 创建自定义报表控制器
func NewCustomReportController() *CustomReportController {
	return &CustomReportController{
		reportService: service.NewCustomReportService(),
	}
}

func extractUserContext(ctx *gin.Context) (uint, bool) {
	userID, _ := ctx.Get("user_id")
	roleVal, _ := ctx.Get("role")
	role, _ := roleVal.(string)
	isAdmin := role == "admin" || role == "super_admin"
	uid, _ := userID.(uint)
	return uid, isAdmin
}

// CreateReport 创建报表
func (c *CustomReportController) CreateReport(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	var req service.CreateReportRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	report, err := c.reportService.CreateReport(userID.(uint), &req)
	if errhttp.HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, report, "创建成功")
}

// GetReport 获取报表详情
func (c *CustomReportController) GetReport(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的报表 ID")
		return
	}

	userID, isAdmin := extractUserContext(ctx)
	report, err := c.reportService.GetReport(uint(id), userID, isAdmin)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, report, "获取成功")
}

// GetReportList 获取报表列表
func (c *CustomReportController) GetReportList(ctx *gin.Context) {

	page := 1
	pageSize := 20

	if p := ctx.Query("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if ps := ctx.Query("page_size"); ps != "" {
		pageSize, _ = strconv.Atoi(ps)
		if pageSize > 100 {
			pageSize = 100
		}
	}

	userID, isAdmin := extractUserContext(ctx)
	reports, total, err := c.reportService.GetReportList(page, pageSize, userID, isAdmin)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      reports,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// UpdateReport 更新报表
func (c *CustomReportController) UpdateReport(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的报表 ID")
		return
	}

	var req service.UpdateReportRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	userID, isAdmin := extractUserContext(ctx)
	report, err := c.reportService.UpdateReport(uint(id), userID, isAdmin, &req)
	if errhttp.HandleDBError(ctx, err, "更新报表") {
		return
	}

	response.Success(ctx, report, "更新成功")
}

// DeleteReport 删除报表
func (c *CustomReportController) DeleteReport(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的报表 ID")
		return
	}

	userID, isAdmin := extractUserContext(ctx)
	err = c.reportService.DeleteReport(uint(id), userID, isAdmin)
	if errhttp.HandleDBError(ctx, err, "删除报表") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetPublicTemplates 获取公开模板
func (c *CustomReportController) GetPublicTemplates(ctx *gin.Context) {
	templates, err := c.reportService.GetPublicTemplates()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, templates, "获取成功")
}

// UseTemplate 使用模板
func (c *CustomReportController) UseTemplate(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的模板 ID")
		return
	}

	report, err := c.reportService.UseTemplate(uint(id), userID.(uint))
	if errhttp.HandleDBError(ctx, err, "使用报表模板") {
		return
	}

	response.Success(ctx, report, "使用模板成功")
}

// QueryReportData 查询报表数据
func (c *CustomReportController) QueryReportData(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的报表 ID")
		return
	}

	params := make(map[string]any)
	if startTime := ctx.Query("start_time"); startTime != "" {
		if t, err := time.Parse("2006-01-02", startTime); err == nil {
			params["start_time"] = t
		}
	}
	if endTime := ctx.Query("end_time"); endTime != "" {
		if t, err := time.Parse("2006-01-02", endTime); err == nil {
			params["end_time"] = t
		}
	}
	if layer := ctx.Query("layer"); layer != "" {
		params["layer"] = layer
	}

	userID, isAdmin := extractUserContext(ctx)
	report, err := c.reportService.GetReport(uint(id), userID, isAdmin)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	data, err := c.reportService.QueryReportData(ctx, report, params)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, data, "获取成功")
}

// ExportCSV CSV 流式导出（D-4）
//
// csv.Writer 直写 ResponseWriter；行数 >30K 时拒绝同步导出并返回 400，
// 提示缩小时间范围/过滤条件（异步任务本期不做，决策源 M18 表 D-4）。
func (c *CustomReportController) ExportCSV(ginCtx *gin.Context) {

	idStr := ginCtx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ginCtx, http.StatusBadRequest, "无效的报表 ID")
		return
	}

	params := make(map[string]any)
	if startTime := ginCtx.Query("start_time"); startTime != "" {
		if t, err := time.Parse("2006-01-02", startTime); err == nil {
			params["start_time"] = t
		}
	}
	if endTime := ginCtx.Query("end_time"); endTime != "" {
		if t, err := time.Parse("2006-01-02", endTime); err == nil {
			params["end_time"] = t
		}
	}
	if layer := ginCtx.Query("layer"); layer != "" {
		params["layer"] = layer
	}

	userID, isAdmin := extractUserContext(ginCtx)
	report, err := c.reportService.GetReport(uint(id), userID, isAdmin)
	if err != nil {
		response.Error(ginCtx, http.StatusNotFound, err.Error())
		return
	}

	ginCtx.Header("Content-Type", "text/csv; charset=utf-8")
	ginCtx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="report_%d.csv"`, id))
	if err := c.reportService.ExportReportCSV(ginCtx.Request.Context(), ginCtx.Writer, report, params); err != nil {
		if errors.Is(err, service.ErrReportTooManyRows) {

			response.Error(ginCtx, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(ginCtx, http.StatusInternalServerError, err.Error())
		return
	}
}
