package controller

// tuning_controller.go 置信度/拟人度/反馈学习 统一管理 API Controller
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md 第十五章 §15.5 + 第十六章 §16.5 + 第十七章 §17.5
//
// 统一管理以下模块的 CRUD/查询接口：
//   1. 置信度信号 (ConfidenceSignal)      -> repository.ConfidenceSignalRepository
//   2. 置信度校准 (ConfidenceCalibration) -> repository.ConfidenceCalibrationRepository
//   3. 阈值策略 (ThresholdPolicy)         -> repository.ThresholdPolicyRepository
//   4. 拟人度评分 (HumanizeScore)         -> repository.HumanizeScoreRepository
//   5. 销冠基线 (ChampionBaseline)         -> repository.ChampionBaselineRepositoryImpl
//   6. 反馈事件 (FeedbackEvent)           -> repository.FeedbackLoopRepository
//   7. 销冠对话 (ChampionDialogue)         -> repository.FeedbackLoopRepository
//   8. Prompt 候选 (PromptCandidate)      -> repository.FeedbackLoopRepository
//   9. Bandit 臂 (BanditArm)              -> repository.FeedbackLoopRepository
//  10. 低质样本 (LowQualitySample)         -> repository.LowQualitySampleRepository
//
// 分层规约：controller 不再持有 *gorm.DB，所有数据访问按业务域下沉到对应
// repository（controller -> repository -> model），禁止在网关层直连数据库。
//
// 路由前缀：/api/admin/tuning/

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// TuningController 统一管理 Controller
//
// 各域数据访问通过已存在的 domain repository 注入；repository 均为无状态单例，
// 由构造器内部构建（与仓库包其他仓储约定一致，避免在 controller 引入 *gorm.DB）。
type TuningController struct {
	signalRepo   *repository.ConfidenceSignalRepository
	calibRepo    *repository.ConfidenceCalibrationRepository
	policyRepo   *repository.ThresholdPolicyRepository
	scoreRepo    *repository.HumanizeScoreRepository
	baselineRepo *repository.ChampionBaselineRepositoryImpl
	lowQRepo     *repository.LowQualitySampleRepository
	feedbackRepo *repository.FeedbackLoopRepository
}

// NewTuningController 构造
func NewTuningController() *TuningController {
	return &TuningController{
		signalRepo:   repository.NewConfidenceSignalRepository(),
		calibRepo:    repository.NewConfidenceCalibrationRepository(),
		policyRepo:   repository.NewThresholdPolicyRepository(),
		scoreRepo:    repository.NewHumanizeScoreRepository(),
		baselineRepo: repository.NewChampionBaselineRepository(),
		lowQRepo:     repository.NewLowQualitySampleRepository(),
		feedbackRepo: repository.NewFeedbackLoopRepository(),
	}
}

// ----------------------------------------------------------------------------
// 1. 置信度信号 (ConfidenceSignal)
// ----------------------------------------------------------------------------

// ListConfidenceSignals 信号列表
func (c *TuningController) ListConfidenceSignals(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	sessionID := ctx.Query("session_id")

	rows, total, err := c.signalRepo.List(ctx.Request.Context(), sessionID, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// GetConfidenceSignal 信号详情
func (c *TuningController) GetConfidenceSignal(ctx *gin.Context) {
	id := ctx.Param("id")
	row, err := c.signalRepo.GetByID(ctx.Request.Context(), id)
	if err != nil {
		c.jsonError(ctx, http.StatusNotFound, "not found")
		return
	}
	c.jsonData(ctx, row)
}

// StatsConfidenceSignals 聚合统计
func (c *TuningController) StatsConfidenceSignals(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	rows, err := c.signalRepo.StatsByBand(ctx.Request.Context(), since)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{"days": days, "bands": rows})
}

// ----------------------------------------------------------------------------
// 2. 置信度校准 (ConfidenceCalibration)
// ----------------------------------------------------------------------------

// ListCalibrations 校准记录列表
func (c *TuningController) ListCalibrations(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	signalType := ctx.Query("signal_type")

	rows, total, err := c.calibRepo.List(ctx.Request.Context(), signalType, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 3. 阈值策略 (ThresholdPolicy)
// ----------------------------------------------------------------------------

// ListThresholdPolicies 策略列表
func (c *TuningController) ListThresholdPolicies(ctx *gin.Context) {
	rows, err := c.policyRepo.ListActive(ctx.Request.Context())
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{"list": rows})
}

// UpsertThresholdPolicy 创建/更新策略
func (c *TuningController) UpsertThresholdPolicy(ctx *gin.Context) {
	var body model.ThresholdPolicy
	if err := ctx.ShouldBindJSON(&body); err != nil {
		c.jsonError(ctx, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	body.IsActive = true
	body.UpdatedAt = time.Now()
	if err := c.policyRepo.Save(ctx.Request.Context(), &body); err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "save failed: "+err.Error())
		return
	}
	c.jsonData(ctx, body)
}

// ----------------------------------------------------------------------------
// 4. 拟人度评分 (HumanizeScore)
// ----------------------------------------------------------------------------

// ListHumanizeScores 评分列表
func (c *TuningController) ListHumanizeScores(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
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

	rows, total, err := c.scoreRepo.List(ctx.Request.Context(), sessionID, passedPtr, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
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

	stat, err := c.scoreRepo.Stats(ctx.Request.Context(), since)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{
		"days":         days,
		"avg_score":    stat.AvgScore,
		"passed_count": stat.Passed,
		"failed_count": stat.Failed,
		"total_count":  stat.Total,
		"pass_rate":    safeDiv(float64(stat.Passed), float64(stat.Total)),
	})
}

// ----------------------------------------------------------------------------
// 5. 销冠基线 (ChampionBaseline)
// ----------------------------------------------------------------------------

// ListChampionBaselines 基线列表
func (c *TuningController) ListChampionBaselines(ctx *gin.Context) {
	rows, err := c.baselineRepo.ListEnabledModels(ctx.Request.Context())
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{"list": rows})
}

// ----------------------------------------------------------------------------
// 6. 反馈事件 (FeedbackEvent)
// ----------------------------------------------------------------------------

// ListFeedbackEvents 事件列表
func (c *TuningController) ListFeedbackEvents(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	sessionID := ctx.Query("session_id")
	signalKey := ctx.Query("signal_key")

	rows, total, err := c.feedbackRepo.ListFeedbackEvents(ctx.Request.Context(), sessionID, signalKey, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
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

	rows, err := c.feedbackRepo.StatsFeedbackEvents(ctx.Request.Context(), since)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "stats failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{"days": days, "signals": rows})
}

// ----------------------------------------------------------------------------
// 7. 销冠对话 (ChampionDialogue)
// ----------------------------------------------------------------------------

// ListChampionDialogues 销冠对话列表
func (c *TuningController) ListChampionDialogues(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	intent := ctx.Query("intent")
	industry := ctx.Query("industry")

	rows, total, err := c.feedbackRepo.ListChampionDialogues(ctx.Request.Context(), intent, industry, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 8. Prompt 候选 (PromptCandidate)
// ----------------------------------------------------------------------------

// ListPromptCandidates 候选列表
func (c *TuningController) ListPromptCandidates(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	status := ctx.Query("status")

	rows, total, err := c.feedbackRepo.ListPromptCandidates(ctx.Request.Context(), status, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// UpdatePromptCandidateStatus 更新候选状态
func (c *TuningController) UpdatePromptCandidateStatus(ctx *gin.Context) {
	id := ctx.Param("id")
	status := ctx.Query("status")
	if status == "" {
		c.jsonError(ctx, http.StatusBadRequest, "status required")
		return
	}
	if err := c.feedbackRepo.UpdatePromptCandidateStatus(ctx.Request.Context(), id, status); err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "update failed: "+err.Error())
		return
	}
	c.jsonData(ctx, gin.H{"id": id, "status": status})
}

// ----------------------------------------------------------------------------
// 9. Bandit 臂 (BanditArm)
// ----------------------------------------------------------------------------

// ListBanditArms 臂列表
func (c *TuningController) ListBanditArms(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	experimentID := ctx.Query("experiment_id")
	sopID := ctx.Query("sop_id")

	rows, total, err := c.feedbackRepo.ListBanditArms(ctx.Request.Context(), experimentID, sopID, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 11. 低质样本 (LowQualitySample)
// ----------------------------------------------------------------------------

// ListLowQualitySamples 低质样本列表
func (c *TuningController) ListLowQualitySamples(ctx *gin.Context) {
	page, pageSize := c.pagination(ctx)
	sampleType := ctx.Query("sample_type")

	rows, total, err := c.lowQRepo.List(ctx.Request.Context(), sampleType, page, pageSize)
	if err != nil {
		c.jsonError(ctx, http.StatusInternalServerError, "list failed: "+err.Error())
		return
	}
	c.jsonList(ctx, rows, total, page, pageSize)
}

// ----------------------------------------------------------------------------
// 通用助手
// ----------------------------------------------------------------------------

// pagination 解析分页参数
func (c *TuningController) pagination(ctx *gin.Context) (int, int) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return page, pageSize
}

// jsonList 通用列表响应
func (c *TuningController) jsonList(ctx *gin.Context, list any, total int64, page, pageSize int) {
	ctx.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"list":  list,
			"total": total,
			"page":  page,
			"size":  pageSize,
		},
	})
}

// jsonData 通用数据响应
func (c *TuningController) jsonData(ctx *gin.Context, data any) {
	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// jsonError 通用错误响应
func (c *TuningController) jsonError(ctx *gin.Context, status int, msg string) {
	// 抑制 strings 包被裁剪（用于占位）
	_ = strings.TrimSpace
	ctx.JSON(status, gin.H{"code": status, "message": msg})
}

// safeDiv 安全除法
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
