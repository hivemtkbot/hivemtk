package repository

// rag_metrics_repository.go RAG 召回率监控仓储（C 域 缺口 #2）
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6 召回率监控
//
// 职责:
//   - 写入 rag_query_logs（单条 / 批量）
// 聚合 rag_query_logs（基础指标 + 延迟偏移取值）
//   - 查询低召回样本
//   - upsert rag_metrics_daily（按 window_start + window_end 幂等）
//   - 查询最近 N 条聚合记录

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
)

// ----------------------------------------------------------------------------
// 聚合行结构
// ----------------------------------------------------------------------------

// RagQueryLogAggRow rag_query_logs 基础聚合行
//
// 与 service.RecallMetrics 字段含义对齐，但保持仓储层独立定义。
type RagQueryLogAggRow struct {
	Total        int64
	AvgRecall    float64
	AvgPrecision float64
	AvgLatency   float64
	ZeroHit      int64
	LowRecall    int64
}

// ----------------------------------------------------------------------------
// 仓储接口
// ----------------------------------------------------------------------------

// RagMetricsRepository RAG 召回率监控仓储接口
type RagMetricsRepository interface {
	// CreateQueryLog 创建单条 RAG 查询日志（同步写入）
	CreateQueryLog(ctx context.Context, log *model.RagQueryLog) error
	// CreateQueryLogsInBatches 批量创建 RAG 查询日志
	CreateQueryLogsInBatches(ctx context.Context, logs []*model.RagQueryLog, batchSize int) error
	// AggregateQueryLogs 聚合时间窗口内的查询日志（基础指标）
	AggregateQueryLogs(ctx context.Context, start, end time.Time, lowRecallThreshold float64) (*RagQueryLogAggRow, error)
	// PluckP99Latency 取 延迟（按 latency_ms 升序偏移取一条）
	PluckP99Latency(ctx context.Context, start, end time.Time, offset int) (int64, error)
	// FindLowRecallQueryLogs 查询召回率低于阈值的样本（按 created_at DESC）
	FindLowRecallQueryLogs(ctx context.Context, threshold float64, limit int) ([]model.RagQueryLog, error)
	// FindDailyByWindow 按 window_start + window_end 查找聚合记录（不存在返回 gorm.ErrRecordNotFound）
	FindDailyByWindow(ctx context.Context, start, end time.Time) (*model.RagMetricsDaily, error)
	// SaveDaily 保存（更新）聚合记录
	SaveDaily(ctx context.Context, daily *model.RagMetricsDaily) error
	// CreateDaily 创建聚合记录
	CreateDaily(ctx context.Context, daily *model.RagMetricsDaily) error
	// FindLatestDailies 查询最近 N 条聚合记录（按 window_start DESC，limit 已校验）
	FindLatestDailies(ctx context.Context, limit int) ([]model.RagMetricsDaily, error)
}

// ----------------------------------------------------------------------------
// 实现
// ----------------------------------------------------------------------------

type ragMetricsRepo struct {
	db *gorm.DB
}

// NewRagMetricsRepository 创建 RAG 召回率监控仓储
func NewRagMetricsRepository(db *gorm.DB) RagMetricsRepository {
	return &ragMetricsRepo{db: db}
}

// CreateQueryLog 创建单条 RAG 查询日志
func (r *ragMetricsRepo) CreateQueryLog(ctx context.Context, log *model.RagQueryLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// CreateQueryLogsInBatches 批量创建 RAG 查询日志
func (r *ragMetricsRepo) CreateQueryLogsInBatches(ctx context.Context, logs []*model.RagQueryLog, batchSize int) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(logs, batchSize).Error
}

// AggregateQueryLogs 聚合时间窗口内的查询日志
//
// SQL 与原实现保持一致：使用 COUNT(*) FILTER (WHERE ...) 统计 zero_hit / low_recall
func (r *ragMetricsRepo) AggregateQueryLogs(ctx context.Context, start, end time.Time, lowRecallThreshold float64) (*RagQueryLogAggRow, error) {
	row := &RagQueryLogAggRow{}
	if err := r.db.WithContext(ctx).
		Model(&model.RagQueryLog{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COALESCE(AVG(recall), 0) AS avg_recall,
			COALESCE(AVG(precision), 0) AS avg_precision,
			COALESCE(AVG(latency_ms), 0) AS avg_latency,
			COUNT(*) FILTER (WHERE retrieved_count = 0) AS zero_hit,
			COUNT(*) FILTER (WHERE recall < ?) AS low_recall
		`, lowRecallThreshold).
		Scan(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// PluckP99Latency 取 延迟
//
// 通过 Order + Offset + Limit(1) + Pluck 实现偏移法取百分位
func (r *ragMetricsRepo) PluckP99Latency(ctx context.Context, start, end time.Time, offset int) (int64, error) {
	var p99Latency int64
	if err := r.db.WithContext(ctx).
		Model(&model.RagQueryLog{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("latency_ms ASC").
		Offset(offset).
		Limit(1).
		Pluck("latency_ms", &p99Latency).Error; err != nil {
		return 0, err
	}
	return p99Latency, nil
}

// FindLowRecallQueryLogs 查询召回率低于阈值的样本
//
// 仅 SELECT 必要字段；按 created_at DESC 优先返回最近样本
func (r *ragMetricsRepo) FindLowRecallQueryLogs(ctx context.Context, threshold float64, limit int) ([]model.RagQueryLog, error) {
	var rows []model.RagQueryLog
	err := r.db.WithContext(ctx).
		Model(&model.RagQueryLog{}).
		Where("recall < ? AND relevant_count > 0", threshold).
		Order("created_at DESC").
		Limit(limit).
		Select("id, query, session_id, recall, precision, latency_ms, retrieved_count, relevant_count, hit_count, created_at").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// FindDailyByWindow 按 window_start + window_end 查找聚合记录
func (r *ragMetricsRepo) FindDailyByWindow(ctx context.Context, start, end time.Time) (*model.RagMetricsDaily, error) {
	var existing model.RagMetricsDaily
	err := r.db.WithContext(ctx).
		Where("window_start = ? AND window_end = ?", start, end).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// SaveDaily 保存（更新）聚合记录
func (r *ragMetricsRepo) SaveDaily(ctx context.Context, daily *model.RagMetricsDaily) error {
	return r.db.WithContext(ctx).Save(daily).Error
}

// CreateDaily 创建聚合记录
func (r *ragMetricsRepo) CreateDaily(ctx context.Context, daily *model.RagMetricsDaily) error {
	return r.db.WithContext(ctx).Create(daily).Error
}

// FindLatestDailies 查询最近 N 条聚合记录（按 window_start DESC）
func (r *ragMetricsRepo) FindLatestDailies(ctx context.Context, limit int) ([]model.RagMetricsDaily, error) {
	var rows []model.RagMetricsDaily
	if err := r.db.WithContext(ctx).
		Order("window_start DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// IsRecordNotFound 判断是否为记录未找到错误
func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// 编译期断言
var _ RagMetricsRepository = (*ragMetricsRepo)(nil)
