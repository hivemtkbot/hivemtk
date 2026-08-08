package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service/trace_learning"

	"github.com/gin-gonic/gin"
)

// TraceLearningController 追踪自学习 API（手动触发评估 + 查询打分/权重）
type TraceLearningController struct {
	svc *trace_learning.Service
}

// NewTraceLearningController 构造控制器
func NewTraceLearningController(svc *trace_learning.Service) *TraceLearningController {
	return &TraceLearningController{svc: svc}
}

// TriggerEval POST /api/monitor/trace-eval/trigger?hours=24&limit=20
// 手动触发：扫描最近 hours 小时内未评估的 trace，批量打分并调整知识库权重。
func (c *TraceLearningController) TriggerEval(ctx *gin.Context) {
	hours, _ := strconv.Atoi(ctx.DefaultQuery("hours", "24"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	n, err := c.svc.RunBatch(ctx.Request.Context(), hours, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"processed": n}, "ok")
}

// EvalLogs GET /api/monitor/trace-eval/logs?limit=50
// 查询最近打分审计记录（供前端展示「自学习打分」）。
func (c *TraceLearningController) EvalLogs(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	logs, err := c.svc.Logs(ctx.Request.Context(), limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, logs, "ok")
}

// KnowledgeWeights GET /api/monitor/knowledge-weights?limit=50
// 查询知识库权重排行（权重偏离 1.0 最大的 chunk，供前端展示「自学习影响」）。
func (c *TraceLearningController) KnowledgeWeights(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	rows, err := c.svc.TopWeights(ctx.Request.Context(), limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, rows, "ok")
}
