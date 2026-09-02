package controller

import (
	"context"
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// PromptController Prompt 版本管理 + A/B 实验控制器
// G13: 竞品标配功能
type PromptController struct {
	svc *service.PromptService
}

// NewPromptController 创建 Prompt 控制器
func NewPromptController() *PromptController {
	return &PromptController{svc: service.NewPromptService()}
}

// NewPromptControllerWithService 注入 Service（测试用）
func NewPromptControllerWithService(svc *service.PromptService) *PromptController {
	return &PromptController{svc: svc}
}

// GetVersions 获取某个 SOP Node / Prompt ID 的所有历史版本（prompt_candidates）
// GET /api/prompts/:id/versions?sop_node_id=xxx&status=active
func (c *PromptController) GetVersions(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 prompt id")
		return
	}

	status := ctx.Query("status")
	versions, err := c.svc.ListVersions(context.Background(), idStr, uint(id), "", status)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取 prompt 版本列表失败")
		return
	}
	response.SuccessWithList(ctx, versions, int64(len(versions)))
}

// PublishRequest 发布新版本请求
type PublishRequest struct {
	SystemPrompt       string `json:"system_prompt" binding:"required"`
	UserPromptTemplate string `json:"user_prompt_template" binding:"required"`
	SOPNodeID          string `json:"sop_node_id,omitempty"`
	SOPID              uint   `json:"sop_id,omitempty"`
	ImprovementNotes   string `json:"improvement_notes,omitempty"`
	Variables          string `json:"variables,omitempty"` // JSON 字符串
}

// Publish 发布新版本（从 draft → active，自动把旧版本降为 retired）
// POST /api/prompts/:id/publish
func (c *PromptController) Publish(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 prompt id")
		return
	}
	var req PublishRequest
	if !response.BindJSON(ctx, &req) {
		return
	}

	newVersion, err := c.svc.Publish(context.Background(), service.PublishRequest{
		SystemPrompt:       req.SystemPrompt,
		UserPromptTemplate: req.UserPromptTemplate,
		SOPNodeID:          req.SOPNodeID,
		SOPID:              req.SOPID,
		ImprovementNotes:   req.ImprovementNotes,
		Variables:          req.Variables,
		ParentID:           uint(id),
	})
	if err != nil {
		response.ErrorFromDB(ctx, err, "发布新版本失败")
		return
	}
	response.Success(ctx, newVersion, "发布成功")
}

// GetABExperiments 获取所有 Prompt A/B 实验列表
// GET /api/prompts/ab-experiments?status=running
func (c *PromptController) GetABExperiments(ctx *gin.Context) {
	status := ctx.Query("status")
	experiments, err := c.svc.ListABTests(context.Background(), status)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取 A/B 实验列表失败")
		return
	}
	response.SuccessWithList(ctx, experiments, int64(len(experiments)))
}
