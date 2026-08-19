package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/aiagent/llm"
)

// TraceController 全链路追踪控制器
//
//   - GET /api/trace/:traceId  返回单条 trace 的完整 span 明细（按时间排序）+ 概要统计。
//   - GET /api/traces/recent   返回最近 N 条 trace 概要（按末个 span 时间倒序）。
//
// 数据源：trace_events 表（由 m_p1_migration 建表，DBTraceSink 异步落库）。
// 此前为 501 占位；本次真实实现对前端 trace 监控面板提供查询能力。
type TraceController struct{}

func NewTraceController() *TraceController {
	return &TraceController{}
}

// GetTrace 查询 trace 详情
func (c *TraceController) GetTrace(ctx *gin.Context) {
	traceID := ctx.Param("traceId")
	if traceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "trace_id is required",
			"code":  "INVALID_PARAM",
		})
		return
	}
	detail, err := llm.QueryTrace(ctx.Request.Context(), traceID)
	if err != nil {
		if errors.Is(err, llm.ErrTraceNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error":    "trace not found",
				"code":     "NOT_FOUND",
				"trace_id": traceID,
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"code":  "INTERNAL_ERROR",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": detail,
	})
}

// ListRecent 查询最近 N 条 trace 概要（limit 可选，默认 50，上限 500）
func (c *TraceController) ListRecent(ctx *gin.Context) {
	limit := 50
	if v := ctx.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := llm.ListRecentTraces(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"code":  "INTERNAL_ERROR",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"code":  0,
		"msg":   "ok",
		"data":  items,
		"count": len(items),
	})
}
