package service

import (
	"time"

	"gorm.io/gorm"
	sysmodel "marketing/internal/model"
	sysrepo "marketing/internal/repository"
)

// ConversionFunnelService 转化漏斗服务
type ConversionFunnelService struct {
	db *gorm.DB
}

// NewConversionFunnelService 创建转化漏斗服务
func NewConversionFunnelService() *ConversionFunnelService {
	return &ConversionFunnelService{db: sysrepo.GetDB()}
}

// FunnelStage 漏斗阶段
type FunnelStage struct {
	Stage    string  `json:"stage"`
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	Rate     float64 `json:"rate"`      // 阶段转化率（相对上一阶段）
	DropRate float64 `json:"drop_rate"` // 流失率
}

// FunnelReport 漏斗报告
type FunnelReport struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Stages      []FunnelStage `json:"stages"`
	Total       int64         `json:"total"`
	Conversion  float64       `json:"conversion"` // 端到端转化率
	GeneratedAt time.Time     `json:"generated_at"`
}

// BuildFunnel 构建漏斗（5 阶段：访问→线索→意向→会话→订单）
func (s *ConversionFunnelService) BuildFunnel(startTime, endTime time.Time) (*FunnelReport, error) {
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}

	report := &FunnelReport{
		StartTime:   startTime,
		EndTime:     endTime,
		Stages:      make([]FunnelStage, 0, 5),
		GeneratedAt: time.Now(),
	}

	// 阶段1：访问
	var visitCount int64
	s.db.Model(&sysmodel.CustomerEvent{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&visitCount)
	report.Stages = append(report.Stages, FunnelStage{Stage: "visit", Name: "访问", Count: visitCount})

	// 阶段2：线索
	var clueCount int64
	s.db.Model(&sysmodel.Clue{}).
		Where("create_time >= ? AND create_time <= ?", startTime.Unix(), endTime.Unix()).
		Count(&clueCount)
	report.Stages = append(report.Stages, FunnelStage{Stage: "clue", Name: "线索", Count: clueCount})

	// 阶段3：意向（来自 intent_records）
	var intentCount int64
	s.db.Table("intent_records").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Where("intent IN ?", []string{"buy", "purchase", "order", "interested"}).
		Count(&intentCount)
	report.Stages = append(report.Stages, FunnelStage{Stage: "intent", Name: "意向", Count: intentCount})

	// 阶段4：会话
	var sessionCount int64
	s.db.Model(&sysmodel.CustomerSession{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&sessionCount)
	report.Stages = append(report.Stages, FunnelStage{Stage: "session", Name: "会话", Count: sessionCount})

	// 阶段5：订单
	var orderCount int64
	s.db.Model(&sysmodel.Order{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Where("status IN ?", []string{"paid", "completed", "shipped", "delivered"}).
		Count(&orderCount)
	report.Stages = append(report.Stages, FunnelStage{Stage: "order", Name: "订单", Count: orderCount})

	// 计算阶段转化率
	report.Total = visitCount
	if visitCount > 0 {
		report.Conversion = float64(orderCount) / float64(visitCount) * 100
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

	det := &StageConversion{Stage: stage}
	switch stage {
	case "clue":
		det.Name = "线索"
		var count int64
		s.db.Model(&sysmodel.Clue{}).
			Where("create_time >= ? AND create_time <= ?", startTime.Unix(), endTime.Unix()).
			Count(&count)
		det.Count = count
		// 来源分布
		type row struct {
			Account string
			Count   int64
		}
		var rows []row
		s.db.Model(&sysmodel.Clue{}).
			Select("account, COUNT(*) as count").
			Where("create_time >= ? AND create_time <= ?", startTime.Unix(), endTime.Unix()).
			Group("account").
			Order("count DESC").Limit(10).
			Scan(&rows)
		for _, r := range rows {
			det.TopSources = append(det.TopSources, SourceStat{Source: r.Account, Count: r.Count})
		}
	case "intent":
		det.Name = "意向"
		var count int64
		s.db.Table("intent_records").
			Where("created_at BETWEEN ? AND ?", startTime, endTime).
			Count(&count)
		det.Count = count
	case "session":
		det.Name = "会话"
		var count int64
		s.db.Model(&sysmodel.CustomerSession{}).
			Where("created_at BETWEEN ? AND ?", startTime, endTime).
			Count(&count)
		det.Count = count
	case "order":
		det.Name = "订单"
		var count int64
		s.db.Model(&sysmodel.Order{}).
			Where("created_at BETWEEN ? AND ?", startTime, endTime).
			Where("status IN ?", []string{"paid", "completed"}).
			Count(&count)
		det.Count = count
		if det.Count > 0 {
			det.Rate = 100
		}
	}

	return det, nil
}
