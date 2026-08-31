package service

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type ResolutionStats struct {
	TotalSuggestions   int64                  `json:"total_suggestions"`
	AdoptedSuggestions int64                  `json:"adopted_suggestions"`
	AdoptionRate       float64                `json:"adoption_rate"`
	ResolvedByAI       int64                  `json:"resolved_by_ai"`
	ResolvedRate       float64                `json:"resolved_rate"`
	DailyTrend         []DailyResolutionPoint `json:"daily_trend"`
}

type DailyResolutionPoint struct {
	Date    string `json:"date"`
	Total   int64  `json:"total"`
	Adopted int64  `json:"adopted"`
}

type AIResolutionStatsService struct {
	db *gorm.DB
}

func NewAIResolutionStatsService(db *gorm.DB) *AIResolutionStatsService {
	return &AIResolutionStatsService{db: db}
}

func (s *AIResolutionStatsService) GetStats(ctx context.Context, days int) (*ResolutionStats, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	stats := &ResolutionStats{}
	if s.db == nil {
		return stats, nil
	}
	if err := s.db.WithContext(ctx).Table("ai_suggestions").
		Where("created_at >= ?", since).
		Count(&stats.TotalSuggestions).Error; err != nil {
		return stats, err
	}
	if err := s.db.WithContext(ctx).Table("ai_suggestions").
		Where("created_at >= ? AND adopted = ?", since, true).
		Count(&stats.AdoptedSuggestions).Error; err != nil {
		return stats, err
	}
	if stats.TotalSuggestions > 0 {
		stats.AdoptionRate = float64(stats.AdoptedSuggestions) / float64(stats.TotalSuggestions) * 100
	}
	stats.ResolvedByAI = stats.AdoptedSuggestions
	stats.ResolvedRate = stats.AdoptionRate
	stats.DailyTrend = []DailyResolutionPoint{}
	return stats, nil
}
