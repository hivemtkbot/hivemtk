package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	_db "hivemtk-user/internal/pkg/db"
)

// I18nStatsRepository 多语言统计仓库接口
type I18nStatsRepository interface {
	LangDistribution(ctx context.Context, since time.Time) ([]LangDistRow, error)

	CacheHitRate(ctx context.Context, since time.Time) (hit, miss int64, err error)

	GlossaryCoverage(ctx context.Context) ([]GlossaryCovRow, error)

	QualityTrend(ctx context.Context, days int) ([]QualityTrendRow, error)

	FallbackRate(ctx context.Context, since time.Time) (total, fallback int64, err error)

	LatencyStats(ctx context.Context, since time.Time) ([]LatencyRow, error)
}

// LangDistRow 语言分布聚合行
type LangDistRow struct {
	InternalLang      string `gorm:"column:internal_lang" json:"internal_lang"`
	TargetLang        string `gorm:"column:target_lang" json:"target_lang"`
	Count             int64  `gorm:"column:count" json:"count"`
	CrossLingualCount int64  `gorm:"column:cross_lingual_count" json:"cross_lingual_count"`
}

// GlossaryCovRow 术语覆盖率聚合行
type GlossaryCovRow struct {
	TargetLang  string `gorm:"column:target_lang" json:"target_lang"`
	TermCount   int64  `gorm:"column:term_count" json:"term_count"`
	ActiveCount int64  `gorm:"column:active_count" json:"active_count"`
}

// QualityTrendRow 质量趋势聚合行（按天）
type QualityTrendRow struct {
	Date       string  `gorm:"column:date" json:"date"`
	AvgScore   float64 `gorm:"column:avg_score" json:"avg_score"`
	TotalCount int64   `gorm:"column:total_count" json:"total_count"`
}

// LatencyRow 延迟统计聚合行（按 target_lang）
type LatencyRow struct {
	TargetLang string  `gorm:"column:target_lang" json:"target_lang"`
	P50        float64 `gorm:"column:p50" json:"p50"`
	P95        float64 `gorm:"column:p95" json:"p95"`
	P99        float64 `gorm:"column:p99" json:"p99"`
	Count      int64   `gorm:"column:count" json:"count"`
}

type i18nStatsRepo struct {
	db *gorm.DB
}

// NewI18nStatsRepository 创建多语言统计仓库（绑定全局默认 DB）
func NewI18nStatsRepository() I18nStatsRepository {
	return &i18nStatsRepo{db: _db.GetDB()}
}

// NewI18nStatsRepositoryWithDB 创建指定 DB 的统计仓库实例（用于依赖注入 / 测试）
// db 为 nil 时回退全局 DB。
func NewI18nStatsRepositoryWithDB(db *gorm.DB) I18nStatsRepository {
	if db == nil {
		return &i18nStatsRepo{db: _db.GetDB()}
	}
	return &i18nStatsRepo{db: db}
}

func (r *i18nStatsRepo) SetDB(_ context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *i18nStatsRepo) GetDB(_ context.Context) *gorm.DB {
	return r.db
}

func (r *i18nStatsRepo) LangDistribution(ctx context.Context, since time.Time) ([]LangDistRow, error) {
	if r.db == nil {
		return nil, errors.New("i18n_stats: db is nil")
	}
	raw := `
		SELECT
			COALESCE(internal_lang, '') AS internal_lang,
			COALESCE(target_lang, '')   AS target_lang,
			COUNT(*)                    AS count,
			COUNT(*) FILTER (WHERE cross_lingual = TRUE) AS cross_lingual_count
		FROM llm_routing_logs
		WHERE created_at >= ? AND target_lang IS NOT NULL AND target_lang <> ''
		GROUP BY COALESCE(internal_lang, ''), COALESCE(target_lang, '')
		ORDER BY count DESC`
	var rows []LangDistRow
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *i18nStatsRepo) CacheHitRate(ctx context.Context, since time.Time) (hit, miss int64, err error) {
	if r.db == nil {
		return 0, 0, errors.New("i18n_stats: db is nil")
	}
	raw := `
		SELECT
			COUNT(*) FILTER (WHERE cache_hit = TRUE)  AS hit,
			COUNT(*) FILTER (WHERE cache_hit = FALSE) AS miss
		FROM llm_routing_logs
		WHERE created_at >= ? AND target_lang IS NOT NULL AND target_lang <> ''`
	var row struct {
		Hit  int64 `gorm:"column:hit"`
		Miss int64 `gorm:"column:miss"`
	}
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Hit, row.Miss, nil
}

func (r *i18nStatsRepo) GlossaryCoverage(ctx context.Context) ([]GlossaryCovRow, error) {
	if r.db == nil {
		return nil, errors.New("i18n_stats: db is nil")
	}
	raw := `
		SELECT
			lang AS target_lang,
			COUNT(*)                                           AS term_count,
			COUNT(*) FILTER (WHERE status = 'active')          AS active_count
		FROM glossaries g,
		LATERAL jsonb_object_keys(g.translations) AS lang
		WHERE g.translations IS NOT NULL
		GROUP BY lang
		ORDER BY term_count DESC`
	var rows []GlossaryCovRow
	if err := r.db.WithContext(ctx).Raw(raw).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *i18nStatsRepo) QualityTrend(ctx context.Context, days int) ([]QualityTrendRow, error) {
	if r.db == nil {
		return nil, errors.New("i18n_stats: db is nil")
	}
	if days <= 0 {
		days = 30
	}
	raw := `
		SELECT
			TO_CHAR(created_at, 'YYYY-MM-DD') AS date,
			COALESCE(AVG(quality_score), 0)   AS avg_score,
			COUNT(*)                          AS total_count
		FROM llm_routing_logs
		WHERE created_at >= NOW() - make_interval(days => ?)
		  AND quality_score IS NOT NULL
		GROUP BY TO_CHAR(created_at, 'YYYY-MM-DD')
		ORDER BY date ASC`
	var rows []QualityTrendRow
	if err := r.db.WithContext(ctx).Raw(raw, days).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *i18nStatsRepo) FallbackRate(ctx context.Context, since time.Time) (total, fallback int64, err error) {
	if r.db == nil {
		return 0, 0, errors.New("i18n_stats: db is nil")
	}
	raw := `
		SELECT
			COUNT(*)                                AS total,
			COUNT(*) FILTER (WHERE is_fallback = TRUE) AS fallback
		FROM llm_routing_logs
		WHERE created_at >= ? AND target_lang IS NOT NULL AND target_lang <> ''`
	var row struct {
		Total    int64 `gorm:"column:total"`
		Fallback int64 `gorm:"column:fallback"`
	}
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Total, row.Fallback, nil
}

func (r *i18nStatsRepo) LatencyStats(ctx context.Context, since time.Time) ([]LatencyRow, error) {
	if r.db == nil {
		return nil, errors.New("i18n_stats: db is nil")
	}
	raw := `
		SELECT
			target_lang,
			COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms), 0) AS p50,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS p95,
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) AS p99,
			COUNT(*) AS count
		FROM llm_routing_logs
		WHERE created_at >= ? AND target_lang IS NOT NULL AND target_lang <> ''
		GROUP BY target_lang
		ORDER BY count DESC`
	var rows []LatencyRow
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

var _ I18nStatsRepository = (*i18nStatsRepo)(nil)
