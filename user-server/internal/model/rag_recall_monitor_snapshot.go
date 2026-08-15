package model

import "time"

// RagRecallMonitorSnapshot RAG 召回质量监控快照（按时间窗口聚合）。
//
// 列定义必须与 repository/rag_recall_monitor.go 的 EnsureSchema DDL 以及
// service 层 CollectAndStore 写入的 map key 完全一致，否则写入会因缺列而报
// "column ... does not exist"。此前该表由 Go 迁移/服务内 EnsureSchema 创建，
// 未登记进 AutoMigrate，导致 /api/rag/recall/snapshots 报 relation not exist；
// 登记后又因本模型列与 DDL 不一致，导致 /api/rag/recall/collect 写入缺列。
// 现严格对齐 EnsureSchema 的 15 个列。
type RagRecallMonitorSnapshot struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	WindowStart    time.Time `gorm:"column:window_start;type:timestamptz" json:"window_start"`
	WindowEnd      time.Time `gorm:"column:window_end;type:timestamptz" json:"window_end"`
	TotalQueries   int64     `gorm:"column:total_queries;type:bigint" json:"total_queries"`
	TopKHitRate    float64   `gorm:"column:top_k_hit_rate;type:numeric(10,6)" json:"top_k_hit_rate"`
	Top1HitRate    float64   `gorm:"column:top_1_hit_rate;type:numeric(10,6)" json:"top_1_hit_rate"`
	AvgRecall      float64   `gorm:"column:avg_recall;type:numeric(10,6)" json:"avg_recall"`
	AvgPrecision   float64   `gorm:"column:avg_precision;type:numeric(10,6)" json:"avg_precision"`
	AvgSimilarity  float64   `gorm:"column:avg_similarity;type:numeric(10,6)" json:"avg_similarity"`
	AvgLatencyMs   float64   `gorm:"column:avg_latency_ms;type:numeric(12,2)" json:"avg_latency_ms"`
	P95LatencyMs   int64     `gorm:"column:p95_latency_ms;type:bigint" json:"p95_latency_ms"`
	ZeroHitCount   int64     `gorm:"column:zero_hit_count;type:bigint" json:"zero_hit_count"`
	LowRecallCount int64     `gorm:"column:low_recall_count;type:bigint" json:"low_recall_count"`
	Payload        string    `gorm:"column:payload;type:text" json:"payload"`
	CreatedAt      time.Time `gorm:"column:created_at;type:timestamptz;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (RagRecallMonitorSnapshot) TableName() string {
	return "rag_recall_monitor_snapshots"
}

