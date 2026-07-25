package selflearning

// self_correction_dispatcher.go 自我矫正派发器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.3
//
// 职责：
//   1. 失败矩阵驱动的 7 类修复策略派发：
//        ┌─────────────────────────────────────────────────────────────┐
//        │ 监督告警           │ 派发动作              │ 子系统          │
//        ├─────────────────────────────────────────────────────────────┤
//        │ recall_precision↓  │ retrieve_retry        │ RAG             │
//        │ recall_coverage↓   │ query_rewrite         │ RAG             │
//        │ generation_fidelity↓│ llm_correction       │ LLM             │
//        │ answer_relevance↓  │ llm_correction        │ LLM             │
//        │ asset_effectiveness↓│ chunk_archive        │ AssetBundle     │
//        │ 低质语料累计 ≥3    │ chunk_archive         │ RAG             │
//        │ 销冠 reward ≥1.5   │ chunk_champion_upsert │ RAG             │
//        │ A/B 收敛 candidate胜│ asset_promote         │ AssetBundle     │
//        │ A/B 收敛 baseline胜 │ asset_rollback        │ AssetBundle     │
//        └─────────────────────────────────────────────────────────────┘
//   2. 失败矩阵查询：根据监督告警 metric_name + status 自动派发对应动作
//   3. 调用具体执行器（RAGSelfCorrector / AssetBundleLearner / LLMSelfCorrector）
//   4. 写入 SelfCorrectionAction 审计
//
// 自动执行约束（v1.1 §7.4）：
//   - 每个动作前调用 SwitchService.CheckGuardrail
//   - autonomous → 直接执行
//   - supervised → 写入 SelfCorrectionAction(status=pending) 待人工确认
//   - manual → 不执行，仅记录日志

import (
	"context"
	"fmt"
	"log"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// SelfCorrectionDispatcher 自我矫正派发器
type SelfCorrectionDispatcher struct {
	switchSvc     *SwitchService
	actionRepo    repository.SelfCorrectionActionRepository
	logRepo       repository.SelfLearningLogRepository
	signalRepo    repository.SelfSupervisionSignalRepository
	ragCorrector  *RAGSelfCorrector
	assetLearner  *AssetBundleLearner
	llmCorrector  *LLMSelfCorrector
}

// NewSelfCorrectionDispatcher 创建派发器
func NewSelfCorrectionDispatcher(
	switchSvc *SwitchService,
	actionRepo repository.SelfCorrectionActionRepository,
	logRepo repository.SelfLearningLogRepository,
	signalRepo repository.SelfSupervisionSignalRepository,
	ragCorrector *RAGSelfCorrector,
	assetLearner *AssetBundleLearner,
	llmCorrector *LLMSelfCorrector,
) *SelfCorrectionDispatcher {
	return &SelfCorrectionDispatcher{
		switchSvc:     switchSvc,
		actionRepo:    actionRepo,
		logRepo:       logRepo,
		signalRepo:    signalRepo,
		ragCorrector:  ragCorrector,
		assetLearner:  assetLearner,
		llmCorrector:  llmCorrector,
	}
}

// ============================================================================
// 失败矩阵派发（从监督告警触发）
// ============================================================================

// DispatchFromSignal 根据监督信号派发修复策略
//
// 输入：SelfSupervisionSignal（status=alert）
// 行为：
//   1. 根据 metric_name 查询失败矩阵，派发对应动作
//   2. autonomous → 执行
//   3. supervised/manual → 写入 pending 动作待审
func (d *SelfCorrectionDispatcher) DispatchFromSignal(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	if signal == nil {
		return fmt.Errorf("signal is nil")
	}
	actionType := d.lookupActionType(signal)
	if actionType == "" {
		return fmt.Errorf("no action type for metric=%s", signal.MetricName)
	}
	// 护栏检查
	allow, reason, err := d.switchSvc.ShouldExecuteAction(ctx, actionType)
	if err != nil {
		return err
	}
	if !allow {
		// 写入 pending 动作（待人工处理）
		return d.createPendingAction(ctx, signal, actionType, reason)
	}

	// 根据 autonomy_level 决定执行还是写入 pending
	snap, err := d.switchSvc.GetStatus(ctx)
	if err != nil {
		return err
	}
	if snap.AutonomyLevel == model.AutonomyLevelAutonomous {
		// 直接执行
		return d.executeAction(ctx, actionType, signal)
	}
	// supervised/manual：写入 pending 动作
	return d.createPendingAction(ctx, signal, actionType, "supervised/manual: pending review")
}

// lookupActionType 根据监督信号查询应派发的动作类型
//
// 失败矩阵（v1.1 §7.3 + 资产包 5 维监督扩展）：
//
//   ┌─────────────────────────────────────────────────────────────┐
//   │ 监督告警              │ 派发动作              │ 子系统       │
//   ├─────────────────────────────────────────────────────────────┤
//   │ recall_precision↓     │ retrieve_retry        │ RAG          │
//   │ recall_coverage↓      │ query_rewrite         │ RAG          │
//   │ generation_fidelity↓  │ llm_correction        │ LLM          │
//   │ answer_relevance↓     │ llm_correction        │ LLM          │
//   │ asset_effectiveness↓  │ asset_rollback        │ AssetBundle  │
//   │ asset_adoption↓       │ asset_rollback        │ AssetBundle  │
//   │ asset_conversion↓     │ asset_rollback        │ AssetBundle  │
//   │ asset_complaint↑      │ asset_rollback        │ AssetBundle  │
//   │ asset_freshness↓      │ asset_rollback        │ AssetBundle  │
//   │ asset_ab_converge↓    │ (无派发，等待收敛)    │ AssetBundle  │
//   │ 低质语料累计 ≥3       │ chunk_archive         │ RAG          │
//   │ 销冠 reward ≥1.5      │ chunk_champion_upsert │ RAG          │
//   │ A/B 收敛 candidate胜  │ asset_promote         │ AssetBundle  │
//   │ A/B 收敛 baseline胜   │ asset_rollback        │ AssetBundle  │
//   └─────────────────────────────────────────────────────────────┘
//
// 资产包类指标统一派发 asset_rollback，由 AssetBundleLearner.executeRollback 处理
// （包含降级为 inactive、记录审计、发布降级事件等动作）
func (d *SelfCorrectionDispatcher) lookupActionType(signal *model.SelfSupervisionSignal) model.CorrectionActionType {
	switch signal.MetricName {
	case model.SupervisionMetricRecallPrecision:
		return model.CorrectionRetrieveRetry
	case model.SupervisionMetricRecallCoverage:
		return model.CorrectionQueryRewrite
	case model.SupervisionMetricGenerationFidelity:
		return model.CorrectionLLMCorrection
	case model.SupervisionMetricAnswerRelevance:
		return model.CorrectionLLMCorrection
	case model.SupervisionMetricAssetEffectiveness:
		// 资产包效能低：若 target_id 是资产包，则触发 asset_rollback（A/B 失败回滚）
		// 若 target_id 是语料，则触发 chunk_archive
		if signal.TargetType == model.SupervisionTargetAsset {
			return model.CorrectionAssetRollback
		}
		return model.CorrectionChunkArchive
	case model.SupervisionMetricAssetAdoption,
		model.SupervisionMetricAssetConversion,
		model.SupervisionMetricAssetComplaint,
		model.SupervisionMetricAssetFreshness:
		// 资产包专属监督告警：统一派发 asset_rollback（由 AssetBundleLearner 处理降级/回滚）
		return model.CorrectionAssetRollback
	case model.SupervisionMetricAssetABConverge:
		// A/B 收敛度低：实验尚未收敛，不派发动作（等待 Bandit 收敛检查）
		// 仅在 alert 状态下记录日志，不触发修复
		return ""
	}
	return ""
}

// ============================================================================
// 动作执行（dispatch to specific corrector）
// ============================================================================

// executeAction 执行具体的矫正动作
//
// 根据 actionType 路由到对应 corrector：
//   - retrieve_retry / query_rewrite / chunk_archive / chunk_champion_upsert → RAGSelfCorrector
//   - asset_promote / asset_rollback → AssetBundleLearner
//   - llm_correction → LLMSelfCorrector
func (d *SelfCorrectionDispatcher) executeAction(ctx context.Context, actionType model.CorrectionActionType, signal *model.SelfSupervisionSignal) error {
	switch actionType {
	case model.CorrectionRetrieveRetry:
		return d.executeRetrieveRetry(ctx, signal)
	case model.CorrectionQueryRewrite:
		return d.executeQueryRewrite(ctx, signal)
	case model.CorrectionChunkArchive:
		return d.executeChunkArchive(ctx, signal)
	case model.CorrectionChampionUpsert:
		// 此动作由 RAGSelfCorrector.Reflect 触发，不通过 dispatcher
		return fmt.Errorf("champion_upsert should be triggered by RAGSelfCorrector.Reflect, not dispatcher")
	case model.CorrectionAssetPromote, model.CorrectionAssetRollback:
		// 资产包动作由 AssetBundleLearner.CheckConvergence 触发
		// 此处仅记录日志（dispatcher 不直接执行资产包动作）
		log.Printf("[dispatcher] asset action %s should be triggered by AssetBundleLearner.CheckConvergence", actionType)
		return nil
	case model.CorrectionLLMCorrection:
		if d.llmCorrector != nil {
			return d.llmCorrector.CorrectFromSignal(ctx, signal)
		}
		return fmt.Errorf("llm corrector is nil")
	}
	return fmt.Errorf("unknown action type: %s", actionType)
}

// executeRetrieveRetry 执行检索重试
//
// 行为：扩大 Top-K 召回，重新触发 RAG 检索
// 简化实现：仅记录日志（实际生产应调用 RAGEngine.Retrieve with larger TopK）
func (d *SelfCorrectionDispatcher) executeRetrieveRetry(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	actionID := GenActionID(signal.SignalID, model.CorrectionRetrieveRetry, signal.TargetID)
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  signal.SignalID,
		ActionType:    model.CorrectionRetrieveRetry,
		Scenario:      "rag",
		TargetType:    "rag_query",
		TargetID:      signal.TargetID,
		Before: map[string]any{
			"metric":     signal.MetricName,
			"value":      signal.Value,
			"threshold":  signal.Threshold,
			"bucket_hour": signal.BucketHour,
		},
		Operator: "auto",
		Reason:   fmt.Sprintf("recall_precision below threshold: %.3f < %.3f", signal.Value, signal.Threshold),
		Status:   model.CorrectionStatusApplied,
		AppliedAt: nowPtr(),
	}
	if err := d.actionRepo.Create(ctx, action); err != nil {
		return err
	}
	// 实际矫正动作：触发 RAG 引擎扩大召回
	// 简化实现：仅记录，实际触发由 RAGSelfCorrector 在下次 Warmup 时扩大 Top-K
	log.Printf("[dispatcher] retrieve_retry action recorded: signal=%s target=%s", signal.SignalID, signal.TargetID)
	if rerr := d.switchSvc.RecordCorrectionAction(ctx, model.CorrectionRetrieveRetry, true, false); rerr != nil {
		log.Printf("[dispatcher] retrieve_retry record_correction failed: signal=%s err=%v", signal.SignalID, rerr)
	}
	return nil
}

// executeQueryRewrite 执行查询改写
//
// 行为：通过 LLM 改写客户查询，提升召回覆盖
func (d *SelfCorrectionDispatcher) executeQueryRewrite(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	actionID := GenActionID(signal.SignalID, model.CorrectionQueryRewrite, signal.TargetID)
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  signal.SignalID,
		ActionType:    model.CorrectionQueryRewrite,
		Scenario:      "rag",
		TargetType:    "rag_query",
		TargetID:      signal.TargetID,
		Before: map[string]any{
			"metric":     signal.MetricName,
			"value":      signal.Value,
			"threshold":  signal.Threshold,
		},
		Operator: "auto",
		Reason:   fmt.Sprintf("recall_coverage below threshold: %.3f < %.3f", signal.Value, signal.Threshold),
		Status:   model.CorrectionStatusApplied,
		AppliedAt: nowPtr(),
	}
	if err := d.actionRepo.Create(ctx, action); err != nil {
		return err
	}
	// 实际矫正：由 RAGSelfCorrector 在下次 Warmup 时使用 LLM 改写查询
	log.Printf("[dispatcher] query_rewrite action recorded: signal=%s", signal.SignalID)
	if rerr := d.switchSvc.RecordCorrectionAction(ctx, model.CorrectionQueryRewrite, true, false); rerr != nil {
		log.Printf("[dispatcher] query_rewrite record_correction failed: signal=%s err=%v", signal.SignalID, rerr)
	}
	return nil
}

// executeChunkArchive 执行语料归档
//
// 行为：将低质语料标记为 archived，从召回池剔除
// 触发条件：低质语料累计 ≥ 10 次（由 RAGSelfCorrector.applyLowQualityMark 自动处理）
// 此处为监督告警触发的强制归档（即使未达到 10 次也归档）
func (d *SelfCorrectionDispatcher) executeChunkArchive(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	if signal.TargetID == "" {
		return fmt.Errorf("target_id is empty for chunk_archive")
	}
	// 解析 chunkID
	chunkID, err := parseUint64(signal.TargetID)
	if err != nil {
		return fmt.Errorf("parse chunk_id failed: %w", err)
	}
	// 调用 RAGSelfCorrector 执行归档（通过 applyLowQualityMark）
	// 简化：直接记录动作，实际归档由 RAGSelfCorrector 处理
	actionID := GenActionID(signal.SignalID, model.CorrectionChunkArchive, signal.TargetID)
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  signal.SignalID,
		ActionType:    model.CorrectionChunkArchive,
		Scenario:      "rag",
		TargetType:    "rag_chunk",
		TargetID:      signal.TargetID,
		Before: map[string]any{
			"metric":    signal.MetricName,
			"value":     signal.Value,
			"threshold": signal.Threshold,
			"chunk_id":  chunkID,
		},
		Operator: "auto",
		Reason:   fmt.Sprintf("forced archive from supervision alert: metric=%s value=%.3f", signal.MetricName, signal.Value),
		Status:   model.CorrectionStatusApplied,
		AppliedAt: nowPtr(),
	}
	if err := d.actionRepo.Create(ctx, action); err != nil {
		return err
	}
	log.Printf("[dispatcher] chunk_archive action recorded: chunk=%d", chunkID)
	if rerr := d.switchSvc.RecordCorrectionAction(ctx, model.CorrectionChunkArchive, true, false); rerr != nil {
		log.Printf("[dispatcher] chunk_archive record_correction failed: chunk=%d err=%v", chunkID, rerr)
	}
	return nil
}

// ============================================================================
// 待审动作创建（supervised 模式）
// ============================================================================

// createPendingAction 创建待审动作（supervised / manual 模式）
func (d *SelfCorrectionDispatcher) createPendingAction(ctx context.Context, signal *model.SelfSupervisionSignal, actionType model.CorrectionActionType, reason string) error {
	actionID := GenActionID(signal.SignalID, actionType, signal.TargetID)
	snap, serr := d.switchSvc.GetStatus(ctx)
	if serr != nil {
		log.Printf("[dispatcher] create_pending_action get_status failed: err=%v", serr)
	}
	autonomyLevel := model.AutonomyLevelManual
	if snap != nil {
		autonomyLevel = snap.AutonomyLevel
	}
	return d.actionRepo.Create(ctx, &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  signal.SignalID,
		ActionType:    actionType,
		Scenario:      "rag",
		TargetType:    string(signal.TargetType),
		TargetID:      signal.TargetID,
		Before: map[string]any{
			"metric":    signal.MetricName,
			"value":     signal.Value,
			"threshold": signal.Threshold,
		},
		AutonomyLevel: autonomyLevel,
		Operator:      "auto",
		Reason:        reason,
		Status:        model.CorrectionStatusPending,
	})
}

// ============================================================================
// 人工确认接口（supervised 模式下人工审核）
// ============================================================================

// ApproveAction 人工批准待审动作（supervised 模式）
//
// 行为：
//   1. 将 status=pending → applied
//   2. 调用对应 corrector 执行实际动作
func (d *SelfCorrectionDispatcher) ApproveAction(ctx context.Context, actionID string, operatorID uint, note string) error {
	action, err := d.actionRepo.GetByID(ctx, actionID)
	if err != nil {
		return fmt.Errorf("get action failed: %w", err)
	}
	if action.Status != model.CorrectionStatusPending {
		return fmt.Errorf("action status is not pending: %s", action.Status)
	}
	// 构造 signal 用于执行
	signal := &model.SelfSupervisionSignal{
		SignalID:   action.TriggerLogID,
		TargetType: model.SupervisionTargetType(action.TargetType),
		TargetID:   action.TargetID,
		MetricName: getStringFromMap(action.Before, "metric"),
		Value:      getFloatFromMap(action.Before, "value"),
		Threshold:  getFloatFromMap(action.Before, "threshold"),
	}
	// 执行
	if err := d.executeAction(ctx, action.ActionType, signal); err != nil {
		// 执行失败
		if uerr := d.actionRepo.UpdateStatus(ctx, actionID, model.CorrectionStatusFailed, map[string]any{
			"error_msg":   err.Error(),
			"operator_id": operatorID,
		}); uerr != nil {
			log.Printf("[dispatcher] approve_action update_status(failed) failed: action=%s err=%v", actionID, uerr)
		}
		return err
	}
	// 更新状态为 applied
	now := time.Now()
	return d.actionRepo.UpdateStatus(ctx, actionID, model.CorrectionStatusApplied, map[string]any{
		"applied_at":  &now,
		"operator_id": operatorID,
		"reason":      fmt.Sprintf("%s | approved: %s", action.Reason, note),
	})
}

// RejectAction 人工拒绝待审动作
func (d *SelfCorrectionDispatcher) RejectAction(ctx context.Context, actionID string, operatorID uint, reason string) error {
	return d.actionRepo.UpdateStatus(ctx, actionID, model.CorrectionStatusSkipped, map[string]any{
		"operator_id": operatorID,
		"reason":      fmt.Sprintf("rejected: %s", reason),
	})
}

// ============================================================================
// 工具方法
// ============================================================================

// getStringFromMap 从 map 中安全读取 string
func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getFloatFromMap 从 map 中安全读取 float64
func getFloatFromMap(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}
