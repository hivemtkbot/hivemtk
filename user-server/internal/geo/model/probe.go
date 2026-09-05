package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// GeoProbeRun AI 搜索引擎探针运行记录（v3 GEO Probe 模块核心表）
//
// 每次对 AI 引擎（Perplexity / Copilot / Gemini 等）发起的探测查询
// 都会落一条此记录，支撑后续品牌提及率、情感倾向、竞品对比的
// 聚合统计（由 geo_daily_stats 按日期维度二次汇总）。
type GeoProbeRun struct {
	ID                  uint           `gorm:"primaryKey;column:id" json:"id"`
	Engine              string         `gorm:"column:engine;size:32;index" json:"engine"`
	Query               string         `gorm:"column:query;size:1024;index" json:"query"`
	Response            string         `gorm:"column:response;type:text" json:"response"`
	Citations           datatypes.JSON `gorm:"column:citations;type:jsonb" json:"citations"`
	Sentiment           string         `gorm:"column:sentiment;size:20" json:"sentiment"`
	BrandMentioned      bool           `gorm:"column:brand_mentioned" json:"brand_mentioned"`
	CompetitorMentioned bool           `gorm:"column:competitor_mentioned" json:"competitor_mentioned"`
	LatencyMs           int64          `gorm:"column:latency_ms" json:"latency_ms"`
	CreatedAt           time.Time      `gorm:"column:created_at;index" json:"created_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoProbeRun) TableName() string { return "geo_probe_runs" }
