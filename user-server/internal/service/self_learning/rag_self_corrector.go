package selflearning

// rag_self_corrector.go RAG 自我矫正器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §3 §7.3
//
// 职责（v1.1 §3 三阶段闭环）：
//   1. Warmup   预热阶段（dialogue.started 触发）：
//      - 缓存 Top-K 召回结果（TTL 5min）
//      - 加速首条回复延迟
//   2. Reflect  反思阶段（dialogue.ended 触发）：
//      - 基于 reward 评估本次会话用到的 RAG 语料质量
//      - reward >= 1.5 → 视为销冠对话 → 调用 ChampionUpsert 补录到 knowledge_chunks
//      - reward <= -1.0 或 outcome=abandoned → 视为低质召回 → 调用 IncrementLowQualityHits
//      - reward 在中间区间 → 不调整（保持稳定）
//   3. Correct  矫正阶段（由 SelfCorrectionDispatcher 触发）：
//      - 接收 7 类修复策略中的 RAG 相关 4 类
//      - retrieve_retry / query_rewrite / chunk_archive / chunk_champion_upsert
//
// 全自动执行约束（v1.1 §7.4）：
//   - 每次动作前调用 SwitchService.CheckGuardrail
//   - autonomous 等级直接执行
//   - supervised 等级写入 SelfCorrectionAction(status=pending) 待人工确认
//   - manual 等级仅记录意图（status=skipped）

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	// defaultChampionThreshold 默认销冠补录 reward 阈值
	defaultChampionThreshold = 1.5
	// lowQualityRewardThreshold 默认低质标记 reward 阈值（下界）
	lowQualityRewardThreshold = -1.0
	// defaultWarmupTTL 默认预热 TTL
	defaultWarmupTTL = 5 * time.Minute
	// logInputMaxLen 日志输入摘要的最大长度（字节）
	logInputMaxLen = 200
)

// RAGSelfCorrector RAG 自我矫正器
type RAGSelfCorrector struct {
	switchSvc     *SwitchService
	chunkExtRepo  repository.KnowledgeChunkExtRepository
	logRepo       repository.SelfLearningLogRepository
	actionRepo    repository.SelfCorrectionActionRepository
	ragEngine     RAGEngine
	publisher     *DialogueEventPublisher
	// 预热 TTL（默认 5min）
	warmupTTL time.Duration
}

// NewRAGSelfCorrector 创建 RAG 自我矫正器
func NewRAGSelfCorrector(
	switchSvc *SwitchService,
	chunkExtRepo repository.KnowledgeChunkExtRepository,
	logRepo repository.SelfLearningLogRepository,
	actionRepo repository.SelfCorrectionActionRepository,
	ragEngine RAGEngine,
	publisher *DialogueEventPublisher,
) *RAGSelfCorrector {
	return &RAGSelfCorrector{
		switchSvc:    switchSvc,
		chunkExtRepo: chunkExtRepo,
		logRepo:      logRepo,
		actionRepo:   actionRepo,
		ragEngine:    ragEngine,
		publisher:    publisher,
		warmupTTL:    defaultWarmupTTL,
	}
}

// SetWarmupTTL 设置预热 TTL（仅供测试或定制化使用）
func (r *RAGSelfCorrector) SetWarmupTTL(ttl time.Duration) {
	if ttl > 0 {
		r.warmupTTL = ttl
	}
}

// ============================================================================
// 1. Warmup 预热阶段（dialogue.started 触发）
// ============================================================================

// Warmup 预热：缓存 Top-K 召回结果
//
// 触发：dialogue.started 事件
// 用途：加速首条回复延迟，同时为后续 Reflect 阶段准备语料 ID 列表
//
// 失败容忍：预热失败不影响主流程，仅记录日志
func (r *RAGSelfCorrector) Warmup(ctx context.Context, payload *event.DialogueStartedPayload) error {
	if payload == nil {
		return fmt.Errorf("payload is nil")
	}
	// 护栏检查
	snap, err := r.switchSvc.GetStatus(ctx)
	if err != nil {
		return err
	}
	if !snap.EnableRAG {
		return nil // RAG 自我学习未启用
	}
	// 记录 self_learning_logs
	logID := GenLogID(payload.SessionID, model.ScenarioRagWarmup)
	startedAt := time.Now()
	inputSummary := map[string]any{
		"session_id":    payload.SessionID,
		"first_message": truncate(payload.FirstMessage, logInputMaxLen),
		"scenario":      payload.Scenario,
		"trace_id":      payload.TraceID,
	}
	// 幂等检查
	exists, err := r.logRepo.ExistsBySessionAndScenario(ctx, payload.SessionID, model.ScenarioRagWarmup)
	if err != nil {
		log.Printf("[rag_self_corrector] warmup exists check failed: %v", err)
	} else if exists {
		// 已预热，跳过
		return nil
	}
	logEntity := &model.SelfLearningLog{
		LogID:        logID,
		SessionID:    payload.SessionID,
		TraceID:      payload.TraceID,
		Scenario:     model.ScenarioRagWarmup,
		TriggerEvent: model.TriggerEventDialogueStarted,
		Status:       model.SelfLearningStatusRunning,
		InputSummary: inputSummary,
		StartedAt:    startedAt,
	}
	if err := r.logRepo.Create(ctx, logEntity); err != nil {
		// UNIQUE 冲突 = 已存在，幂等返回
		// 其他 DB 错误（连接断/字段超长等）必须上抛，不能掩盖
		if errors.Is(err, repository.ErrDuplicateLog) {
			return nil
		}
		return fmt.Errorf("rag_warmup create log: %w", err)
	}

	// 执行预热
	if r.ragEngine == nil {
		if uerr := r.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSkipped, "rag engine is nil", nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[rag_self_corrector] warmup update_status(skipped, rag_engine_nil) failed: log=%s err=%v", logID, uerr)
		}
		return nil
	}
	if payload.FirstMessage == "" {
		if uerr := r.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSkipped, "first_message is empty", nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[rag_self_corrector] warmup update_status(skipped, empty_msg) failed: log=%s err=%v", logID, uerr)
		}
		return nil
	}
	err = r.ragEngine.Warmup(ctx, payload.SessionID, payload.FirstMessage, r.warmupTTL)
	durationMs := int64(time.Since(startedAt).Milliseconds())
	if err != nil {
		if uerr := r.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusFailed, err.Error(), nil, durationMs); uerr != nil {
			log.Printf("[rag_self_corrector] warmup update_status(failed) failed: log=%s err=%v", logID, uerr)
		}
		log.Printf("[rag_self_corrector] warmup failed: session=%s err=%v", payload.SessionID, err)
		return nil
	}
	output := map[string]any{
		"ttl_sec":   int(r.warmupTTL.Seconds()),
		"warmed_at": time.Now().Format(time.RFC3339),
	}
	if uerr := r.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSuccess, "", output, durationMs); uerr != nil {
		log.Printf("[rag_self_corrector] warmup update_status(success) failed: log=%s err=%v", logID, uerr)
	}
	return nil
}

// ============================================================================
// 2. Reflect 反思阶段（dialogue.ended 触发）
// ============================================================================

// Reflect 反思：基于 reward 评估本次会话用到的 RAG 语料质量
//
// 触发：dialogue.ended 事件
// 决策矩阵（v1.1 §3.3）：
//   reward >= 1.5 & outcome=converted → ChampionUpsert（销冠补录）
//   reward <= -1.0 OR outcome=abandoned → 低质标记（IncrementLowQualityHits）
//   -1.0 < reward < 1.5 → 不调整
//
// 关键输入：payload.UsedCorpusIDs（本次会话用到的 RAG 语料 ID 列表）
func (r *RAGSelfCorrector) Reflect(ctx context.Context, payload *event.DialogueEndedPayload) error {
	if payload == nil {
		return fmt.Errorf("payload is nil")
	}
	snap, err := r.switchSvc.GetStatus(ctx)
	if err != nil {
		return err
	}
	if !snap.EnableRAG {
		return nil
	}
	// 幂等检查
	exists, err := r.logRepo.ExistsBySessionAndScenario(ctx, payload.SessionID, model.ScenarioRagReflect)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// 创建日志
	logID := GenLogID(payload.SessionID, model.ScenarioRagReflect)
	startedAt := time.Now()
	logEntity := &model.SelfLearningLog{
		LogID:        logID,
		SessionID:    payload.SessionID,
		TraceID:      payload.TraceID,
		Scenario:     model.ScenarioRagReflect,
		TriggerEvent: model.TriggerEventDialogueEnded,
		Status:       model.SelfLearningStatusRunning,
		InputSummary: map[string]any{
			"session_id":      payload.SessionID,
			"aggregated_reward": payload.AggregatedReward,
			"outcome":         payload.Outcome,
			"used_corpus_ids": payload.UsedCorpusIDs,
			"used_asset_ids":  payload.UsedAssetIDs,
			"duration_sec":    payload.DurationSec,
			"trace_id":        payload.TraceID,
		},
		StartedAt: startedAt,
	}
	if err := r.logRepo.Create(ctx, logEntity); err != nil {
		// UNIQUE 冲突 = 已处理；其他 DB 错误上抛
		if errors.Is(err, repository.ErrDuplicateLog) {
			return nil
		}
		return fmt.Errorf("rag_reflect create log: %w", err)
	}

	// 决策
	var actions []*CorrectionResult
	championThreshold := snap.ChampionRewardThreshold
	if championThreshold == 0 {
		championThreshold = defaultChampionThreshold
	}

	// 2.1 销冠补录
	if payload.AggregatedReward >= championThreshold && len(payload.UsedCorpusIDs) > 0 {
		for _, corpusIDStr := range payload.UsedCorpusIDs {
			corpusID, parseErr := parseUint64(corpusIDStr)
			if parseErr != nil {
				continue
			}
			res, cErr := r.applyChampionUpsert(ctx, logID, corpusID, payload.AggregatedReward, payload.SessionID, snap)
			if cErr != nil {
				log.Printf("[rag_self_corrector] champion upsert failed: corpus=%d err=%v", corpusID, cErr)
				continue
			}
			actions = append(actions, res)
		}
	}

	// 2.2 低质标记
	if (payload.AggregatedReward <= lowQualityRewardThreshold || payload.Outcome == "abandoned") && len(payload.UsedCorpusIDs) > 0 {
		for _, corpusIDStr := range payload.UsedCorpusIDs {
			corpusID, parseErr := parseUint64(corpusIDStr)
			if parseErr != nil {
				continue
			}
			res, cErr := r.applyLowQualityMark(ctx, logID, corpusID, payload.SessionID, snap)
			if cErr != nil {
				log.Printf("[rag_self_corrector] low_quality mark failed: corpus=%d err=%v", corpusID, cErr)
				continue
			}
			actions = append(actions, res)
		}
	}

	// 更新日志
	durationMs := int64(time.Since(startedAt).Milliseconds())
	output := map[string]any{
		"actions_count":    len(actions),
		"reward":           payload.AggregatedReward,
		"outcome":          payload.Outcome,
		"action_summaries": summarizeActions(actions),
	}
	if uerr := r.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSuccess, "", output, durationMs); uerr != nil {
		log.Printf("[rag_self_corrector] reflect update_status(success) failed: log=%s err=%v", logID, uerr)
	}
	return nil
}

// ============================================================================
// 3. 矫正动作执行（由 SelfCorrectionDispatcher 或 Reflect 内部触发）
// ============================================================================

// applyChampionUpsert 执行销冠补录
//
// 行为：
//   - 调用 KnowledgeChunkExtRepository.IncrementChampionHits 累加销冠命中
//   - quality_label 自动升级为 champion
//   - 发布 rag.corpus.updated 事件
//   - 记录 SelfCorrectionAction 审计
func (r *RAGSelfCorrector) applyChampionUpsert(ctx context.Context, triggerLogID string, chunkID uint64, reward float64, sourceSessionID string, snap *SwitchSnapshot) (*CorrectionResult, error) {
	// 护栏检查
	allow, reason, err := r.switchSvc.ShouldExecuteAction(ctx, model.CorrectionChampionUpsert)
	if err != nil {
		return nil, err
	}
	if !allow {
		return r.recordSkippedAction(ctx, triggerLogID, model.CorrectionChampionUpsert, fmt.Sprintf("%d", chunkID), reason), nil
	}

	// 执行前快照
	before, berr := r.chunkExtRepo.GetExt(ctx, chunkID)
	if berr != nil {
		log.Printf("[rag_self_corrector] champion_upsert get_ext(before) failed: chunk=%d err=%v", chunkID, berr)
	}

	// 执行矫正
	if err := r.chunkExtRepo.IncrementChampionHits(ctx, chunkID, 1, reward, sourceSessionID); err != nil {
		return r.recordFailedAction(ctx, triggerLogID, model.CorrectionChampionUpsert, fmt.Sprintf("%d", chunkID), err), nil
	}

	// 执行后快照
	after, aerr := r.chunkExtRepo.GetExt(ctx, chunkID)
	if aerr != nil {
		log.Printf("[rag_self_corrector] champion_upsert get_ext(after) failed: chunk=%d err=%v", chunkID, aerr)
	}

	// 记录审计动作
	actionID := GenActionID(triggerLogID, model.CorrectionChampionUpsert, fmt.Sprintf("%d", chunkID))
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  triggerLogID,
		ActionType:    model.CorrectionChampionUpsert,
		Scenario:      "rag",
		TargetType:    "rag_chunk",
		TargetID:      fmt.Sprintf("%d", chunkID),
		Before:        extToMap(before),
		After:         extToMap(after),
		AutonomyLevel: snap.AutonomyLevel,
		Operator:      "auto",
		Reason:        fmt.Sprintf("champion upsert: reward=%.3f source_session=%s", reward, sourceSessionID),
		Status:        model.CorrectionStatusApplied,
		AppliedAt:     nowPtr(),
	}
	if err := r.actionRepo.Create(ctx, action); err != nil {
		log.Printf("[rag_self_corrector] champion_upsert action_repo.Create failed: action=%s err=%v", actionID, err)
	}

	// 配额统计 & 熔断器更新
	if rerr := r.switchSvc.RecordCorrectionAction(ctx, model.CorrectionChampionUpsert, true, false); rerr != nil {
		log.Printf("[rag_self_corrector] champion_upsert record_correction failed: chunk=%d err=%v", chunkID, rerr)
	}

	// 发布语料变更事件
	if r.publisher != nil {
		if perr := r.publisher.PublishRagCorpusUpdated(ctx, &event.RagCorpusUpdatedPayload{
			CorpusID:        fmt.Sprintf("%d", chunkID),
			Action:          "champion_upsert",
			SourceSessionID: sourceSessionID,
			NewQualityLabel: string(model.QualityLabelChampion),
			TraceID:         triggerLogID,
			UpdatedAt:       time.Now(),
		}); perr != nil {
			log.Printf("[rag_self_corrector] champion_upsert publish_event failed: chunk=%d err=%v", chunkID, perr)
		}
	}

	return &CorrectionResult{
		ActionID:   actionID,
		ActionType: model.CorrectionChampionUpsert,
		TargetID:   fmt.Sprintf("%d", chunkID),
		Status:     model.CorrectionStatusApplied,
		Reason:     action.Reason,
		Before:     extToMap(before),
		After:      extToMap(after),
		AppliedAt:  time.Now(),
	}, nil
}

// applyLowQualityMark 执行低质标记
//
// 行为：
//   - 调用 KnowledgeChunkExtRepository.IncrementLowQualityHits
//   - 累加 hits >= 3 → quality_label=low_quality
//   - 累加 hits >= 10 → quality_label=archived（从召回池剔除）
//   - 发布 rag.corpus.updated 事件
func (r *RAGSelfCorrector) applyLowQualityMark(ctx context.Context, triggerLogID string, chunkID uint64, sourceSessionID string, snap *SwitchSnapshot) (*CorrectionResult, error) {
	allow, reason, err := r.switchSvc.ShouldExecuteAction(ctx, model.CorrectionChunkArchive)
	if err != nil {
		return nil, err
	}
	if !allow {
		return r.recordSkippedAction(ctx, triggerLogID, model.CorrectionChunkArchive, fmt.Sprintf("%d", chunkID), reason), nil
	}

	before, berr := r.chunkExtRepo.GetExt(ctx, chunkID)
	if berr != nil {
		log.Printf("[rag_self_corrector] low_quality_mark get_ext(before) failed: chunk=%d err=%v", chunkID, berr)
	}

	if err := r.chunkExtRepo.IncrementLowQualityHits(ctx, chunkID, 1, sourceSessionID); err != nil {
		return r.recordFailedAction(ctx, triggerLogID, model.CorrectionChunkArchive, fmt.Sprintf("%d", chunkID), err), nil
	}

	after, aerr := r.chunkExtRepo.GetExt(ctx, chunkID)
	if aerr != nil {
		log.Printf("[rag_self_corrector] low_quality_mark get_ext(after) failed: chunk=%d err=%v", chunkID, aerr)
	}

	actionID := GenActionID(triggerLogID, model.CorrectionChunkArchive, fmt.Sprintf("%d", chunkID))
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  triggerLogID,
		ActionType:    model.CorrectionChunkArchive,
		Scenario:      "rag",
		TargetType:    "rag_chunk",
		TargetID:      fmt.Sprintf("%d", chunkID),
		Before:        extToMap(before),
		After:         extToMap(after),
		AutonomyLevel: snap.AutonomyLevel,
		Operator:      "auto",
		Reason:        fmt.Sprintf("low_quality mark: source_session=%s", sourceSessionID),
		Status:        model.CorrectionStatusApplied,
		AppliedAt:     nowPtr(),
	}
	if err := r.actionRepo.Create(ctx, action); err != nil {
		log.Printf("[rag_self_corrector] low_quality_mark action_repo.Create failed: action=%s err=%v", actionID, err)
	}

	if rerr := r.switchSvc.RecordCorrectionAction(ctx, model.CorrectionChunkArchive, true, false); rerr != nil {
		log.Printf("[rag_self_corrector] low_quality_mark record_correction failed: chunk=%d err=%v", chunkID, rerr)
	}

	if r.publisher != nil {
		newLabel := "low_quality"
		if after != nil {
			newLabel = string(after.QualityLabel)
		}
		if perr := r.publisher.PublishRagCorpusUpdated(ctx, &event.RagCorpusUpdatedPayload{
			CorpusID:        fmt.Sprintf("%d", chunkID),
			Action:          "low_quality_mark",
			SourceSessionID: sourceSessionID,
			NewQualityLabel: newLabel,
			TraceID:         triggerLogID,
			UpdatedAt:       time.Now(),
		}); perr != nil {
			log.Printf("[rag_self_corrector] low_quality_mark publish_event failed: chunk=%d err=%v", chunkID, perr)
		}
	}

	return &CorrectionResult{
		ActionID:   actionID,
		ActionType: model.CorrectionChunkArchive,
		TargetID:   fmt.Sprintf("%d", chunkID),
		Status:     model.CorrectionStatusApplied,
		Reason:     action.Reason,
		Before:     extToMap(before),
		After:      extToMap(after),
		AppliedAt:  time.Now(),
	}, nil
}

// ============================================================================
// 工具方法
// ============================================================================

// recordSkippedAction 记录跳过的动作（护栏拦截）
func (r *RAGSelfCorrector) recordSkippedAction(ctx context.Context, triggerLogID string, actionType model.CorrectionActionType, targetID, reason string) *CorrectionResult {
	actionID := GenActionID(triggerLogID, actionType, targetID)
	if err := r.actionRepo.Create(ctx, &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  triggerLogID,
		ActionType:    actionType,
		Scenario:      "rag",
		TargetType:    "rag_chunk",
		TargetID:      targetID,
		AutonomyLevel: model.AutonomyLevelManual, // 标记为 manual 等级
		Operator:      "auto",
		Reason:        reason,
		Status:        model.CorrectionStatusSkipped,
	}); err != nil {
		log.Printf("[rag_self_corrector] record_skipped action_repo.Create failed: action=%s err=%v", actionID, err)
	}
	return &CorrectionResult{
		ActionID:   actionID,
		ActionType: actionType,
		TargetID:   targetID,
		Status:     model.CorrectionStatusSkipped,
		Reason:     reason,
		AppliedAt:  time.Now(),
	}
}

// recordFailedAction 记录失败的动作
func (r *RAGSelfCorrector) recordFailedAction(ctx context.Context, triggerLogID string, actionType model.CorrectionActionType, targetID string, err error) *CorrectionResult {
	actionID := GenActionID(triggerLogID, actionType, targetID)
	if cerr := r.actionRepo.Create(ctx, &model.SelfCorrectionAction{
		ActionID:     actionID,
		TriggerLogID: triggerLogID,
		ActionType:   actionType,
		Scenario:     "rag",
		TargetType:   "rag_chunk",
		TargetID:     targetID,
		Operator:     "auto",
		Reason:       err.Error(),
		Status:       model.CorrectionStatusFailed,
		ErrorMsg:     err.Error(),
	}); cerr != nil {
		log.Printf("[rag_self_corrector] record_failed action_repo.Create failed: action=%s err=%v", actionID, cerr)
	}
	// 失败也计入熔断器
	if rerr := r.switchSvc.RecordCorrectionAction(ctx, actionType, false, false); rerr != nil {
		log.Printf("[rag_self_corrector] record_failed record_correction failed: action=%s err=%v", actionID, rerr)
	}
	return &CorrectionResult{
		ActionID:   actionID,
		ActionType: actionType,
		TargetID:   targetID,
		Status:     model.CorrectionStatusFailed,
		Reason:     err.Error(),
		AppliedAt:  time.Now(),
	}
}

// extToMap 扩展字段转 map（用于审计 before/after）
func extToMap(ext *model.KnowledgeChunkExt) map[string]any {
	if ext == nil {
		return nil
	}
	return map[string]any{
		"quality_score":      ext.QualityScore,
		"quality_label":      string(ext.QualityLabel),
		"low_quality_hits":   ext.LowQualityHits,
		"champion_hits":      ext.ChampionHits,
		"source_session_ids": ext.SourceSessionIDs,
		"last_reward_at":     ext.LastRewardAt,
	}
}

// summarizeActions 汇总动作结果
func summarizeActions(actions []*CorrectionResult) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		out = append(out, map[string]any{
			"action_id":   a.ActionID,
			"action_type": string(a.ActionType),
			"target_id":   a.TargetID,
			"status":      string(a.Status),
			"reason":      a.Reason,
		})
	}
	return out
}

// parseUint64 字符串转 uint64
//
// 使用 fmt.Sscanf 宽松解析：允许 "12abc" → 12（兼容上游带后缀的 ID 格式）。
// 纯数字字符串行为与 strconv.ParseUint 一致。
func parseUint64(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// truncate 截断字符串（按 rune 截断，避免切断多字节 UTF-8 字符）
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// 按 rune 截断，确保不切断多字节字符
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes)
		if len(candidate) <= maxBytes {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return ""
}

// nowPtr 返回当前时间的指针
func nowPtr() *time.Time {
	now := time.Now()
	return &now
}
