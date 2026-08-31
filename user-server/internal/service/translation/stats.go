package translation

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/repository"
)

// I18nStatsRepo 统计仓储接口（由 repository.I18nStatsRepository 实现）。
//
// 在 service 层重新声明接口而非直接依赖 repository.I18nStatsRepository，
// 是为了与 GlossaryRepo 保持一致的五层架构风格（依赖倒置，便于 mock 测试）。
type I18nStatsRepo interface {
	LangDistribution(ctx context.Context, since time.Time) ([]repository.LangDistRow, error)
	CacheHitRate(ctx context.Context, since time.Time) (hit, miss int64, err error)
	GlossaryCoverage(ctx context.Context) ([]repository.GlossaryCovRow, error)
	QualityTrend(ctx context.Context, days int) ([]repository.QualityTrendRow, error)
	FallbackRate(ctx context.Context, since time.Time) (total, fallback int64, err error)
	LatencyStats(ctx context.Context, since time.Time) ([]repository.LatencyRow, error)
}

// I18nStatsOverview 看板首页总览视图
//
// 汇总最近 N 天（默认 7 天）的核心指标，前端一次性拉取渲染首屏。
type I18nStatsOverview struct {
	Days  int       `json:"days"`
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`

	TotalCalls        int64 `json:"total_calls"`
	CrossLingualCalls int64 `json:"cross_lingual_calls"`
	LangCount         int   `json:"lang_count"`

	CacheHit     int64   `json:"cache_hit"`
	CacheMiss    int64   `json:"cache_miss"`
	CacheHitRate float64 `json:"cache_hit_rate"`

	FallbackTotal int64   `json:"fallback_total"`
	FallbackCount int64   `json:"fallback_count"`
	FallbackRate  float64 `json:"fallback_rate"`

	GlossaryLangCount int `json:"glossary_lang_count"`

	LatencyP50 float64 `json:"latency_p50"`
	LatencyP95 float64 `json:"latency_p95"`
	LatencyP99 float64 `json:"latency_p99"`
}

// I18nStatsService 多语言统计服务
type I18nStatsService struct {
	repo I18nStatsRepo
}

// NewI18nStatsService 构造 I18nStatsService
func NewI18nStatsService(repo I18nStatsRepo) *I18nStatsService {
	return &I18nStatsService{repo: repo}
}

// GetStats 总览统计（看板首页）
//
// 汇总最近 days 天的核心指标。days <= 0 时默认 7 天。
// 内部并发安全：依次调用 repository 各聚合方法，单次失败仅记录到 error 返回，
// 不阻断其他维度（看板允许部分指标缺失）。
func (s *I18nStatsService) GetStats(ctx context.Context) (*I18nStatsOverview, error) {
	return s.GetStatsWithDays(ctx, 7)
}

// GetStatsWithDays 总览统计（自定义时间窗口）
func (s *I18nStatsService) GetStatsWithDays(ctx context.Context, days int) (*I18nStatsOverview, error) {
	if days <= 0 {
		days = 7
	}
	now := time.Now()
	since := now.AddDate(0, 0, -days)

	overview := &I18nStatsOverview{
		Days:  days,
		Since: since,
		Until: now,
	}

	if dist, err := s.repo.LangDistribution(ctx, since); err != nil {
		return nil, fmt.Errorf("i18n_stats: lang distribution failed: %w", err)
	} else {
		langSeen := make(map[string]struct{}, len(dist))
		for _, row := range dist {
			overview.TotalCalls += row.Count
			overview.CrossLingualCalls += row.CrossLingualCount
			if row.TargetLang != "" {
				langSeen[row.TargetLang] = struct{}{}
			}
		}
		overview.LangCount = len(langSeen)
	}

	if hit, miss, err := s.repo.CacheHitRate(ctx, since); err != nil {
		return nil, fmt.Errorf("i18n_stats: cache hit rate failed: %w", err)
	} else {
		overview.CacheHit = hit
		overview.CacheMiss = miss
		total := hit + miss
		if total > 0 {
			overview.CacheHitRate = float64(hit) / float64(total)
		}
	}

	if total, fallback, err := s.repo.FallbackRate(ctx, since); err != nil {
		return nil, fmt.Errorf("i18n_stats: fallback rate failed: %w", err)
	} else {
		overview.FallbackTotal = total
		overview.FallbackCount = fallback
		if total > 0 {
			overview.FallbackRate = float64(fallback) / float64(total)
		}
	}

	if cov, err := s.repo.GlossaryCoverage(ctx); err != nil {
		return nil, fmt.Errorf("i18n_stats: glossary coverage failed: %w", err)
	} else {
		overview.GlossaryLangCount = len(cov)
	}

	if latencies, err := s.repo.LatencyStats(ctx, since); err != nil {
		return nil, fmt.Errorf("i18n_stats: latency stats failed: %w", err)
	} else {
		var totalCount int64
		var p50Sum, p95Sum, p99Sum float64
		for _, l := range latencies {
			totalCount += l.Count
			p50Sum += l.P50 * float64(l.Count)
			p95Sum += l.P95 * float64(l.Count)
			p99Sum += l.P99 * float64(l.Count)
		}
		if totalCount > 0 {
			overview.LatencyP50 = p50Sum / float64(totalCount)
			overview.LatencyP95 = p95Sum / float64(totalCount)
			overview.LatencyP99 = p99Sum / float64(totalCount)
		}
	}

	return overview, nil
}

// GetLangDistribution 语言分布
//
// days <= 0 时默认 7 天。
func (s *I18nStatsService) GetLangDistribution(ctx context.Context, days int) ([]repository.LangDistRow, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := s.repo.LangDistribution(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("i18n_stats: lang distribution failed: %w", err)
	}
	return rows, nil
}

// GetCacheHitRate 缓存命中率
//
// 返回 0~1 之间的浮点数。days <= 0 时默认 7 天。
// total=0 时返回 0（避免除零）。
func (s *I18nStatsService) GetCacheHitRate(ctx context.Context, days int) (float64, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	hit, miss, err := s.repo.CacheHitRate(ctx, since)
	if err != nil {
		return 0, fmt.Errorf("i18n_stats: cache hit rate failed: %w", err)
	}
	total := hit + miss
	if total == 0 {
		return 0, nil
	}
	return float64(hit) / float64(total), nil
}

// GetGlossaryCoverage 术语覆盖率
func (s *I18nStatsService) GetGlossaryCoverage(ctx context.Context) ([]repository.GlossaryCovRow, error) {
	rows, err := s.repo.GlossaryCoverage(ctx)
	if err != nil {
		return nil, fmt.Errorf("i18n_stats: glossary coverage failed: %w", err)
	}
	return rows, nil
}

// GetQualityTrend 质量趋势
//
// days <= 0 时默认 30 天。
func (s *I18nStatsService) GetQualityTrend(ctx context.Context, days int) ([]repository.QualityTrendRow, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.repo.QualityTrend(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("i18n_stats: quality trend failed: %w", err)
	}
	return rows, nil
}

// GetLatencyStats 延迟统计
//
// days <= 0 时默认 7 天。
func (s *I18nStatsService) GetLatencyStats(ctx context.Context, days int) ([]repository.LatencyRow, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	rows, err := s.repo.LatencyStats(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("i18n_stats: latency stats failed: %w", err)
	}
	return rows, nil
}

// GetFallbackRate 降级率
//
// days <= 0 时默认 7 天。返回 0~1 之间的浮点数。
func (s *I18nStatsService) GetFallbackRate(ctx context.Context, days int) (float64, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)
	total, fallback, err := s.repo.FallbackRate(ctx, since)
	if err != nil {
		return 0, fmt.Errorf("i18n_stats: fallback rate failed: %w", err)
	}
	if total == 0 {
		return 0, nil
	}
	return float64(fallback) / float64(total), nil
}
