package trace_learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

var adjustMu sync.Mutex

func traceLockKey(traceID string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(traceID))
	return int64(h.Sum32())
}

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
// dryRun=true 时仅评分+预览计划调整，不调权、不写审计（用于安全评估自学习质量）。
//
// 并发安全（2026-08-08 修复）：cron 批量评估与手动触发可能同时评估同一条 trace，
// 用 pg_advisory_xact_lock 在事务内串行化，配合幂等检查，杜绝双重调权（权重乘算漂移）。
func (s *Service) EvaluateTrace(ctx context.Context, traceID string, dryRun bool) (*model.TraceEvalLog, error) {
	return s.evaluateTraceOn(ctx, s.db, traceID, dryRun)
}

func (s *Service) evaluateTraceOn(ctx context.Context, db *gorm.DB, traceID string, dryRun bool) (*model.TraceEvalLog, error) {
	ctx = ensureCtx(ctx)
	agg, err := AggregateTrace(ctx, db, traceID)
	if err != nil {
		return nil, err
	}
	if agg == nil {
		return nil, fmt.Errorf("trace 不存在: %s", traceID)
	}
	res, err := Evaluate(ctx, s.dispatcher, s.cfg, agg)
	skipAdjust := false
	if err != nil {
		if errors.Is(err, ErrNoEvaluableContent) {
			if dryRun {
				return nil, nil
			}
			skipAdjust = true
			res = &EvalResult{
				Score:      0,
				Bad:        false,
				Reason:     "跳过：" + err.Error(),
				Dimensions: map[string]float64{},
			}
		} else {
			if !dryRun {
				if werr := s.persistAttemptedLog(ctx, db, traceID, agg, "评估出错: "+err.Error()); werr != nil {
					logger.Warnf("[trace_learning] 写错误尝试审计失败 trace=%s: %v", traceID, werr)
				}
			}
			return nil, err
		}
	}
	dimJSON, _ := json.Marshal(res.Dimensions)

	if dryRun {

		var adjusted []AdjustedChunk
		if len(agg.RecalledChunkIDs) > 0 {
			adjusted, _ = PreviewAdjustments(ctx, db, agg.RecalledChunkIDs, *res, s.cfg)
		}
		adjJSON, _ := json.Marshal(adjusted)
		return &model.TraceEvalLog{
			TraceID:        traceID,
			ConversationID: agg.ConversationID,
			Channel:        agg.Channel,
			Score:          res.Score,
			DimensionsJSON: string(dimJSON),
			Reason:         res.Reason,
			Bad:            res.Bad,
			AdjustedChunks: string(adjJSON),
		}, nil
	}

	var resultLog *model.TraceEvalLog
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", traceLockKey(traceID)).Error; e != nil {
			return e
		}

		var existing model.TraceEvalLog
		if e := tx.Where("trace_id = ?", traceID).First(&existing).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		alreadyEvaluated := existing.ID != 0

		var adjusted []AdjustedChunk
		if len(agg.RecalledChunkIDs) > 0 && !alreadyEvaluated && !skipAdjust {

			var e error
			adjustMu.Lock()
			adjusted, e = AdjustWeights(ctx, tx, agg.RecalledChunkIDs, *res, s.cfg)
			adjustMu.Unlock()
			if e != nil {
				logger.Warnf("[trace_learning] 调整权重失败 trace=%s: %v", traceID, e)
			}
		}
		adjJSON, _ := json.Marshal(adjusted)
		log := model.TraceEvalLog{
			TraceID:        traceID,
			ConversationID: agg.ConversationID,
			Channel:        agg.Channel,
			Score:          res.Score,
			DimensionsJSON: string(dimJSON),
			Reason:         res.Reason,
			Bad:            res.Bad,
			AdjustedChunks: string(adjJSON),
		}
		if e := tx.Where("trace_id = ?", traceID).Assign(log).FirstOrCreate(&log).Error; e != nil {
			return e
		}
		resultLog = &log
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	s.distillInsightForTrace(ctx, agg, res)
	return resultLog, nil
}

func (s *Service) persistAttemptedLog(ctx context.Context, db *gorm.DB, traceID string, agg *AggregatedTrace, reason string) error {
	log := model.TraceEvalLog{
		TraceID:        traceID,
		ConversationID: agg.ConversationID,
		Channel:        agg.Channel,
		Score:          0,
		DimensionsJSON: "{}",
		Reason:         reason,
		Bad:            false,
		AdjustedChunks: "[]",
	}
	return db.WithContext(ctx).Where("trace_id = ?", traceID).Assign(log).FirstOrCreate(&log).Error
}

const runBatchLockKey int64 = 9173001

// RunBatch 扫描所有「尚未评估」的 ai_dispatch trace 并批量打分+调权。
// 返回处理结果（含 dryRun 时的预览列表）。
//
// 关键修复（2026-08-08 审查）：
//   - 不再硬性 Limit(20)：按 batchSize 分批循环处理全部待评，消除高流量积压；
//   - 加 pg_try_advisory_lock 全局锁：cron 与手动触发并发时只跑一个实例（专用连接获取+释放，防泄漏）。
//   - sinceHours 现作为 opt-in 时间窗：>0 时仅评估该小时内的 trace；默认 0=评估全部未评估 trace（不漏评）。
//
// 性能优化（2026-08-09）：
//   - 并发评估：LLM 打分为瓶颈，用有界 worker 池（cfg.Concurrency）并行打分；
//   - 权重调整为快路径，由全局 adjustMu 串行化，避免跨 trace 召回同一 chunk 的丢失更新，且不损吞吐；
//   - 每 trace 评估在独立事务（s.db 连接池）内进行，pg_advisory_xact_lock 随事务结束自动释放，安全并发。
func (s *Service) RunBatch(ctx context.Context, sinceHours, batchSize int, dryRun bool) (*BatchResult, error) {
	ctx = ensureCtx(ctx)
	if s.db == nil {
		return nil, fmt.Errorf("db nil")
	}
	if batchSize <= 0 || batchSize > 500 {
		batchSize = s.cfg.BatchSize
	}
	conc := s.cfg.Concurrency
	if conc < 1 {
		conc = 4
	}

	result := &BatchResult{}
	var previewMu sync.Mutex
	var previewBuf []*model.TraceEvalLog

	connErr := s.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		var held bool
		if e := conn.Raw("SELECT pg_try_advisory_lock(?)", runBatchLockKey).Scan(&held).Error; e != nil {
			return e
		}
		if !held {
			logger.Infof("[trace_learning] RunBatch 跳过：已有实例在运行")
			return nil
		}
		defer func() { _ = conn.Exec("SELECT pg_advisory_unlock(?)", runBatchLockKey).Error }()

		stalled := 0
		for {
			sub := conn.WithContext(ctx).Table("message_trace").
				Select("trace_id").
				Where("node = ?", tracing.NodeAIDispatch).
				Where("output::text LIKE ?", `%"`+`reply`+`"%`)
			if sinceHours > 0 {
				sub = sub.Where("created_at >= now() - make_interval(hours => ?)", sinceHours)
			}
			var traceIDs []string
			if err := conn.WithContext(ctx).
				Table("message_trace").
				Select("trace_id").
				Where("trace_id IN (?)", sub).
				Where("trace_id NOT IN (SELECT trace_id FROM trace_eval_log)").
				Group("trace_id").
				Order("MAX(id) ASC").
				Limit(batchSize).
				Pluck("trace_id", &traceIDs).Error; err != nil {
				return err
			}
			if len(traceIDs) == 0 {
				break
			}
			sem := make(chan struct{}, conc)
			var wg sync.WaitGroup
			var processed int32
			for _, tid := range traceIDs {
				wg.Add(1)
				go func(tid string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					log, e2 := s.evaluateTraceOn(ctx, s.db, tid, dryRun)
					if e2 != nil {
						logger.Warnf("[trace_learning] 评估失败 trace=%s: %v", tid, e2)
						return
					}
					atomic.AddInt32(&processed, 1)
					if dryRun {
						previewMu.Lock()
						previewBuf = append(previewBuf, log)
						previewMu.Unlock()
					}
				}(tid)
			}
			wg.Wait()
			result.Processed += int(atomic.LoadInt32(&processed))
			if dryRun {
				previewMu.Lock()
				result.Previews = append(result.Previews, previewBuf...)
				previewBuf = previewBuf[:0]
				previewMu.Unlock()
			}
			if processed == 0 && len(traceIDs) >= batchSize {
				stalled++
				if stalled >= 2 {
					logger.Warnf("[trace_learning] RunBatch 连续 %d 轮满批无成功评估，提前退出避免死循环", stalled)
					break
				}
			} else {
				stalled = 0
			}
			if len(traceIDs) < batchSize {
				break
			}
		}
		return nil
	})
	if connErr != nil {
		return result, connErr
	}
	return result, nil
}

// BatchResult RunBatch 处理结果
type BatchResult struct {
	Processed int
	Previews  []*model.TraceEvalLog
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

var _ = marshalJSON
