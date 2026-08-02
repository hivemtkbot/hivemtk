package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TraceController 全链路追踪控制器（占位）
//
// TODO: 完整实现 trace 查询/聚合
// 当前仅返回 501，让路由可达、build 通过
type TraceController struct{}

func NewTraceController() *TraceController {
	return &TraceController{}
}

// GetTrace 查询 trace 详情
func (c *TraceController) GetTrace(ctx *gin.Context) {
	ctx.JSON(http.StatusNotImplemented, gin.H{
		"error":    "Trace endpoint not yet implemented",
		"code":     "NOT_IMPLEMENTED",
		"trace_id": ctx.Param("traceId"),
	})
}
