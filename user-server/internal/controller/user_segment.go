package controller

import (
	"context"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UserSegmentController 用户分层控制器
type UserSegmentController struct {
	rfmService *service.RFMCalculatorService
}

// NewUserSegmentController 创建用户分层控制器
func NewUserSegmentController() *UserSegmentController {
	return &UserSegmentController{
		rfmService: service.NewRFMCalculatorService(),
	}
}

// GetRFMRule 获取 RFM 规则
func (c *UserSegmentController) GetRFMRule(ctx *gin.Context) {
	rule, err := c.rfmService.GetRFMRule(context.Background(), )
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "未找到 RFM 规则")
		return
	}

	response.Success(ctx, rule, "获取成功")
}

// SaveRFMRule 保存 RFM 规则
func (c *UserSegmentController) SaveRFMRule(ctx *gin.Context) {
	var req service.SaveRFMRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	rule, err := c.rfmService.SaveRFMRule(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, rule, "保存成功")
}

// UpdateRFMRule 更新 RFM 规则
func (c *UserSegmentController) UpdateRFMRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的规则 ID")
		return
	}

	var req service.SaveRFMRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	rule, err := c.rfmService.UpdateRFMRule(context.Background(), uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, rule, "更新成功")
}

// DeleteRFMRule 删除 RFM 规则
func (c *UserSegmentController) DeleteRFMRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的规则 ID")
		return
	}

	if HandleDBError(ctx, c.rfmService.DeleteRFMRule(context.Background(), uint(id)), "删除 RFM 规则") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetRFMList 获取用户 RFM 列表
func (c *UserSegmentController) GetRFMList(ctx *gin.Context) {
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

	layer := ctx.Query("layer")
	var rfms []*service.UserRFMWithUser
	var total int64
	var err error

	if layer != "" {
		// 按分层筛选
		rfms, total, err = c.rfmService.GetUsersByLayer(context.Background(), layer, page, pageSize)
	} else {
		rfms, total, err = c.rfmService.GetRFMList(context.Background(), page, pageSize)
	}
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      rfms,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetRFMStats 获取 RFM 统计
func (c *UserSegmentController) GetRFMStats(ctx *gin.Context) {
	stats, err := c.rfmService.GetRFMStats(context.Background(), )
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, stats, "获取成功")
}

// CalculateRFM 手动触发 RFM 计算
func (c *UserSegmentController) CalculateRFM(ctx *gin.Context) {
	count, err := c.rfmService.CalculateAllUsers(ctx)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"updated_count": count,
	}, "计算完成")
}

// GetUserRFM 获取单个用户的 RFM
func (c *UserSegmentController) GetUserRFM(ctx *gin.Context) {
	userIDStr := ctx.Query("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 user_id")
		return
	}

	rfm, err := c.rfmService.GetUserRFM(context.Background(), uint(userID))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "未找到用户 RFM 信息")
		return
	}

	response.Success(ctx, rfm, "获取成功")
}

// GetLayerDescription 获取分层说明
func (c *UserSegmentController) GetLayerDescription(ctx *gin.Context) {
	layers := []map[string]string{
		{"layer": "important_value", "name": "重要价值用户", "desc": "最近消费、消费频次高、消费金额高"},
		{"layer": "important_keep", "name": "重要保持用户", "desc": "很久未消费、消费频次高、消费金额高"},
		{"layer": "important_develop", "name": "重要发展用户", "desc": "最近消费、消费频次低、消费金额高"},
		{"layer": "important_stay", "name": "重要挽留用户", "desc": "很久未消费、消费频次低、消费金额高"},
		{"layer": "general_value", "name": "一般价值用户", "desc": "最近消费、消费频次高、消费金额低"},
		{"layer": "general_keep", "name": "一般保持用户", "desc": "很久未消费、消费频次高、消费金额低"},
		{"layer": "general_develop", "name": "一般发展用户", "desc": "最近消费、消费频次低、消费金额低"},
		{"layer": "general_stay", "name": "一般挽留用户", "desc": "很久未消费、消费频次低、消费金额低"},
		{"layer": "new", "name": "新用户", "desc": "首次消费"},
		{"layer": "sleep", "name": "沉睡用户", "desc": "超过 60 天未消费"},
		{"layer": "lost", "name": "流失用户", "desc": "超过 90 天未消费"},
	}

	response.Success(ctx, layers, "获取成功")
}
