package trace_learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/tracing"
	"marketing/internal/pkg/utils/logger"
	"gorm.io/gorm"
)

// traceLockKey 把 trace_id 映射为 int64 咨询锁 key（FNV-32，碰撞仅导致偶发串行化，不影响正确性）。
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
//
// 并发安全（2026-08-08 修复）：cron 批量评估与手动触发可能同时评估同一条 trace，
// 用 pg_advisory_xact_lock 在事务内串行化，配合幂等检查，杜绝双重调权（权重乘算漂移）。
func (s *Service) EvaluateTrace(ctx context.Context, traceID string) (*model.TraceEvalLog, error) {
	return s.evaluateTraceOn(ctx, s.db, traceID)
}

// evaluateTraceOn 在指定 db 句柄上评估单条 trace。
// db 可传入 RunBatch 持有的专用连接，使全局咨询锁的获取与释放在同一物理连接上完成，
// 避免连接池把「获取锁」与「释放锁」落到不同连接导致锁泄漏（会话级咨询锁需同连接释放）。
func (s *Service) evaluateTraceOn(ctx context.Context, db *gorm.DB, traceID string) (*model.TraceEvalLog, error) {
	ctx = ensureCtx(ctx)
	agg, err := AggregateTrace(ctx, db, traceID)
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

	var resultLog *model.TraceEvalLog
	// 事务内：加咨询锁 → 幂等检查 → 调权 → 写审计，整段串行化，避免并发双调。
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", traceLockKey(traceID)).Error; e != nil {
			return e
		}
		// 幂等：若已评估过，仅刷新审计分数，不再重复调权（权重是乘算，重复评估会叠加漂移）。
		var existing model.TraceEvalLog
		if e := tx.Where("trace_id = ?", traceID).First(&existing).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		alreadyEvaluated := existing.ID != 0

		var adjusted []AdjustedChunk
		if len(agg.RecalledChunkIDs) > 0 && !alreadyEvaluated {
			// 注意：必须用 `=` 赋值给外层 adjusted，不能用 `:=` 否则会遮蔽外层变量，
			// 导致下面写入审计日志的 adjusted_chunks 恒为 null（权重其实已调，但审计丢失）。
			var e error
			adjusted, e = AdjustWeights(ctx, tx, agg.RecalledChunkIDs, *res, s.cfg)
			if e != nil {
				logger.Warnf("[trace_learning] 调整权重失败 trace=%s: %v", traceID, e)
			}
		}
		dimJSON, _ := json.Marshal(res.Dimensions)
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
		// 用结构体 Assign：列名由 GORM 推导为 dimensions_json / adjusted_chunks，
		// 避免手填 map 键名与真实列名不一致导致更新失败、进而无限重评。
		if e := tx.Where("trace_id = ?", traceID).Assign(log).FirstOrCreate(&log).Error; e != nil {
			return e
		}
		resultLog = &log
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return resultLog, nil
}

// runBatchLockKey 全局咨询锁 key：防止 cron 与手动触发并发跑 RunBatch 造成重复评估。
const runBatchLockKey int64 = 9173001

// RunBatch 扫描所有「尚未评估」的 ai_dispatch trace 并批量打分+调权。返回处理条数。
//
// 关键修复（2026-08-08 审查）：
//   - 不再按 sinceHours 时间窗过滤：原 `created_at >= now()-24h` 会让超龄 trace 永久漏评；
//   - 不再硬性 Limit(20)：改为按 batchSize 分批循环处理全部待评，消除高流量积压；
//   - 修复积压/漏评根因：原 `Limit(20)` + `sinceHours`(24h) 时间窗会让高流量积压、超龄 trace 永久漏评；
//   - 加 pg_try_advisory_lock 全局锁：cron 与手动触发并发时只跑一个实例。
//
// 修复（2026-08-08 测试发现）：原实现用 `s.db.Raw(...).Scan` 获取会话级咨询锁、再 `defer s.db.Exec`
// 释放，二者分别从连接池取连接，锁常落在 A 连接、释放在 B 连接 → 解锁失败、锁泄漏；此后所有
// RunBatch（含 cron 与手动触发）的 pg_try_advisory_lock 恒返回 false，评估彻底停摆。
// 现改为在 `db.Connection` 专用连接上获取+释放同一把锁，杜绝泄漏。
func (s *Service) RunBatch(ctx context.Context, sinceHours, batchSize int) (int, error) {
	ctx = ensureCtx(ctx)
	if s.db == nil {
		return 0, fmt.Errorf("db nil")
	}
	if batchSize <= 0 || batchSize > 500 {
		batchSize = s.cfg.BatchSize // 200
	}

	var processed int
	// 在专用连接上获取+释放全局咨询锁，确保解锁落回获取锁的同一物理连接（防泄漏）。
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

		for {
			var traceIDs []string
			if err := conn.WithContext(ctx).
				Table("message_trace").
				Select("trace_id").
				Where("trace_id IN (SELECT trace_id FROM message_trace WHERE node = ? AND output::text LIKE ?)", tracing.NodeAIDispatch, `%"`+`reply`+`"%`).
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
			for _, tid := range traceIDs {
				if _, e2 := s.evaluateTraceOn(ctx, conn, tid); e2 != nil {
					logger.Warnf("[trace_learning] 评估失败 trace=%s: %v", tid, e2)
					continue
				}
				processed++
			}
			// 本批已全评估完；不足一批说明已到底，结束。
			if len(traceIDs) < batchSize {
				break
			}
		}
		return nil
	})
	if connErr != nil {
		return processed, connErr
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
