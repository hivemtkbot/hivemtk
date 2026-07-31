package repository

// layer_decision_log.go Layer 决策日志 Repository
//
// 五层架构归属: L5 数据访问层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T4)
//   - 每次 Layer 决策 + Fallback 触发落一条, 私域巡检通过 SQL 聚合统计
//   - StatsByLayer / StatsByIntent 用于聚合查询
//   - 与 llm_routing_logs 互补: llm_routing_logs 关注 LLM 调用, 本表关注 Layer 决策
//
// 方法:
//   - Record           新增一条决策日志
//   - GetByTraceID     按 trace 查询 (端到端串联)
//   - StatsByLayer     按 (layer, day) 聚合
//   - StatsByIntent    按 (intent, day) 聚合
//   - Recent           查询最近 N 条

import (
	"context"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
)

type LayerDecisionLogRepository struct {
	db *gorm.DB
}

func NewLayerDecisionLogRepository(db *gorm.DB) *LayerDecisionLogRepository {
	return &LayerDecisionLogRepository{db: db}
}

// Record 新增决策日志
func (r *LayerDecisionLogRepository) Record(ctx context.Context, log *model.LayerDecisionLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByTraceID 按 trace 查询所有相关日志
func (r *LayerDecisionLogRepository) GetByTraceID(ctx context.Context, traceID string) ([]model.LayerDecisionLog, error) {
	if traceID == "" {
		return nil, nil
	}
	var logs []model.LayerDecisionLog
	err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Order("created_at ASC, id ASC").
		Find(&logs).Error
	return logs, err
}

// LayerStat Layer 维度统计
type LayerStat struct {
	Layer string
	Count int64
}

// StatsByLayer 按 layer 聚合 (since 指定起点)
func (r *LayerDecisionLogRepository) StatsByLayer(ctx context.Context, since time.Time) ([]LayerStat, error) {
	type row struct {
		Layer string
		Count int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&model.LayerDecisionLog{}).
		Select("layer, COUNT(*) AS count").
		Where("created_at >= ?", since).
		Group("layer").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]LayerStat, 0, len(rows))
	for _, x := range rows {
		out = append(out, LayerStat{Layer: x.Layer, Count: x.Count})
	}
	return out, nil
}

// IntentStat 意图维度统计
type IntentStat struct {
	Intent string
	Count  int64
}

// StatsByIntent 按 intent 聚合
func (r *LayerDecisionLogRepository) StatsByIntent(ctx context.Context, since time.Time) ([]IntentStat, error) {
	type row struct {
		Intent string
		Count  int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&model.LayerDecisionLog{}).
		Select("intent, COUNT(*) AS count").
		Where("created_at >= ?", since).
		Group("intent").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]IntentStat, 0, len(rows))
	for _, x := range rows {
		out = append(out, IntentStat{Intent: x.Intent, Count: x.Count})
	}
	return out, nil
}

// Recent 查询最近 N 条 (按时间倒序)
func (r *LayerDecisionLogRepository) Recent(ctx context.Context, limit int) ([]model.LayerDecisionLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []model.LayerDecisionLog
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// LLMSkippedCount 统计跳过 LLM 的次数 (Layer1 命中)
func (r *LayerDecisionLogRepository) LLMSkippedCount(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.LayerDecisionLog{}).
		Where("llm_skipped = ? AND created_at >= ?", true, since).
		Count(&count).Error
	return count, err
}
