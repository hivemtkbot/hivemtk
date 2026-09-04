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
	Engine string // 空=全部引擎
	Intent string // 空=全部意图
	Days   int    // 回看天数，默认 30
}

// VisibilityTrendPoint 单日可见性指标
type VisibilityTrendPoint struct {
	Date          string  `json:"date"`            // YYYY-MM-DD
	ProbeCount    int     `json:"probe_count"`     // 探针总次数（分母）
	BrandHits     int     `json:"brand_hits"`      // 品牌提及次数
	Visibility    float64 `json:"visibility"`      // 可见率 = brand_hits / probe_count（0-1，probe_count=0 时为 0）
	CitationCount int     `json:"citation_count"`  // 被引次数
	NegativeCount int     `json:"negative_count"`  // 负面命中次数
}

// VisibilityTrendResult 趋势序列 + 环比
type VisibilityTrendResult struct {
	Points       []VisibilityTrendPoint `json:"points"`
	CurrentAvg   float64                `json:"current_avg"`    // 本周期日均可见率（0-1）
	PreviousAvg  float64                `json:"previous_avg"`   // 上一周期日均可见率
	Change       float64                `json:"change"`         // 环比变化（百分点差值，如 +0.052 表示 +5.2pt）
	ChangePct    float64                `json:"change_pct"`     // 环比变化率（相对值，previous_avg=0 时为 0）
	TotalProbes  int                    `json:"total_probes"`   // 周期探针总数
	TotalBrandHits int                  `json:"total_brand_hits"`
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

	// 按日合并（engine×intent 多行 → 单日汇总）
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

	// 可见率计算：无 probe_count 历史行（旧数据）退化为 brand>0 视为 1 次探测的近似，
	// 避免趋势图在补列前的日期全部为 0
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

	// 环比：按天对半分（前半 vs 后半）
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
