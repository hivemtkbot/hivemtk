package controller

import (
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// safeDiv 安全除法（避免除 0 触发 panic）
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// tuning_controller.go 置信度/拟人度/反馈学习 统一管理 API Controller
//
// 五层架构归属: L3 表现层
// 全部数据访问通过 TuningService,不再持有任何 Repository。
//
// 路由前缀: /api/admin/tuning/

// TuningController 统一管理 Controller
type TuningController struct {
	svc service.TuningService
}

// NewTuningController 构造
func NewTuningController(svc service.TuningService) *TuningController {
	return &TuningController{svc: svc}
}

// ----------------------------------------------------------------------------
// 1. 置信度信号 (ConfidenceSignal)
// ----------------------------------------------------------------------------

// ListConfidenceSignals 信号列表
func (c *TuningController) ListConfidenceSignals(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	sessionID := ctx.Query("session_id")

	rows, total, err := c.svc.ListConfidenceSignals(ctx.Request.Context(), sessionID, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// GetConfidenceSignal 信号详情
func (c *TuningController) GetConfidenceSignal(ctx *gin.Context) {
	id := ctx.Param("id")
	row, err := c.svc.GetConfidenceSignal(ctx.Request.Context(), id)
	if err != nil {
		response.NotFoundError(ctx, "signal")
		return
	}
	response.Success(ctx, row, "")
}

// StatsConfidenceSignals 聚合统计
func (c *TuningController) StatsConfidenceSignals(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	rows, err := c.svc.StatsConfidenceSignals(ctx.Request.Context(), since)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"days": days, "bands": rows}, "")
}

// ----------------------------------------------------------------------------
// 2. 置信度校准 (ConfidenceCalibration)
// ----------------------------------------------------------------------------

// ListCalibrations 校准记录列表
func (c *TuningController) ListCalibrations(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	signalType := ctx.Query("signal_type")

	rows, total, err := c.svc.ListCalibrations(ctx.Request.Context(), signalType, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 3. 阈值策略 (ThresholdPolicy)
// ----------------------------------------------------------------------------

// ListThresholdPolicies 策略列表
func (c *TuningController) ListThresholdPolicies(ctx *gin.Context) {
	rows, err := c.svc.ListThresholdPolicies(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"list": rows}, "")
}

// UpsertThresholdPolicy 创建/更新策略
func (c *TuningController) UpsertThresholdPolicy(ctx *gin.Context) {
	var body dto.ThresholdPolicyRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		response.Error(ctx, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	policy := body.ToModel()
	if err := c.svc.UpsertThresholdPolicy(ctx.Request.Context(), policy); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "save failed: "+err.Error())
		return
	}
	response.Success(ctx, policy, "")
}

// ----------------------------------------------------------------------------
// 4. 拟人度评分 (HumanizeScore)
// ----------------------------------------------------------------------------

// ListHumanizeScores 评分列表
func (c *TuningController) ListHumanizeScores(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	sessionID := ctx.Query("session_id")
	passed := ctx.Query("passed")

	var passedPtr *bool
	if passed == "true" {
		v := true
		passedPtr = &v
	} else if passed == "false" {
		v := false
		passedPtr = &v
	}

	rows, total, err := c.svc.ListHumanizeScores(ctx.Request.Context(), sessionID, passedPtr, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// StatsHumanizeScores 拟人度统计
func (c *TuningController) StatsHumanizeScores(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	stat, err := c.svc.StatsHumanizeScores(ctx.Request.Context(), since)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"days":		days,
		"avg_score":	stat.AvgScore,
		"passed_count":	stat.Passed,
		"failed_count":	stat.Failed,
		"total_count":	stat.Total,
		"pass_rate":	safeDiv(float64(stat.Passed), float64(stat.Total)),
	}, "")
}

// ----------------------------------------------------------------------------
// 5. 销冠基线 (ChampionBaseline)
// ----------------------------------------------------------------------------

// ListChampionBaselines 基线列表
func (c *TuningController) ListChampionBaselines(ctx *gin.Context) {
	rows, err := c.svc.ListChampionBaselines(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"list": rows}, "")
}

// ----------------------------------------------------------------------------
// 6. 反馈事件 (FeedbackEvent)
// ----------------------------------------------------------------------------

// ListFeedbackEvents 事件列表
func (c *TuningController) ListFeedbackEvents(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	sessionID := ctx.Query("session_id")
	signalKey := ctx.Query("signal_key")

	rows, total, err := c.svc.ListFeedbackEvents(ctx.Request.Context(), sessionID, signalKey, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// StatsFeedbackEvents 反馈事件统计
func (c *TuningController) StatsFeedbackEvents(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	rows, err := c.svc.StatsFeedbackEvents(ctx.Request.Context(), since)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"days": days, "signals": rows}, "")
}

// ----------------------------------------------------------------------------
// 7. 销冠对话 (ChampionDialogue)
// ----------------------------------------------------------------------------

// ListChampionDialogues 销冠对话列表
func (c *TuningController) ListChampionDialogues(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	intent := ctx.Query("intent")
	industry := ctx.Query("industry")

	rows, total, err := c.svc.ListChampionDialogues(ctx.Request.Context(), intent, industry, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 8. Prompt 候选 (PromptCandidate)
// ----------------------------------------------------------------------------

// ListPromptCandidates 候选列表
func (c *TuningController) ListPromptCandidates(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	status := ctx.Query("status")

	rows, total, err := c.svc.ListPromptCandidates(ctx.Request.Context(), status, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// UpdatePromptCandidateStatus 更新候选状态
// 注意: 前端通过请求体 {status:'approved'} 传递(见 api/tuning.js updatePromptCandidateStatus), 故从 body 绑定
func (c *TuningController) UpdatePromptCandidateStatus(ctx *gin.Context) {
	id := ctx.Param("id")
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "status required")
		return
	}
	if err := c.svc.UpdatePromptCandidateStatus(ctx.Request.Context(), id, req.Status); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "update failed: "+err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id, "status": req.Status}, "")
}

// ----------------------------------------------------------------------------
// 9. Bandit 臂 (BanditArm)
// ----------------------------------------------------------------------------

// ListBanditArms 臂列表
func (c *TuningController) ListBanditArms(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	experimentID := ctx.Query("experiment_id")
	sopID := ctx.Query("sop_id")

	rows, total, err := c.svc.ListBanditArms(ctx.Request.Context(), experimentID, sopID, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 11. 低质样本 (LowQualitySample)
// ----------------------------------------------------------------------------

// ListLowQualitySamples 低质样本列表
func (c *TuningController) ListLowQualitySamples(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	sampleType := ctx.Query("sample_type")

	rows, total, err := c.svc.ListLowQualitySamples(ctx.Request.Context(), sampleType, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 通用助手
// ----------------------------------------------------------------------------

// jsonList 通用列表响应(分页元数据)
func (c *TuningController) jsonList(ctx *gin.Context, list any, total int64, page, pageSize int) {
	response.Success(ctx, gin.H{
		"list":		list,
		"total":	total,
		"page":		page,
		"size":		pageSize,
	}, "")
}
