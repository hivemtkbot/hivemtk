package service

import (
	"context"
	"time"

	opsrepo "hivemtk-user/internal/ops/repository"
)

// ConversionFunnelService 转化漏斗服务
type ConversionFunnelService struct {
	repo *opsrepo.ConversionFunnelRepository
}

// NewConversionFunnelService 创建转化漏斗服务
func NewConversionFunnelService() *ConversionFunnelService {
	return &ConversionFunnelService{repo: opsrepo.NewConversionFunnelRepository()}
}

// FunnelStage 漏斗阶段
type FunnelStage struct {
	Stage    string  `json:"stage"`
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	Rate     float64 `json:"rate"`      
	DropRate float64 `json:"drop_rate"` 
}

// FunnelReport 漏斗报告
type FunnelReport struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Stages      []FunnelStage `json:"stages"`
	Total       int64         `json:"total"`
	Conversion  float64       `json:"conversion"` 
	GeneratedAt time.Time     `json:"generated_at"`
}

// BuildFunnel 构建漏斗（4 阶段：访问→线索→意向→会话）
func (s *ConversionFunnelService) BuildFunnel(startTime, endTime time.Time) (*FunnelReport, error) {
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}

	ctx := context.Background()
	report := &FunnelReport{
		StartTime:   startTime,
		EndTime:     endTime,
		Stages:      make([]FunnelStage, 0, 4),
		GeneratedAt: time.Now(),
	}

	visitCount, _ := s.repo.CountCustomerEventsByTimeRange(ctx, startTime, endTime)
	report.Stages = append(report.Stages, FunnelStage{Stage: "visit", Name: "访问", Count: visitCount})

	clueCount, _ := s.repo.CountCluesByUnixTimeRange(ctx, startTime, endTime)
	report.Stages = append(report.Stages, FunnelStage{Stage: "clue", Name: "线索", Count: clueCount})

	intentCount, _ := s.repo.CountIntentRecords(ctx, startTime, endTime, []string{"buy", "purchase", "order", "interested"})
	report.Stages = append(report.Stages, FunnelStage{Stage: "intent", Name: "意向", Count: intentCount})

	sessionCount, _ := s.repo.CountCustomerSessionsByTimeRange(ctx, startTime, endTime)
	report.Stages = append(report.Stages, FunnelStage{Stage: "session", Name: "会话", Count: sessionCount})

	report.Total = visitCount
	if visitCount > 0 {
		report.Conversion = float64(sessionCount) / float64(visitCount) * 100
	}
	for i := range report.Stages {
		if i == 0 {
			report.Stages[i].Rate = 100
			continue
		}
		prev := report.Stages[i-1].Count
		cur := report.Stages[i].Count
		if prev > 0 {
			report.Stages[i].Rate = float64(cur) / float64(prev) * 100
			report.Stages[i].DropRate = 100 - report.Stages[i].Rate
		}
	}

	return report, nil
}

// StageConversion 单阶段转化详情
type StageConversion struct {
	Stage       string       `json:"stage"`
	Name        string       `json:"name"`
	Count       int64        `json:"count"`
	Rate        float64      `json:"rate"`
	AvgDuration float64      `json:"avg_duration_seconds"`
	TopSources  []SourceStat `json:"top_sources"`
}

// SourceStat 来源统计
type SourceStat struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

// GetStageDetails 阶段详情
func (s *ConversionFunnelService) GetStageDetails(stage string, startTime, endTime time.Time) (*StageConversion, error) {
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}

	ctx := context.Background()
	det := &StageConversion{Stage: stage}
	switch stage {
	case "visit":
		det.Name = "访问"
		count, _ := s.repo.CountCustomerEventsByTimeRange(ctx, startTime, endTime)
		det.Count = count
	case "clue":
		det.Name = "线索"
		count, _ := s.repo.CountCluesByUnixTimeRange(ctx, startTime, endTime)
		det.Count = count
		rows, _ := s.repo.GetClueSourceStats(ctx, startTime, endTime)
		for _, r := range rows {
			det.TopSources = append(det.TopSources, SourceStat{Source: r.Source, Count: r.Count})
		}
	case "intent":
		det.Name = "意向"
		count, _ := s.repo.CountIntentRecords(ctx, startTime, endTime, nil)
		det.Count = count
	case "session":
		det.Name = "会话"
		count, _ := s.repo.CountCustomerSessionsByTimeRange(ctx, startTime, endTime)
		det.Count = count

	}

	return det, nil
}

