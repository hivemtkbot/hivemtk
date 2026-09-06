package service

import (
	"context"
	"sort"

	"hivemtk-user/internal/geo/repository"
)

// VisibilityService AI 可见性趋势分析服务
//
// 基于 geo_daily_stats 预聚合表输出每日可见率序列与环比变化，
// 对标 Peec AI / Otterly 的 Visibility Trend 能力。
type VisibilityService struct {
	dailyRepo repository.GeoDailyStatRepository
}

// NewVisibilityService 创建可见性趋势服务
func NewVisibilityService(dailyRepo repository.GeoDailyStatRepository) *VisibilityService {
	return &VisibilityService{dailyRepo: dailyRepo}
}

// TrendQuery 趋势查询参数
type TrendQuery struct {
	Engine string
	Intent string
	Days   int
}

// VisibilityTrendPoint 单日可见性指标
type VisibilityTrendPoint struct {
	Date          string  `json:"date"`
	ProbeCount    int     `json:"probe_count"`
	BrandHits     int     `json:"brand_hits"`
	Visibility    float64 `json:"visibility"`
	CitationCount int     `json:"citation_count"`
	NegativeCount int     `json:"negative_count"`
}

// VisibilityTrendResult 趋势序列 + 环比
type VisibilityTrendResult struct {
	Points         []VisibilityTrendPoint `json:"points"`
	CurrentAvg     float64                `json:"current_avg"`
	PreviousAvg    float64                `json:"previous_avg"`
	Change         float64                `json:"change"`
	ChangePct      float64                `json:"change_pct"`
	TotalProbes    int                    `json:"total_probes"`
	TotalBrandHits int                    `json:"total_brand_hits"`
}

// GetTrend 可见性趋势 + 环比（周环比：各取 days/2 对半对比；不足 2 天无环比）
func (s *VisibilityService) GetTrend(ctx context.Context, q TrendQuery) (*VisibilityTrendResult, error) {
	if q.Days <= 0 || q.Days > 365 {
		q.Days = 30
	}
	stats, err := s.dailyRepo.GetTrend(ctx, q.Engine, "", q.Intent, q.Days)
	if err != nil {
		return nil, err
	}

	byDate := map[string]*VisibilityTrendPoint{}
	for _, st := range stats {
		p, ok := byDate[st.Date]
		if !ok {
			p = &VisibilityTrendPoint{Date: st.Date}
			byDate[st.Date] = p
		}
		p.ProbeCount += st.ProbeCount
		p.BrandHits += st.BrandMentionedCount
		p.CitationCount += st.CitationCount
		p.NegativeCount += st.NegativeCount
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	points := make([]VisibilityTrendPoint, 0, len(dates))
	for _, d := range dates {
		p := byDate[d]
		if p.ProbeCount == 0 {
			if p.BrandHits > 0 {
				p.ProbeCount = p.BrandHits
				p.Visibility = 1
			}
		} else {
			p.Visibility = float64(p.BrandHits) / float64(p.ProbeCount)
		}
		points = append(points, *p)
	}

	totalProbes, totalBrandHits := 0, 0
	for _, p := range points {
		totalProbes += p.ProbeCount
		totalBrandHits += p.BrandHits
	}

	res := &VisibilityTrendResult{Points: points, TotalProbes: totalProbes, TotalBrandHits: totalBrandHits}
	half := len(points) / 2
	if half > 0 {
		curSum, curN, prevSum, prevN := 0.0, 0.0, 0.0, 0.0
		for i, p := range points {
			if i < half {
				prevSum += p.Visibility
				prevN++
			} else {
				curSum += p.Visibility
				curN++
			}
		}
		if prevN > 0 {
			res.PreviousAvg = prevSum / prevN
		}
		if curN > 0 {
			res.CurrentAvg = curSum / curN
		}
		res.Change = res.CurrentAvg - res.PreviousAvg
		if res.PreviousAvg > 0 {
			res.ChangePct = res.Change / res.PreviousAvg
		}
	}
	return res, nil
}
