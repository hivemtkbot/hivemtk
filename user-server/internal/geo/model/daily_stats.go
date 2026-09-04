package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoDailyStat GEO 每日统计聚合表（v3 统计层 G4.1）
//
// 按日期 × 引擎 × 意图 三元组唯一，对 geo_probe_runs 做预聚合。
// 写入由 ProbeAnalyzer 定时任务完成，读取端直接查此表避免实时大聚合。
type GeoDailyStat struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	Date                     string    `gorm:"column:stat_date;size:10;index:idx_date_engine_intent,uniqueIndex" json:"date"`
	Engine                   string    `gorm:"column:engine;size:32;index:idx_date_engine_intent,uniqueIndex" json:"engine"`
	Intent                   string    `gorm:"column:intent;size:64;index:idx_date_engine_intent,uniqueIndex" json:"intent"`
	FunnelStage              string    `gorm:"column:funnel_stage;size:20;index" json:"funnel_stage"`
	BrandMentionedCount      int       `gorm:"column:brand_mentioned_count" json:"brand_mentioned_count"`
	CompetitorMentionedCount int       `gorm:"column:competitor_mentioned_count" json:"competitor_mentioned_count"`
	CitationCount            int       `gorm:"column:citation_count" json:"citation_count"`
	NegativeCount            int       `gorm:"column:negative_count" json:"negative_count"`
	ProbeCount               int       `gorm:"column:probe_count;default:0" json:"probe_count"` // 当日该组探针总次数（可见率分母）
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	DeletedAt                gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoDailyStat) TableName() string { return "geo_daily_stats" }
