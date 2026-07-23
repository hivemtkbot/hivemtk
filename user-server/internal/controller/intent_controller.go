package controller

import (
	"context"
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// IntentController 意图识别控制器
type IntentController struct {
	rec *service.IntentRecognizer
}

// NewIntentController 创建意图识别控制器
func NewIntentController(rec *service.IntentRecognizer) *IntentController {
	return &IntentController{
		rec: rec,
	}
}

// RecognizeRequest 识别请求
type RecognizeRequest struct {
	SessionID  string `json:"session_id"`
	CustomerID string `json:"customer_id"`
	Text       string `json:"text" binding:"required"`
}

// Recognize 单条识别
func (c *IntentController) Recognize(ctx *gin.Context) {
	var req RecognizeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := c.rec.Recognize(ctx.Request.Context(), req.SessionID, req.CustomerID, req.Text)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, result, "识别成功")
}

// BatchRecognizeRequest 批量识别请求
type BatchRecognizeRequest struct {
	Items []RecognizeRequest `json:"items" binding:"required"`
}

// BatchRecognize 批量识别
func (c *IntentController) BatchRecognize(ctx *gin.Context) {
	var req BatchRecognizeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	out := make([]*dto.RecognizeResult, 0, len(req.Items))
	for _, item := range req.Items {
		r, err := c.rec.Recognize(ctx.Request.Context(), item.SessionID, item.CustomerID, item.Text)
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	response.Success(ctx, out, "批量识别成功")
}

// Stats 意图统计
func (c *IntentController) Stats(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	stats, err := c.rec.GetIntentStats(context.Background(), days)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "查询成功")
}

// RecentIntents 客户近期意图
func (c *IntentController) RecentIntents(ctx *gin.Context) {
	customerID := ctx.Query("customer_id")
	_, limit, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	list, err := c.rec.GetRecentIntents(context.Background(), customerID, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// Intents 意图词典
func (c *IntentController) Intents(ctx *gin.Context) {
	response.Success(ctx, service.DefaultIntents, "查询成功")
}

// ===== M-2 P1：精细意图识别（8 大类 + 7 子类）=====

// RecognizeFineRequest 精细识别请求
type RecognizeFineRequest struct {
	Message    string `json:"message" binding:"required"`
	CustomerID string `json:"customer_id"`
	SessionID  string `json:"session_id"`
}

// RecognizeFine 精细意图识别
// POST /api/intent/recognize/fine
func (c *IntentController) RecognizeFine(ctx *gin.Context) {
	var req RecognizeFineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := c.rec.RecognizeIntent(ctx.Request.Context(), req.Message, req.CustomerID, req.SessionID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, result, "识别成功")
}

// IntentLogs 查询意图识别日志
// GET /api/intent/logs?customer_id=xxx&major=xxx&limit=100
func (c *IntentController) IntentLogs(ctx *gin.Context) {
	customerID := ctx.Query("customer_id")
	major := ctx.Query("major")
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "100"))
	logs, err := c.rec.GetIntentLogs(context.Background(), customerID, major, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, logs, "查询成功")
}

// IntentStatsFine 精细意图统计
// GET /api/intent/stats/fine?days=7
func (c *IntentController) IntentStatsFine(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	stats, err := c.rec.GetIntentLogStats(context.Background(), days)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "查询成功")
}
