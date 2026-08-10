package service

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// ============================================================================
// TuningService 置信度/拟人度/反馈学习 统一服务
// ============================================================================
//
// 五层架构归属: L4 业务层
// 用途: 收敛所有 tuning 相关的数据访问,Controller 不再直接持有 Repository。
// 依赖: 8 个 tuning 仓储(已在 internal/repository 内实现)。
//
// 设计依据: docs/核心链路优化.md 第十五章 §15.5 + 第十六章 §16.5 + 第十七章 §17.5
// ============================================================================

// TuningService 统一管理服务接口
//
// 所有列表方法返回 **值切片**（[]T），与下层 Repository 保持一致；
// 单条记录仍用 *T 指针。
type TuningService interface {
	// 1. 置信度信号 (ConfidenceSignal)
	ListConfidenceSignals(ctx context.Context, sessionID string, page, pageSize int) ([]model.ConfidenceSignal, int64, error)
	GetConfidenceSignal(ctx context.Context, id string) (*model.ConfidenceSignal, error)
	StatsConfidenceSignals(ctx context.Context, since time.Time) (map[string]int64, error)

	// 2. 置信度校准 (ConfidenceCalibration)
	ListCalibrations(ctx context.Context, signalType string, page, pageSize int) ([]model.ConfidenceCalibration, int64, error)

	// 3. 阈值策略 (ThresholdPolicy)
	ListThresholdPolicies(ctx context.Context) ([]model.ThresholdPolicy, error)
	UpsertThresholdPolicy(ctx context.Context, p *model.ThresholdPolicy) error

	// 4. 拟人度评分 (HumanizeScore)
	ListHumanizeScores(ctx context.Context, sessionID string, passed *bool, page, pageSize int) ([]model.HumanizeScore, int64, error)
	StatsHumanizeScores(ctx context.Context, since time.Time) (*HumanizeStats, error)

	// 5. 销冠基线 (ChampionBaseline)
	ListChampionBaselines(ctx context.Context) ([]model.ChampionBaseline, error)

	// 6. 反馈事件 (FeedbackEvent)
	ListFeedbackEvents(ctx context.Context, sessionID, signalKey string, page, pageSize int) ([]model.FeedbackEvent, int64, error)
	StatsFeedbackEvents(ctx context.Context, since time.Time) (map[string]int64, error)

	// 7. 销冠对话 (ChampionDialogue)
	ListChampionDialogues(ctx context.Context, intent, industry string, page, pageSize int) ([]model.ChampionDialogue, int64, error)

	// 8. Prompt 候选 (PromptCandidate)
	ListPromptCandidates(ctx context.Context, status string, page, pageSize int) ([]model.PromptCandidate, int64, error)
	UpdatePromptCandidateStatus(ctx context.Context, id, status string) error

	// 9. Bandit 臂 (BanditArm)
	ListBanditArms(ctx context.Context, experimentID, sopID string, page, pageSize int) ([]model.BanditArm, int64, error)

	// 11. 低质样本 (LowQualitySample)
	ListLowQualitySamples(ctx context.Context, sampleType string, page, pageSize int) ([]model.LowQualitySample, int64, error)
}

// HumanizeStats 拟人度统计结果
type HumanizeStats struct {
	AvgScore float64
	Passed   int64
	Failed   int64
	Total    int64
}

// tuningService 实现
type tuningService struct {
	signalRepo   *repository.ConfidenceSignalRepository
	calibRepo    *repository.ConfidenceCalibrationRepository
	policyRepo   *repository.ThresholdPolicyRepository
	scoreRepo    *repository.HumanizeScoreRepository
	baselineRepo *repository.ChampionBaselineRepositoryImpl
	lowQRepo     *repository.LowQualitySampleRepository
	feedbackRepo *repository.FeedbackLoopRepository
}

// NewTuningService 构造服务(注入所有 tuning 仓储)
func NewTuningService() TuningService {
	return &tuningService{
		signalRepo:   repository.NewConfidenceSignalRepository(),
		calibRepo:    repository.NewConfidenceCalibrationRepository(),
		policyRepo:   repository.NewThresholdPolicyRepository(),
		scoreRepo:    repository.NewHumanizeScoreRepository(),
		baselineRepo: repository.NewChampionBaselineRepository(),
		lowQRepo:     repository.NewLowQualitySampleRepository(),
		feedbackRepo: repository.NewFeedbackLoopRepository(),
	}
}

// ----- 1. ConfidenceSignal -----

func (s *tuningService) ListConfidenceSignals(ctx context.Context, sessionID string, page, pageSize int) ([]model.ConfidenceSignal, int64, error) {
	return s.signalRepo.List(ctx, sessionID, page, pageSize)
}

func (s *tuningService) GetConfidenceSignal(ctx context.Context, id string) (*model.ConfidenceSignal, error) {
	return s.signalRepo.GetByID(ctx, id)
}

// StatsConfidenceSignals 统计各决策区间的信号数
//
// 仓储层返回 []ConfidenceBandStat，每个元素含 Band/Count。
// Service 层做 band → count 映射后回吐给 Controller。
func (s *tuningService) StatsConfidenceSignals(ctx context.Context, since time.Time) (map[string]int64, error) {
	rows, err := s.signalRepo.StatsByBand(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.DecisionBand] = r.Count
	}
	return out, nil
}

// ----- 2. ConfidenceCalibration -----

func (s *tuningService) ListCalibrations(ctx context.Context, signalType string, page, pageSize int) ([]model.ConfidenceCalibration, int64, error) {
	return s.calibRepo.List(ctx, signalType, page, pageSize)
}

// ----- 3. ThresholdPolicy -----

func (s *tuningService) ListThresholdPolicies(ctx context.Context) ([]model.ThresholdPolicy, error) {
	return s.policyRepo.ListActive(ctx)
}

func (s *tuningService) UpsertThresholdPolicy(ctx context.Context, p *model.ThresholdPolicy) error {
	p.IsActive = true
	p.UpdatedAt = time.Now()
	return s.policyRepo.Save(ctx, p)
}

// ----- 4. HumanizeScore -----

func (s *tuningService) ListHumanizeScores(ctx context.Context, sessionID string, passed *bool, page, pageSize int) ([]model.HumanizeScore, int64, error) {
	return s.scoreRepo.List(ctx, sessionID, passed, page, pageSize)
}

func (s *tuningService) StatsHumanizeScores(ctx context.Context, since time.Time) (*HumanizeStats, error) {
	stat, err := s.scoreRepo.Stats(ctx, since)
	if err != nil {
		return nil, err
	}
	return &HumanizeStats{
		AvgScore: stat.AvgScore,
		Passed:   stat.Passed,
		Failed:   stat.Failed,
		Total:    stat.Total,
	}, nil
}

// ----- 5. ChampionBaseline -----

func (s *tuningService) ListChampionBaselines(ctx context.Context) ([]model.ChampionBaseline, error) {
	return s.baselineRepo.ListEnabledModels(ctx)
}

// ----- 6. FeedbackEvent -----

func (s *tuningService) ListFeedbackEvents(ctx context.Context, sessionID, signalKey string, page, pageSize int) ([]model.FeedbackEvent, int64, error) {
	return s.feedbackRepo.ListFeedbackEvents(ctx, sessionID, signalKey, page, pageSize)
}

// StatsFeedbackEvents 统计各 signal_key 的反馈计数
//
// 仓储返回 []FeedbackEventStat，含 SignalKey/Count 字段。
func (s *tuningService) StatsFeedbackEvents(ctx context.Context, since time.Time) (map[string]int64, error) {
	rows, err := s.feedbackRepo.StatsFeedbackEvents(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.SignalKey] = r.Count
	}
	return out, nil
}

// ----- 7. ChampionDialogue -----

func (s *tuningService) ListChampionDialogues(ctx context.Context, intent, industry string, page, pageSize int) ([]model.ChampionDialogue, int64, error) {
	return s.feedbackRepo.ListChampionDialogues(ctx, intent, industry, page, pageSize)
}

// ----- 8. PromptCandidate -----

func (s *tuningService) ListPromptCandidates(ctx context.Context, status string, page, pageSize int) ([]model.PromptCandidate, int64, error) {
	return s.feedbackRepo.ListPromptCandidates(ctx, status, page, pageSize)
}

func (s *tuningService) UpdatePromptCandidateStatus(ctx context.Context, id, status string) error {
	return s.feedbackRepo.UpdatePromptCandidateStatus(ctx, id, status)
}

// ----- 9. BanditArm -----

func (s *tuningService) ListBanditArms(ctx context.Context, experimentID, sopID string, page, pageSize int) ([]model.BanditArm, int64, error) {
	return s.feedbackRepo.ListBanditArms(ctx, experimentID, sopID, page, pageSize)
}

// ----- 11. LowQualitySample -----

func (s *tuningService) ListLowQualitySamples(ctx context.Context, sampleType string, page, pageSize int) ([]model.LowQualitySample, int64, error) {
	return s.lowQRepo.List(ctx, sampleType, page, pageSize)
}

// safeDiv 安全除法(避免除 0 触发 panic)
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
