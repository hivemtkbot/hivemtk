package repository

// rag_recall_monitor_repository.go RAG 召回率监控快照仓储（C 域 缺口 #2 - 监控指标）
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6 召回率监控
//
// 职责:
// 聚合 rag_query_logs 计算 Top-K / Top-1 命中率 / 平均相似度 / 延迟
//   - 写入 / 查询 rag_recall_monitor_snapshots 监控快照表
//   - 确保监控表存在（CREATE TABLE IF NOT EXISTS，部署初始化时调用）

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RagRecallMonitorTableName 监控快照表名
//
// 与 service 层常量保持一致；放此处以便 repository 内部使用，避免循环引用。
const RagRecallMonitorTableName = "rag_recall_monitor_snapshots"

// ----------------------------------------------------------------------------
// 聚合行结构
// ----------------------------------------------------------------------------

// RagRecallAggRow rag_query_logs 召回监控聚合行
//
// 包含 Top-K / Top-1 命中率与平均相似度等监控指标所需字段。
type RagRecallAggRow struct {
	Total         int64
	AvgRecall     float64
	AvgPrecision  float64
	AvgLatency    float64
	AvgSimilarity float64
	ZeroHit       int64
	LowRecall     int64
	Top1Hit       int64
}

// ----------------------------------------------------------------------------
// 仓储接口
// ----------------------------------------------------------------------------

// RagRecallMonitorRepository RAG 召回率监控仓储接口
type RagRecallMonitorRepository interface {
	// AggregateRecallLogs 聚合时间窗口内的召回日志
	AggregateRecallLogs(ctx context.Context, start, end time.Time) (*RagRecallAggRow, error)
	// PluckP95Latency 取 延迟（按 latency_ms 升序偏移取一条）
	PluckP95Latency(ctx context.Context, start, end time.Time, offset int) (int64, error)
	// CreateSnapshot 创建一条监控快照（row 字段与原 map[string]any 行为一致）
	CreateSnapshot(ctx context.Context, row map[string]any) error
	// ListSnapshots 列出最近 N 条快照（按 window_start DESC，limit 已校验）
	ListSnapshots(ctx context.Context, limit int) ([]map[string]any, error)
	// EnsureSchema 确保监控表存在（CREATE TABLE IF NOT EXISTS）
	EnsureSchema(ctx context.Context) error
}

// ----------------------------------------------------------------------------
// 实现
// ----------------------------------------------------------------------------

type ragRecallMonitorRepo struct {
	db *gorm.DB
}

// NewRagRecallMonitorRepository 创建 RAG 召回率监控仓储
func NewRagRecallMonitorRepository(db *gorm.DB) RagRecallMonitorRepository {
	return &ragRecallMonitorRepo{db: db}
}

// AggregateRecallLogs 聚合时间窗口内的召回日志
//
// SQL 与原实现保持一致：使用 COUNT(*) FILTER (WHERE ...) 统计 zero_hit / low_recall / top1_hit
func (r *ragRecallMonitorRepo) AggregateRecallLogs(ctx context.Context, start, end time.Time) (*RagRecallAggRow, error) {
	row := &RagRecallAggRow{}
	if err := r.db.WithContext(ctx).
		Table("rag_query_logs").
		Where("created_at >= ? AND created_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COALESCE(AVG(recall), 0) AS avg_recall,
			COALESCE(AVG(precision), 0) AS avg_precision,
			COALESCE(AVG(latency_ms), 0) AS avg_latency,
			COALESCE(AVG(top_similarity), 0) AS avg_similarity,
			COUNT(*) FILTER (WHERE retrieved_count = 0) AS zero_hit,
			COUNT(*) FILTER (WHERE recall < 0.3) AS low_recall,
			COUNT(*) FILTER (WHERE hit_in_top1 = true) AS top1_hit
		`).
		Scan(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// PluckP95Latency 取 延迟
//
// 通过 Order + Offset + Limit(1) + Pluck 实现偏移法取百分位
func (r *ragRecallMonitorRepo) PluckP95Latency(ctx context.Context, start, end time.Time, offset int) (int64, error) {
	var p95 int64
	if err := r.db.WithContext(ctx).
		Table("rag_query_logs").
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("latency_ms ASC").
		Offset(offset).
		Limit(1).
		Pluck("latency_ms", &p95).Error; err != nil {
		return 0, err
	}
	return p95, nil
}

// CreateSnapshot 创建一条监控快照
func (r *ragRecallMonitorRepo) CreateSnapshot(ctx context.Context, row map[string]any) error {
	return r.db.WithContext(ctx).Table(RagRecallMonitorTableName).Create(row).Error
}

// ListSnapshots 列出最近 N 条快照
func (r *ragRecallMonitorRepo) ListSnapshots(ctx context.Context, limit int) ([]map[string]any, error) {
	var rows []map[string]any
	if err := r.db.WithContext(ctx).
		Table(RagRecallMonitorTableName).
		Order("window_start DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// EnsureSchema 确保监控表存在
//
// 监控表不是核心业务表，使用 CREATE TABLE IF NOT EXISTS 模式，部署初始化时调用一次即可。
// 保留原 service 层的 DDL 文本以不改运行时行为。
func (r *ragRecallMonitorRepo) EnsureSchema(ctx context.Context) error {
	stmt := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			window_start TIMESTAMPTZ NOT NULL,
			window_end TIMESTAMPTZ NOT NULL,
			total_queries BIGINT NOT NULL DEFAULT 0,
			top_k_hit_rate NUMERIC(10,6) NOT NULL DEFAULT 0,
			top_1_hit_rate NUMERIC(10,6) NOT NULL DEFAULT 0,
			avg_recall NUMERIC(10,6) NOT NULL DEFAULT 0,
			avg_precision NUMERIC(10,6) NOT NULL DEFAULT 0,
			avg_similarity NUMERIC(10,6) NOT NULL DEFAULT 0,
			avg_latency_ms NUMERIC(12,2) NOT NULL DEFAULT 0,
			p95_latency_ms BIGINT NOT NULL DEFAULT 0,
			zero_hit_count BIGINT NOT NULL DEFAULT 0,
			low_recall_count BIGINT NOT NULL DEFAULT 0,
			payload TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_rag_recall_monitor_window ON %s (window_start DESC);
	`, RagRecallMonitorTableName, RagRecallMonitorTableName)
	return r.db.WithContext(ctx).Exec(stmt).Error
}

// 编译期断言
var _ RagRecallMonitorRepository = (*ragRecallMonitorRepo)(nil)
