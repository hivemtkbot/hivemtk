package trace_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/tracing"
	"marketing/internal/pkg/utils/logger"
	"gorm.io/gorm"
)

// Service 追踪自学习服务：聚合 trace → LLM 打分 → 调整知识库权重 → 记录审计。
type Service struct {
	db         *gorm.DB
	dispatcher *llm.Dispatcher
	cfg        Config
}

// New 构造服务；若评估场景无路由，则兜底复用 friendly_chat 路由，确保评估有 provider。
func New(db *gorm.DB, dispatcher *llm.Dispatcher, cfg Config) *Service {
	if dispatcher != nil && dispatcher.GetRoute(cfg.Scenario) == nil {
		if r := dispatcher.GetRoute(llm.ScenarioFriendlyChat); r != nil {
			dispatcher.SetRoute(llm.ScenarioRoute{
				Scenario:   cfg.Scenario,
				Provider:   r.Provider,
				Fallbacks:  r.Fallbacks,
				MaxLatency: r.MaxLatency,
			})
		}
	}
	return &Service{db: db, dispatcher: dispatcher, cfg: cfg}
}

// EvaluateTrace 评估单条 trace 并调整权重（幂等：同 trace_id 覆盖审计记录）。
func (s *Service) EvaluateTrace(ctx context.Context, traceID string) (*model.TraceEvalLog, error) {
	ctx = ensureCtx(ctx)
	agg, err := AggregateTrace(ctx, s.db, traceID)
	if err != nil {
		return nil, err
	}
	if agg == nil {
		return nil, fmt.Errorf("trace 不存在: %s", traceID)
	}
	res, err := Evaluate(ctx, s.dispatcher, s.cfg, agg)
	if err != nil {
		return nil, err
	}
	var adjusted []AdjustedChunk
	if len(agg.RecalledChunkIDs) > 0 {
		adjusted, err = AdjustWeights(ctx, s.db, agg.RecalledChunkIDs, res.Score, s.cfg)
		if err != nil {
			logger.Warnf("[trace_learning] 调整权重失败 trace=%s: %v", traceID, err)
		}
	}
	dimJSON, _ := json.Marshal(res.Dimensions)
	adjJSON, _ := json.Marshal(adjusted)
	log := &model.TraceEvalLog{
		TraceID:        traceID,
		ConversationID: agg.ConversationID,
		Channel:        agg.Channel,
		Score:          res.Score,
		DimensionsJSON: string(dimJSON),
		Reason:         res.Reason,
		Bad:            res.Bad,
		AdjustedChunks: string(adjJSON),
	}
	if e := s.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Assign(map[string]any{
			"conversation_id": log.ConversationID,
			"channel":         log.Channel,
			"score":           log.Score,
			"dimensions":      log.DimensionsJSON,
			"reason":          log.Reason,
			"bad":             log.Bad,
			"adjusted_chunks": log.AdjustedChunks,
		}).
		FirstOrCreate(log).Error; e != nil {
		return nil, e
	}
	return log, nil
}

// RunBatch 扫描最近未评估的 trace，批量评估+调权。返回处理条数。
func (s *Service) RunBatch(ctx context.Context, sinceHours, batchSize int) (int, error) {
	ctx = ensureCtx(ctx)
	if s.db == nil {
		return 0, fmt.Errorf("db nil")
	}
	if sinceHours <= 0 {
		sinceHours = s.cfg.SinceHours
	}
	if batchSize <= 0 || batchSize > 200 {
		batchSize = s.cfg.BatchSize
	}
	since := time.Now().Add(-time.Duration(sinceHours) * time.Hour)
	var traceIDs []string
	if err := s.db.WithContext(ctx).
		Table("message_trace").
		Select("trace_id").
		Where("created_at >= ?", since).
		Where("trace_id IN (SELECT trace_id FROM message_trace WHERE node = ? AND output LIKE ?)", tracing.NodeAIDispatch, `%"`+`reply`+`"%`).
		Where("trace_id NOT IN (SELECT trace_id FROM trace_eval_log)").
		Group("trace_id").
		Order("MAX(id) DESC").
		Limit(batchSize).
		Pluck("trace_id", &traceIDs).Error; err != nil {
		return 0, err
	}
	processed := 0
	for _, tid := range traceIDs {
		if _, e2 := s.EvaluateTrace(ctx, tid); e2 != nil {
			logger.Warnf("[trace_learning] 评估失败 trace=%s: %v", tid, e2)
			continue
		}
		processed++
	}
	return processed, nil
}

// Logs 查询最近打分记录（供前端展示）。
func (s *Service) Logs(ctx context.Context, limit int) ([]model.TraceEvalLog, error) {
	ctx = ensureCtx(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var logs []model.TraceEvalLog
	if err := s.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// TopWeights 知识库权重排行（取权重偏离 1.0 最大的 chunk，供前端展示「自学习影响」）。
func (s *Service) TopWeights(ctx context.Context, limit int) ([]map[string]any, error) {
	ctx = ensureCtx(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []map[string]any
	if err := s.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id, content, weight, hit_count").
		Where("weight <> 1").
		Order("abs(weight - 1) DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// marshalJSON 供外部复用（保持与 adjuster 同名函数一致）
var _ = marshalJSON
