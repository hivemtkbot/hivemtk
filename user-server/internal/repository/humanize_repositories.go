package repository

// humanize_repositories.go P0-4 拟人度评估器仓储实现
//
// 五层架构归属: L4 数据访问层
// 设计依据: docs/核心链路优化.md 第十六章 §16.3 表结构设计
//
// 4 个 Repository：
//  1. HumanizeScoreRepository         - 评分主表 + 维度明细
//  2. HumanizeDimensionRepository     - 维度得分明细独立查询
//  3. ChampionBaselineRepository      - 销冠基线 + 短语库
//  4. ABTestStatRepository            - A/B 测试统计结果
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service/humanize"
)

// ============================================================================
// 1. HumanizeScoreRepository 评分主表 + 维度明细
// ============================================================================

// HumanizeScoreRepository 评分仓储
type HumanizeScoreRepository struct{}

// NewHumanizeScoreRepository 构造
func NewHumanizeScoreRepository() *HumanizeScoreRepository {
	return &HumanizeScoreRepository{}
}

// Save 保存评分结果（含维度明细）
func (r *HumanizeScoreRepository) Save(ctx context.Context, result *dto.HumanizeEvalResult, input *dto.HumanizeEvalInput) error {
	if result == nil {
		return fmt.Errorf("result is nil")
	}
	gormDB := db.GetDB().WithContext(ctx)
	scoreID := generateScoreID()
	if input != nil {
		// 写入主表
	}
	main := &model.HumanizeScore{
		ScoreID:            scoreID,
		SessionID:          safeStr(input.SessionID),
		CustomerID:         safeStr(input.CustomerID),
		MessageID:          safeStr(input.MessageID),
		Persona:            safeStr(input.Persona),
		Industry:           safeStr(input.Industry),
		Platform:           safeStr(input.Platform),
		Intent:             safeStr(input.Intent),
		CustomerMessage:    safeStr(input.CustomerMessage),
		AIReply:            safeStr(input.AIReply),
		FinalReply:         result.FinalReply,
		EvaluatorType:      model.HumanizeEvaluatorType(result.EvaluatorType),
		SampleStrategy:     model.HumanizeSampleStrategy(result.SampleStrategy),
		TotalScore:         result.TotalScore,
		Threshold:          0.85,
		DistanceToChampion: result.DistanceToChampion,
		Passed:             result.Passed,
		AttemptCount:       result.AttemptCount,
		LLMModel:           result.LLMModel,
		LLMLatencyMs:       result.LLMLatencyMs,
		ReasonJSON:         buildReasonJSON(result),
	}
	// 填充 5 维得分
	for _, s := range result.Scores {
		switch s.Dimension {
		case dto.HumanizeDimNaturalness:
			main.Naturalness = s.Score
		case dto.HumanizeDimConciseness:
			main.Conciseness = s.Score
		case dto.HumanizeDimEmpathy:
			main.Empathy = s.Score
		case dto.HumanizeDimProfessionalism:
			main.Professionalism = s.Score
		case dto.HumanizeDimPersuasiveness:
			main.Persuasiveness = s.Score
		}
	}
	if err := gormDB.Create(main).Error; err != nil {
		return fmt.Errorf("create humanize_score: %w", err)
	}
	// 写入维度明细
	if len(result.Scores) > 0 {
		records := make([]model.HumanizeDimensionRecord, 0, len(result.Scores))
		for _, s := range result.Scores {
			records = append(records, model.HumanizeDimensionRecord{
				ScoreID:   scoreID,
				Dimension: string(s.Dimension),
				Score:     s.Score,
				Weight:    dto.HumanizeDimensionWeight[s.Dimension],
				Reason:    s.Reason,
			})
		}
		if err := gormDB.CreateInBatches(records, 100).Error; err != nil {
			return fmt.Errorf("create humanize_dimensions: %w", err)
		}
	}
	return nil
}

// ListBySession 按 session 查询评分历史
func (r *HumanizeScoreRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]model.HumanizeScore, error) {
	var list []model.HumanizeScore
	q := db.GetDB().WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// ListByPersona 按 persona+industry+intent 查询
func (r *HumanizeScoreRepository) ListByPersona(ctx context.Context, persona, industry, intent string, limit int) ([]model.HumanizeScore, error) {
	var list []model.HumanizeScore
	q := db.GetDB().WithContext(ctx).
		Where("persona = ? AND industry = ? AND intent = ?", persona, industry, intent).
		Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// GetByID 按 ID 查询
func (r *HumanizeScoreRepository) GetByID(ctx context.Context, id uint64) (*model.HumanizeScore, error) {
	var s model.HumanizeScore
	err := db.GetDB().WithContext(ctx).First(&s, id).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListDimensionsByScoreID 查询维度明细
func (r *HumanizeScoreRepository) ListDimensionsByScoreID(ctx context.Context, scoreID string) ([]model.HumanizeDimensionRecord, error) {
	var list []model.HumanizeDimensionRecord
	err := db.GetDB().WithContext(ctx).
		Where("score_id = ?", scoreID).
		Order("created_at ASC").
		Find(&list).Error
	return list, err
}

// ============================================================================
// 2. ChampionBaselineRepository 销冠基线 + 短语库
// ============================================================================

// ChampionBaselineRepositoryImpl 销冠基线仓储
//
// 实现 humanize.ChampionBaselineRepository 接口
type ChampionBaselineRepositoryImpl struct {
	phraseExtractor *humanize.TFIDFPhraseExtractor
}

// NewChampionBaselineRepository 构造
func NewChampionBaselineRepository() *ChampionBaselineRepositoryImpl {
	return &ChampionBaselineRepositoryImpl{
		phraseExtractor: humanize.NewTFIDFPhraseExtractor(),
	}
}

// FindByPersonaIndustryIntent 查找启用的基线（取最新版本）
func (r *ChampionBaselineRepositoryImpl) FindByPersonaIndustryIntent(
	ctx context.Context, persona, industry, intent string,
) (*dto.ChampionBaselineDTO, error) {
	var b model.ChampionBaseline
	err := db.GetDB().WithContext(ctx).
		Where("persona = ? AND industry = ? AND intent = ? AND enabled = ?", persona, industry, intent, true).
		Order("version DESC").
		First(&b).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &dto.ChampionBaselineDTO{
		Persona:         b.Persona,
		Industry:        b.Industry,
		Intent:          b.Intent,
		Naturalness:     b.Naturalness,
		Conciseness:     b.Conciseness,
		Empathy:         b.Empathy,
		Professionalism: b.Professionalism,
		Persuasiveness:  b.Persuasiveness,
	}, nil
}

// Save 保存新版本基线
func (r *ChampionBaselineRepositoryImpl) Save(
	ctx context.Context, b *dto.ChampionBaselineDTO,
	sampleCount int, stddev float64, periodStart, periodEnd any,
) (uint64, error) {
	if b == nil {
		return 0, fmt.Errorf("baseline is nil")
	}
	// 查询当前最大版本号
	var maxVersion int
	db.GetDB().WithContext(ctx).
		Model(&model.ChampionBaseline{}).
		Where("persona = ? AND industry = ? AND intent = ?", b.Persona, b.Industry, b.Intent).
		Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)

	var pStart, pEnd time.Time
	if t, ok := periodStart.(time.Time); ok {
		pStart = t
	}
	if t, ok := periodEnd.(time.Time); ok {
		pEnd = t
	}
	entity := &model.ChampionBaseline{
		Persona:         b.Persona,
		Industry:        b.Industry,
		Intent:          b.Intent,
		Naturalness:     b.Naturalness,
		Conciseness:     b.Conciseness,
		Empathy:         b.Empathy,
		Professionalism: b.Professionalism,
		Persuasiveness:  b.Persuasiveness,
		SampleCount:     sampleCount,
		SampleStddev:    stddev,
		PeriodStart:     pStart,
		PeriodEnd:       pEnd,
		Version:         maxVersion + 1,
		Enabled:         true,
	}
	if err := db.GetDB().WithContext(ctx).Create(entity).Error; err != nil {
		return 0, fmt.Errorf("create champion_baseline: %w", err)
	}
	// 旧版本禁用（保留历史）
	db.GetDB().WithContext(ctx).
		Model(&model.ChampionBaseline{}).
		Where("persona = ? AND industry = ? AND intent = ? AND id != ?",
			b.Persona, b.Industry, b.Intent, entity.ID).
		Update("enabled", false)
	return entity.ID, nil
}

// ListEnabled 列出所有启用的基线
func (r *ChampionBaselineRepositoryImpl) ListEnabled(ctx context.Context) ([]dto.ChampionBaselineDTO, error) {
	var entities []model.ChampionBaseline
	err := db.GetDB().WithContext(ctx).
		Where("enabled = ?", true).
		Order("persona, industry, intent, version DESC").
		Find(&entities).Error
	if err != nil {
		return nil, err
	}
	out := make([]dto.ChampionBaselineDTO, 0, len(entities))
	for _, e := range entities {
		out = append(out, dto.ChampionBaselineDTO{
			Persona:         e.Persona,
			Industry:        e.Industry,
			Intent:          e.Intent,
			Naturalness:     e.Naturalness,
			Conciseness:     e.Conciseness,
			Empathy:         e.Empathy,
			Professionalism: e.Professionalism,
			Persuasiveness:  e.Persuasiveness,
		})
	}
	return out, nil
}

// RefreshPhrases 刷新短语库
func (r *ChampionBaselineRepositoryImpl) RefreshPhrases(ctx context.Context, baselineID uint64, messages []humanize.ChampionMessage) error {
	if baselineID == 0 || len(messages) == 0 {
		return nil
	}
	phrases := r.phraseExtractor.Extract(messages, 30)
	gormDB := db.GetDB().WithContext(ctx)
	// 删除旧短语
	if err := gormDB.Where("baseline_id = ?", baselineID).Delete(&model.ChampionPhrase{}).Error; err != nil {
		return fmt.Errorf("delete old phrases: %w", err)
	}
	if len(phrases) == 0 {
		return nil
	}
	records := make([]model.ChampionPhrase, 0, len(phrases))
	for _, p := range phrases {
		records = append(records, model.ChampionPhrase{
			BaselineID: baselineID,
			Phrase:     p.Phrase,
			TFIDFScore: p.TFIDFScore,
			TF:         p.TF,
			DF:         p.DF,
			PhraseType: string(p.PhraseType),
			Rank:       p.Rank,
		})
	}
	if err := gormDB.CreateInBatches(records, 100).Error; err != nil {
		return fmt.Errorf("create champion_phrases: %w", err)
	}
	return nil
}

// ListPhrases 列出基线的 top-N 短语
func (r *ChampionBaselineRepositoryImpl) ListPhrases(ctx context.Context, baselineID uint64, limit int) ([]model.ChampionPhrase, error) {
	var list []model.ChampionPhrase
	q := db.GetDB().WithContext(ctx).
		Where("baseline_id = ?", baselineID).
		Order("rank ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// ============================================================================
// 3. ABTestStatRepository A/B 测试统计结果
// ============================================================================

// ABTestStatRepository A/B 测试统计仓储
type ABTestStatRepository struct{}

// NewABTestStatRepository 构造
func NewABTestStatRepository() *ABTestStatRepository {
	return &ABTestStatRepository{}
}

// Save 保存统计结果（同时写入 control 与 treatment 两条记录）
func (r *ABTestStatRepository) Save(ctx context.Context, result *dto.ABTestStatsResult, controlSize, treatmentSize int, controlStddev, treatmentStddev, controlMedian, treatmentMedian float64) error {
	if result == nil {
		return fmt.Errorf("result is nil")
	}
	rows := []model.ABTestStat{
		{
			ExperimentID:    result.ExperimentID,
			GroupName:       "control",
			SampleSize:      controlSize,
			MeanScore:       result.ControlMean,
			MedianScore:     controlMedian,
			StddevScore:     controlStddev,
			MannWhitneyU:    int64(result.MannWhitneyU),
			MannWhitneyP:    result.MannWhitneyP,
			CohensD:         result.CohensD,
			BootstrapCILow:  result.BootstrapCILow,
			BootstrapCIHigh: result.BootstrapCIHigh,
			Significant:     result.Significant,
			EffectSizeLabel: result.EffectSizeLabel,
			Winner:          result.Winner,
		},
		{
			ExperimentID:    result.ExperimentID,
			GroupName:       "treatment",
			SampleSize:      treatmentSize,
			MeanScore:       result.TreatmentMean,
			MedianScore:     treatmentMedian,
			StddevScore:     treatmentStddev,
			MannWhitneyU:    int64(result.MannWhitneyU),
			MannWhitneyP:    result.MannWhitneyP,
			CohensD:         result.CohensD,
			BootstrapCILow:  result.BootstrapCILow,
			BootstrapCIHigh: result.BootstrapCIHigh,
			Significant:     result.Significant,
			EffectSizeLabel: result.EffectSizeLabel,
			Winner:          result.Winner,
		},
	}
	return db.GetDB().WithContext(ctx).CreateInBatches(rows, 100).Error
}

// ListByExperiment 按实验 ID 查询统计结果
func (r *ABTestStatRepository) ListByExperiment(ctx context.Context, experimentID string) ([]model.ABTestStat, error) {
	var list []model.ABTestStat
	err := db.GetDB().WithContext(ctx).
		Where("experiment_id = ?", experimentID).
		Order("group_name ASC, created_at DESC").
		Find(&list).Error
	return list, err
}

// ============================================================================
// 辅助函数
// ============================================================================

// safeStr 安全字符串转换（避免 nil pointer）
func safeStr(s string) string {
	return s
}

// generateScoreID 生成评分 ID
//
// 格式：hs_<unix_nano>
func generateScoreID() string {
	return fmt.Sprintf("hs_%d", time.Now().UnixNano())
}

// buildReasonJSON 构建原因 JSON
func buildReasonJSON(result *dto.HumanizeEvalResult) string {
	if result == nil || len(result.Scores) == 0 {
		return "{}"
	}
	m := make(map[string]string, len(result.Scores))
	for _, s := range result.Scores {
		m[string(s.Dimension)] = s.Reason
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// 编译时接口断言
var (
	_ humanize.ChampionBaselineRepository = (*ChampionBaselineRepositoryImpl)(nil)
	_ humanize.HumanizeScoreRepository    = (*HumanizeScoreRepository)(nil)
	_ humanize.LowQualitySampleCollector  = (*HumanizeLowQualitySampleCollector)(nil)
)

// ============================================================================
// 管理面扩展方法（TuningController 使用，按业务域下沉，不新建上帝 service）
// ============================================================================

// HumanizeScoreStat 拟人度统计聚合
type HumanizeScoreStat struct {
	AvgScore float64 `json:"avg_score"`
	Passed   int64   `json:"passed_count"`
	Failed   int64   `json:"failed_count"`
	Total    int64   `json:"total_count"`
}

// List 分页查询拟人度评分（管理面）
// 注意：模型无 calculated_at 列，原 controller 使用 calculated_at 为潜在 bug，此处修正为 created_at。
func (r *HumanizeScoreRepository) List(ctx context.Context, sessionID string, passed *bool, page, pageSize int) ([]model.HumanizeScore, int64, error) {
	q := db.GetDB().WithContext(ctx).Model(&model.HumanizeScore{})
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	if passed != nil {
		q = q.Where("passed = ?", *passed)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.HumanizeScore
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Stats 拟人度统计（按时间窗，管理面）
func (r *HumanizeScoreRepository) Stats(ctx context.Context, since time.Time) (*HumanizeScoreStat, error) {
	var row HumanizeScoreStat
	if err := db.GetDB().WithContext(ctx).
		Model(&model.HumanizeScore{}).
		Select("AVG(total_score) as avg_score, "+
			"SUM(CASE WHEN passed = true THEN 1 ELSE 0 END) as passed, "+
			"SUM(CASE WHEN passed = false THEN 1 ELSE 0 END) as failed, "+
			"COUNT(*) as total").
		Where("created_at > ?", since).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// BaselineMetricAggregate 基线聚合结果（5 维均值 + 标准差 + 计数）
type BaselineMetricAggregate struct {
	AvgN   float64 // AVG(naturalness)
	AvgC   float64 // AVG(conciseness)
	AvgE   float64 // AVG(empathy)
	AvgP   float64 // AVG(professionalism)
	AvgR   float64 // AVG(persuasiveness)
	Stddev float64 // STDDEV(total_score)
	Count  int64   // COUNT(*)
}

// AggregateBaselineMetrics 聚合指定时间窗口内 humanize_scores 的 5 维均值 + 标准差 + 计数
// 用于 FeedbackLoopCron 月度刷新 ChampionBaseline
// 注意：原 feedback_loop_cron.go Raw SQL 中误用 final_score，实际表字段为 total_score，此处修正
func (r *HumanizeScoreRepository) AggregateBaselineMetrics(
	ctx context.Context, start, end time.Time,
) (*BaselineMetricAggregate, error) {
	var row BaselineMetricAggregate
	err := db.GetDB().WithContext(ctx).
		Model(&model.HumanizeScore{}).
		Select(`
			COALESCE(AVG(naturalness), 0)     AS "AvgN",
			COALESCE(AVG(conciseness), 0)     AS "AvgC",
			COALESCE(AVG(empathy), 0)         AS "AvgE",
			COALESCE(AVG(professionalism), 0) AS "AvgP",
			COALESCE(AVG(persuasiveness), 0)  AS "AvgR",
			COALESCE(STDDEV(total_score), 0)  AS "Stddev",
			COUNT(*)                           AS "Count"
		`).
		Where("created_at >= ? AND created_at <= ? AND total_score IS NOT NULL", start, end).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListEnabledModels 返回启用中的销冠基线（model 实体，供管理面展示）
// 注意：模型启用列是 enabled，原 controller 使用 is_enabled 为潜在 bug，此处修正。
func (r *ChampionBaselineRepositoryImpl) ListEnabledModels(ctx context.Context) ([]model.ChampionBaseline, error) {
	var list []model.ChampionBaseline
	if err := db.GetDB().WithContext(ctx).
		Where("enabled = ?", true).
		Order("created_at DESC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// MaxVersion 查询指定三元组 (persona, industry, intent) 的最大版本号
// 用于 ChampionBaseline 版本递增（FeedbackLoopCron 月度刷新）
func (r *ChampionBaselineRepositoryImpl) MaxVersion(
	ctx context.Context, persona, industry, intent string,
) (int, error) {
	var maxVersion int
	err := db.GetDB().WithContext(ctx).
		Model(&model.ChampionBaseline{}).
		Where("persona = ? AND industry = ? AND intent = ?", persona, industry, intent).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error
	if err != nil {
		return 0, err
	}
	return maxVersion, nil
}

// DisableOldVersions 禁用指定三元组下小于 keepVersion 的所有旧版本
// 用于 ChampionBaseline 新版本启用后自动禁用旧版本（仅保留最新启用）
func (r *ChampionBaselineRepositoryImpl) DisableOldVersions(
	ctx context.Context, persona, industry, intent string, keepVersion int,
) error {
	return db.GetDB().WithContext(ctx).
		Model(&model.ChampionBaseline{}).
		Where("persona = ? AND industry = ? AND intent = ? AND version < ?",
			persona, industry, intent, keepVersion).
		Update("enabled", false).Error
}

// CreateBaseline 创建新的 ChampionBaseline 记录
// 用于 FeedbackLoopCron 月度刷新写入新版本
func (r *ChampionBaselineRepositoryImpl) CreateBaseline(
	ctx context.Context, baseline *model.ChampionBaseline,
) error {
	return db.GetDB().WithContext(ctx).Create(baseline).Error
}
