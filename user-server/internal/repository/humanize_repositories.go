package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// HumanizeScoreRepository 评分仓储
type HumanizeScoreRepository struct{}

// NewHumanizeScoreRepository 构造
func NewHumanizeScoreRepository() *HumanizeScoreRepository {
	return &HumanizeScoreRepository{}
}

// Save 保存评分结果（含维度明细）
// score 和 dimensions 由 service 层构建，仓储仅负责持久化
func (r *HumanizeScoreRepository) Save(ctx context.Context, score *model.HumanizeScore, dimensions []model.HumanizeDimensionRecord) error {
	if score == nil {
		return fmt.Errorf("score is nil")
	}
	gormDB := db.GetDB().WithContext(ctx)
	if score.ScoreID == "" {
		score.ScoreID = generateScoreID()
	}
	if err := gormDB.Create(score).Error; err != nil {
		return fmt.Errorf("create humanize_score: %w", err)
	}
	if len(dimensions) > 0 {
		for i := range dimensions {
			dimensions[i].ScoreID = score.ScoreID
		}
		if err := gormDB.CreateInBatches(dimensions, 100).Error; err != nil {
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

// ChampionBaselineRepositoryImpl 销冠基线仓储
type ChampionBaselineRepositoryImpl struct{}

// NewChampionBaselineRepository 构造
func NewChampionBaselineRepository() *ChampionBaselineRepositoryImpl {
	return &ChampionBaselineRepositoryImpl{}
}

// FindByPersonaIndustryIntent 查找启用的基线（取最新版本）
func (r *ChampionBaselineRepositoryImpl) FindByPersonaIndustryIntent(
	ctx context.Context, persona, industry, intent string,
) (*model.ChampionBaseline, error) {
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
	return &b, nil
}

// Save 保存新版本基线
// b 由 service 层构建（含 SampleCount/Stddev/PeriodStart/PeriodEnd），仓储负责版本号递增与旧版本禁用
func (r *ChampionBaselineRepositoryImpl) Save(
	ctx context.Context, b *model.ChampionBaseline,
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
	b.Version = maxVersion + 1
	b.Enabled = true
	if err := db.GetDB().WithContext(ctx).Create(b).Error; err != nil {
		return 0, fmt.Errorf("create champion_baseline: %w", err)
	}
	db.GetDB().WithContext(ctx).
		Model(&model.ChampionBaseline{}).
		Where("persona = ? AND industry = ? AND intent = ? AND id != ?",
			b.Persona, b.Industry, b.Intent, b.ID).
		Update("enabled", false)
	return b.ID, nil
}

// ListEnabled 列出所有启用的基线
func (r *ChampionBaselineRepositoryImpl) ListEnabled(ctx context.Context) ([]model.ChampionBaseline, error) {
	var entities []model.ChampionBaseline
	err := db.GetDB().WithContext(ctx).
		Where("enabled = ?", true).
		Order("persona, industry, intent, version DESC").
		Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// RefreshPhrases 刷新短语库
// phrases 由 service 层通过 TFIDFPhraseExtractor 预计算，仓储仅负责持久化
func (r *ChampionBaselineRepositoryImpl) RefreshPhrases(ctx context.Context, baselineID uint64, phrases []model.ChampionPhrase) error {
	if baselineID == 0 || len(phrases) == 0 {
		return nil
	}
	gormDB := db.GetDB().WithContext(ctx)
	if err := gormDB.Where("baseline_id = ?", baselineID).Delete(&model.ChampionPhrase{}).Error; err != nil {
		return fmt.Errorf("delete old phrases: %w", err)
	}
	for i := range phrases {
		phrases[i].BaselineID = baselineID
	}
	if err := gormDB.CreateInBatches(phrases, 100).Error; err != nil {
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

// ABTestStatRepository A/B 测试统计仓储
type ABTestStatRepository struct{}

// NewABTestStatRepository 构造
func NewABTestStatRepository() *ABTestStatRepository {
	return &ABTestStatRepository{}
}

// Save 保存统计结果（同时写入 control 与 treatment 两条记录）
// rows 由 service 层构建，仓储仅负责持久化
func (r *ABTestStatRepository) Save(ctx context.Context, rows []model.ABTestStat) error {
	if len(rows) == 0 {
		return nil
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

// generateScoreID 生成评分 ID
//
// 格式：hs_<unix_nano>
func generateScoreID() string {
	return fmt.Sprintf("hs_%d", time.Now().UnixNano())
}

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
	AvgN   float64
	AvgC   float64
	AvgE   float64
	AvgP   float64
	AvgR   float64
	Stddev float64
	Count  int64
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
