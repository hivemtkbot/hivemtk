package controller

import (
	"context"
	"net/http"
	"strconv"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PromptController Prompt 版本管理 + A/B 实验控制器
// G13: 竞品标配功能
type PromptController struct {
	db *gorm.DB
}

// NewPromptController 创建 Prompt 控制器
func NewPromptController() *PromptController {
	return &PromptController{db: db.GetDB()}
}

// NewPromptControllerWithDB 注入 DB（测试用）
func (c *PromptController) WithDB(d *gorm.DB) *PromptController {
	c.db = d
	return c
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

	q := c.db.WithContext(context.Background()).Model(&model.PromptCandidate{}).Where("id = ? OR sop_id = ? OR sop_node_id = ?", idStr, uint(id), idStr)

	if status := ctx.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var versions []model.PromptCandidate
	if err := q.Order("created_at DESC").Find(&versions).Error; err != nil {
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

	// 把当前 active 的同 node 版本降为 retired
	if req.SOPNodeID != "" {
		c.db.WithContext(context.Background()).
			Model(&model.PromptCandidate{}).
			Where("sop_node_id = ? AND status = ?", req.SOPNodeID, model.PromptCandidateStatusActive).
			Update("status", model.PromptCandidateStatusRetired)
	}

	// 创建新版本
	parentID := uint(id)
	newVersion := model.PromptCandidate{
		SOPNodeID:          req.SOPNodeID,
		SOPID:              req.SOPID,
		SystemPrompt:       req.SystemPrompt,
		UserPromptTemplate: req.UserPromptTemplate,
		ImprovementNotes:   req.ImprovementNotes,
		Status:             model.PromptCandidateStatusActive,
		ParentID:           parentID,
	}
	if err := c.db.WithContext(context.Background()).Create(&newVersion).Error; err != nil {
		response.ErrorFromDB(ctx, err, "发布新版本失败")
		return
	}
	response.Success(ctx, newVersion, "发布成功")
}

// GetABExperiments 获取所有 Prompt A/B 实验列表
// GET /api/prompts/ab-experiments?status=running
func (c *PromptController) GetABExperiments(ctx *gin.Context) {
	q := c.db.WithContext(context.Background()).Model(&model.PromptABTest{})
	if status := ctx.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var experiments []model.PromptABTest
	if err := q.Order("created_at DESC").Find(&experiments).Error; err != nil {
		response.ErrorFromDB(ctx, err, "获取 A/B 实验列表失败")
		return
	}
	response.SuccessWithList(ctx, experiments, int64(len(experiments)))
}
