package controller

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// OperationLogController 操作日志控制器
//
// 通过 service.OperationLogService 访问数据，遵循五层架构。
type OperationLogController struct {
	logSvc *service.OperationLogService
}

// NewOperationLogController 创建操作日志控制器实例
func NewOperationLogController() *OperationLogController {
	return &OperationLogController{
		logSvc: service.NewOperationLogService(),
	}
}

// GetList 获取操作日志列表
func (c *OperationLogController) GetList(ctx *gin.Context) {
	// 获取分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 获取过滤条件
	filters := make(map[string]any)
	if userID := ctx.Query("user_id"); userID != "" {
		filters["user_id"] = userID
	}
	if action := ctx.Query("action"); action != "" {
		filters["action"] = action
	}
	if module := ctx.Query("module"); module != "" {
		filters["module"] = module
	}
	if startTime := ctx.Query("start_time"); startTime != "" {
		filters["start_time"] = startTime
	}
	if endTime := ctx.Query("end_time"); endTime != "" {
		filters["end_time"] = endTime
	}

	// 获取日志列表（service 返回 OperationLogView DTO）
	logs, total, err := c.logSvc.GetAll(ctx, page, pageSize, filters)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取操作日志成功")
}

// GetByID 获取操作日志详情
func (c *OperationLogController) GetByID(ctx *gin.Context) {
	// 获取日志ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的日志ID")
		return
	}

	// 获取日志详情（service 在不存在时返回 nil, nil）
	log, err := c.logSvc.GetByID(ctx, uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "日志不存在")
		return
	}
	if log == nil {
		response.Error(ctx, http.StatusNotFound, "日志不存在")
		return
	}

	response.Success(ctx, log, "获取日志详情成功")
}

// GetMyLogs 获取当前用户的操作日志
func (c *OperationLogController) GetMyLogs(ctx *gin.Context) {
	// 获取用户ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		// 尝试从 float64 转换
		if f, ok := userID.(float64); ok {
			uid = uint(f)
		}
	}

	// 获取分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 获取日志列表
	logs, total, err := c.logSvc.GetByUserID(ctx, uid, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取操作日志成功")
}

// GetStatistics 获取操作日志统计
func (c *OperationLogController) GetStatistics(ctx *gin.Context) {
	// service 已经封装好统计逻辑，直接返回 DTO
	stats, err := c.logSvc.GetStatistics(ctx)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"total":        stats.Total,
		"module_stats": stats.ModuleStats,
		"action_stats": stats.ActionStats,
		"user_stats":   stats.UserStats,
	}, "获取统计信息成功")
}

// ExportLogs 导出操作日志(返回 CSV 文件流)
// 支持 query: start_date, end_date, module
func (c *OperationLogController) ExportLogs(ctx *gin.Context) {
	pageSizeStr := ctx.DefaultQuery("page_size", "10000")
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 || pageSize > 50000 {
		pageSize = 10000
	}

	// service 返回 OperationLogView 列表，controller 负责渲染 CSV
	logs, err := c.logSvc.ExportAll(ctx, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "查询日志失败: "+err.Error())
		return
	}

	// 构造 CSV
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"ID", "Module", "Action", "Username", "IP", "CreatedAt"})
	for _, log := range logs {
		_ = writer.Write([]string{
			fmt.Sprintf("%d", log.ID),
			log.Module,
			log.Action,
			log.Username,
			log.IP,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()

	filename := fmt.Sprintf("operation_logs_%s.csv", time.Now().Format("20060102_150405"))
	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	ctx.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// CleanLogs 清理指定日期之前的操作日志
// 入参 JSON: { before_date: "" } 或 { days: 90 }
func (c *OperationLogController) CleanLogs(ctx *gin.Context) {
	var req struct {
		BeforeDate string `json:"before_date"`
		Days       int    `json:"days"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	var cutoff time.Time
	if req.BeforeDate != "" {
		t, err := time.Parse("2006-01-02", req.BeforeDate)
		if err != nil {
			response.Error(ctx, http.StatusBadRequest, "日期格式错误,需要 YYYY-MM-DD")
			return
		}
		cutoff = t
	} else if req.Days > 0 {
		cutoff = time.Now().AddDate(0, 0, -req.Days)
	}
	if err := c.logSvc.DeleteOldLogs(ctx, cutoff); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "清理失败: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"before": cutoff.Format("2006-01-02 15:04:05"),
	}, "清理完成")
}

// DeleteLogs 批量删除操作日志
// 入参 JSON: { ids: [1, 2, 3] }
func (c *OperationLogController) DeleteLogs(ctx *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	if len(req.IDs) == 0 {
		response.Error(ctx, http.StatusBadRequest, "请选择要删除的日志")
		return
	}

	count, err := c.logSvc.DeleteByIDs(ctx, req.IDs)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "删除失败: "+err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"requested":     len(req.IDs),
		"deleted_count": count,
	}, "删除成功")
}
