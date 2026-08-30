package controller

import (
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ManageRagEvalController 管理端 RAG 自动评测控制器
type ManageRagEvalController struct {
	svc *service.RagEvalAutoService
}

// NewManageRagEvalController 构造
func NewManageRagEvalController() *ManageRagEvalController {
	return &ManageRagEvalController{svc: service.NewRagEvalAutoService()}
}

// Run POST /api/manage/rag-eval/run
// body: {"name": "my_eval", "max_questions": 30}
func (c *ManageRagEvalController) Run(ctx *gin.Context) {
	var cfg service.RagEvalConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		cfg = service.RagEvalConfig{}
	}
	run, err := c.svc.RunAutoEvaluation(ctx.Request.Context(), &cfg)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, run, "评测完成")
}

// List GET /api/manage/rag-eval/runs
func (c *ManageRagEvalController) List(ctx *gin.Context) {
	runs, err := c.svc.ListRuns(ctx.Request.Context(), 20)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": runs, "total": len(runs)}, "ok")
}

// Detail GET /api/manage/rag-eval/runs/:id
func (c *ManageRagEvalController) Detail(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(ctx, 400, "无效的 id")
		return
	}
	run, questions, err := c.svc.GetRun(ctx.Request.Context(), uint(id))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"run": run, "questions": questions}, "ok")
}
