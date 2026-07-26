package selflearning

// llm_self_corrector.go LLM 自我矫正器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.3
//
// 职责：
//   1. 幻觉矫正（hallucination_correction）：
//      - 检测 LLM 回复中的幻觉（编造信息、虚构数据）
//      - 通过 LLM-as-Critic 二次验证
//      - 自动改写为忠实于召回语料的回复
//   2. 跑题矫正（off_topic_correction）：
//      - 检测 LLM 回复与客户问题的相关性
//      - 自动重新生成聚焦客户问题的回复
//   3. 与监督信号联动：
//      - generation_fidelity < 阈值 → 触发幻觉矫正
//      - answer_relevance < 阈值 → 触发跑题矫正
//
// 实现：
//   - LangGraph Reflection 模式（生成者 + 批评者双身份迭代）
//   - 最大 2 次反思迭代（避免无限循环）

import (
	"context"
	"fmt"
	"log"
	"strings"

	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	// defaultMaxReflectionIterations 默认最大反思迭代次数（避免无限循环）
	defaultMaxReflectionIterations = 2
	// criticMaxTokens Critic 角色 LLM 调用的最大 token 数
	criticMaxTokens = 200
	// generatorMaxTokens Generator 角色 LLM 调用的最大 token 数
	generatorMaxTokens = 500
	// llmCorrectionScenario LLM 矫正场景标识
	llmCorrectionScenario = "llm_correction"
)

// LLMSelfCorrector LLM 自我矫正器
type LLMSelfCorrector struct {
	switchSvc     *SwitchService
	actionRepo    repository.SelfCorrectionActionRepository
	logRepo       repository.SelfLearningLogRepository
	llmDispatcher LLMDispatcher
	// 最大反思迭代次数（默认 2，避免无限循环）
	maxReflectionIterations int
}

// NewLLMSelfCorrector 创建 LLM 自我矫正器
func NewLLMSelfCorrector(
	switchSvc *SwitchService,
	actionRepo repository.SelfCorrectionActionRepository,
	logRepo repository.SelfLearningLogRepository,
	llmDispatcher LLMDispatcher,
) *LLMSelfCorrector {
	return &LLMSelfCorrector{
		switchSvc:               switchSvc,
		actionRepo:              actionRepo,
		logRepo:                 logRepo,
		llmDispatcher:           llmDispatcher,
		maxReflectionIterations: defaultMaxReflectionIterations,
	}
}

// ============================================================================
// 从监督告警触发矫正
// ============================================================================

// CorrectFromSignal 根据监督信号触发 LLM 自我矫正
//
// 触发：
//   - generation_fidelity < 阈值 → 幻觉矫正
//   - answer_relevance < 阈值 → 跑题矫正
func (l *LLMSelfCorrector) CorrectFromSignal(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	if signal == nil {
		return fmt.Errorf("signal is nil")
	}
	if l.llmDispatcher == nil {
		return fmt.Errorf("llm dispatcher is nil")
	}
	switch signal.MetricName {
	case model.SupervisionMetricGenerationFidelity:
		return l.correctHallucination(ctx, signal)
	case model.SupervisionMetricAnswerRelevance:
		return l.correctOffTopic(ctx, signal)
	}
	return fmt.Errorf("unsupported metric for llm correction: %s", signal.MetricName)
}

// ============================================================================
// 幻觉矫正（hallucination_correction）
// ============================================================================

// correctHallucination 幻觉矫正
//
// 流程（LangGraph Reflection 模式）：
//  1. 提取原始 LLM 回复（从 signal.Detail 或重新生成）
//  2. Critic 角色判定：是否包含幻觉？
//  3. 若有幻觉，Generator 角色重新生成
//  4. 最多迭代 maxReflectionIterations 次
//  5. 写入 SelfCorrectionAction 审计
func (l *LLMSelfCorrector) correctHallucination(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	// 提取原始回复
	originalReply := getStringFromMap(signal.Detail, "ai_reply")
	customerMsg := getStringFromMap(signal.Detail, "customer_msg")
	usedCorpusIDs := []string{}
	if v, ok := signal.Detail["used_corpus_ids"]; ok {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					usedCorpusIDs = append(usedCorpusIDs, s)
				}
			}
		}
	}
	if originalReply == "" {
		// 无法重放原始回复，仅记录告警
		return l.recordSkippedAction(ctx, signal, "original_reply is empty, cannot correct")
	}

	// 迭代反思
	correctedReply := originalReply
	var lastCritique string
	iterationsUsed := 0
	for i := 0; i < l.maxReflectionIterations; i++ {
		iterationsUsed++
		// Critic：判定是否有幻觉
		critique, err := l.criticHallucination(ctx, customerMsg, correctedReply, usedCorpusIDs)
		if err != nil {
			log.Printf("[llm_corrector] critic failed: %v", err)
			break
		}
		lastCritique = critique
		// 若无幻觉，结束迭代
		if !l.hasHallucination(critique) {
			break
		}
		// Generator：基于 critique 重新生成
		newReply, err := l.regenerateReply(ctx, customerMsg, correctedReply, critique, usedCorpusIDs)
		if err != nil {
			log.Printf("[llm_corrector] regenerate failed: %v", err)
			break
		}
		correctedReply = newReply
	}

	// 记录审计动作
	actionID := GenActionID(signal.SignalID, model.CorrectionLLMCorrection, signal.TargetID)
	snap, serr := l.switchSvc.GetStatus(ctx)
	if serr != nil {
		log.Printf("[llm_corrector] hallucination get_status failed: err=%v", serr)
	}
	autonomyLevel := model.AutonomyLevelManual
	if snap != nil {
		autonomyLevel = snap.AutonomyLevel
	}
	action := &model.SelfCorrectionAction{
		ActionID:     actionID,
		TriggerLogID: signal.SignalID,
		ActionType:   model.CorrectionLLMCorrection,
		Scenario:     "llm",
		TargetType:   "llm_reply",
		TargetID:     signal.TargetID,
		Before: map[string]any{
			"original_reply": originalReply,
			"customer_msg":   customerMsg,
			"metric":         signal.MetricName,
			"value":          signal.Value,
			"threshold":      signal.Threshold,
		},
		After: map[string]any{
			"corrected_reply": correctedReply,
			"iterations":      iterationsUsed,
			"final_critique":  lastCritique,
		},
		AutonomyLevel: autonomyLevel,
		Operator:      "auto",
		Reason:        fmt.Sprintf("hallucination correction: %d iterations", iterationsUsed),
		Status:        model.CorrectionStatusApplied,
		AppliedAt:     nowPtr(),
	}
	if err := l.actionRepo.Create(ctx, action); err != nil {
		return err
	}
	if rerr := l.switchSvc.RecordCorrectionAction(ctx, model.CorrectionLLMCorrection, true, false); rerr != nil {
		log.Printf("[llm_corrector] hallucination record_correction failed: signal=%s err=%v", signal.SignalID, rerr)
	}
	log.Printf("[llm_corrector] hallucination corrected: signal=%s iterations=%d", signal.SignalID, iterationsUsed)
	return nil
}

// ============================================================================
// 跑题矫正（off_topic_correction）
// ============================================================================

// correctOffTopic 跑题矫正
//
// 流程：
//  1. Critic 判定：AI 回复是否回应客户问题？
//  2. 若跑题，重新生成聚焦客户问题的回复
//  3. 迭代直至 critic 通过或达到最大迭代次数
func (l *LLMSelfCorrector) correctOffTopic(ctx context.Context, signal *model.SelfSupervisionSignal) error {
	originalReply := getStringFromMap(signal.Detail, "ai_reply")
	customerMsg := getStringFromMap(signal.Detail, "customer_msg")
	if originalReply == "" || customerMsg == "" {
		return l.recordSkippedAction(ctx, signal, "original_reply or customer_msg is empty")
	}

	correctedReply := originalReply
	var lastCritique string
	iterationsUsed := 0
	for i := 0; i < l.maxReflectionIterations; i++ {
		iterationsUsed++
		critique, err := l.criticOffTopic(ctx, customerMsg, correctedReply)
		if err != nil {
			break
		}
		lastCritique = critique
		if !l.isOffTopic(critique) {
			break
		}
		newReply, err := l.regenerateFocusedReply(ctx, customerMsg, correctedReply, critique)
		if err != nil {
			break
		}
		correctedReply = newReply
	}

	actionID := GenActionID(signal.SignalID, model.CorrectionLLMCorrection, signal.TargetID)
	snap, serr := l.switchSvc.GetStatus(ctx)
	if serr != nil {
		log.Printf("[llm_corrector] off_topic get_status failed: err=%v", serr)
	}
	autonomyLevel := model.AutonomyLevelManual
	if snap != nil {
		autonomyLevel = snap.AutonomyLevel
	}
	action := &model.SelfCorrectionAction{
		ActionID:     actionID,
		TriggerLogID: signal.SignalID,
		ActionType:   model.CorrectionLLMCorrection,
		Scenario:     "llm",
		TargetType:   "llm_reply",
		TargetID:     signal.TargetID,
		Before: map[string]any{
			"original_reply": originalReply,
			"customer_msg":   customerMsg,
			"metric":         signal.MetricName,
			"value":          signal.Value,
		},
		After: map[string]any{
			"corrected_reply": correctedReply,
			"iterations":      iterationsUsed,
			"final_critique":  lastCritique,
		},
		AutonomyLevel: autonomyLevel,
		Operator:      "auto",
		Reason:        fmt.Sprintf("off_topic correction: %d iterations", iterationsUsed),
		Status:        model.CorrectionStatusApplied,
		AppliedAt:     nowPtr(),
	}
	if err := l.actionRepo.Create(ctx, action); err != nil {
		return err
	}
	if rerr := l.switchSvc.RecordCorrectionAction(ctx, model.CorrectionLLMCorrection, true, false); rerr != nil {
		log.Printf("[llm_corrector] off_topic record_correction failed: signal=%s err=%v", signal.SignalID, rerr)
	}
	return nil
}

// ============================================================================
// LLM-as-Critic 实现
// ============================================================================

// criticHallucination 幻觉检测
//
// Prompt：判定 AI 回复中是否包含编造的信息
// 返回：JSON {"has_hallucination": bool, "evidence": "...", "severity": "high/medium/low"}
func (l *LLMSelfCorrector) criticHallucination(ctx context.Context, customerMsg, aiReply string, corpusIDs []string) (string, error) {
	prompt := fmt.Sprintf(`你是幻觉检测专家。请评估以下 AI 回复是否包含幻觉（编造信息、虚构数据、与召回语料不符的内容）。

客户消息：%s
AI 回复：%s
召回语料 ID：%v

请严格按以下 JSON 格式输出（不要其他内容）：
{"has_hallucination": true/false, "evidence": "幻觉内容或无幻觉说明", "severity": "high/medium/low"}`,
		customerMsg, aiReply, corpusIDs)
	content, _, err := l.llmDispatcher.Dispatch(ctx, llmCorrectionScenario, prompt, "", true, criticMaxTokens)
	if err != nil {
		return "", err
	}
	return content, nil
}

// criticOffTopic 跑题检测
func (l *LLMSelfCorrector) criticOffTopic(ctx context.Context, customerMsg, aiReply string) (string, error) {
	prompt := fmt.Sprintf(`你是回复相关性评估专家。请评估以下 AI 回复是否跑题（未回应客户问题）。

客户消息：%s
AI 回复：%s

请严格按以下 JSON 格式输出（不要其他内容）：
{"is_off_topic": true/false, "evidence": "跑题内容或无跑题说明", "severity": "high/medium/low"}`,
		customerMsg, aiReply)
	content, _, err := l.llmDispatcher.Dispatch(ctx, llmCorrectionScenario, prompt, "", true, criticMaxTokens)
	if err != nil {
		return "", err
	}
	return content, nil
}

// regenerateReply 重新生成回复（基于 critique）
func (l *LLMSelfCorrector) regenerateReply(ctx context.Context, customerMsg, prevReply, critique string, corpusIDs []string) (string, error) {
	prompt := fmt.Sprintf(`你是客服回复专家。基于以下反馈，重新生成回复（必须忠实于召回语料，不编造信息）。

客户消息：%s
原回复：%s
批评意见：%s
召回语料 ID：%v

请直接输出改进后的回复（不要其他内容）：`,
		customerMsg, prevReply, critique, corpusIDs)
	content, _, err := l.llmDispatcher.Dispatch(ctx, llmCorrectionScenario, prompt, "", false, generatorMaxTokens)
	if err != nil {
		return "", err
	}
	return content, nil
}

// regenerateFocusedReply 重新生成聚焦客户问题的回复
func (l *LLMSelfCorrector) regenerateFocusedReply(ctx context.Context, customerMsg, prevReply, critique string) (string, error) {
	prompt := fmt.Sprintf(`你是客服回复专家。基于以下反馈，重新生成回复（必须聚焦客户问题，不跑题）。

客户消息：%s
原回复：%s
批评意见：%s

请直接输出改进后的回复（不要其他内容）：`,
		customerMsg, prevReply, critique)
	content, _, err := l.llmDispatcher.Dispatch(ctx, llmCorrectionScenario, prompt, "", false, generatorMaxTokens)
	if err != nil {
		return "", err
	}
	return content, nil
}

// ============================================================================
// 工具方法
// ============================================================================

// hasHallucination 判定 critic 输出是否包含幻觉
func (l *LLMSelfCorrector) hasHallucination(critique string) bool {
	critique = strings.ToLower(critique)
	return strings.Contains(critique, `"has_hallucination": true`) ||
		strings.Contains(critique, `"has_hallucination":true`)
}

// isOffTopic 判定 critic 输出是否跑题
func (l *LLMSelfCorrector) isOffTopic(critique string) bool {
	critique = strings.ToLower(critique)
	return strings.Contains(critique, `"is_off_topic": true`) ||
		strings.Contains(critique, `"is_off_topic":true`)
}

// recordSkippedAction 记录跳过的动作
func (l *LLMSelfCorrector) recordSkippedAction(ctx context.Context, signal *model.SelfSupervisionSignal, reason string) error {
	actionID := GenActionID(signal.SignalID, model.CorrectionLLMCorrection, signal.TargetID)
	return l.actionRepo.Create(ctx, &model.SelfCorrectionAction{
		ActionID:     actionID,
		TriggerLogID: signal.SignalID,
		ActionType:   model.CorrectionLLMCorrection,
		Scenario:     "llm",
		TargetType:   "llm_reply",
		TargetID:     signal.TargetID,
		Operator:     "auto",
		Reason:       reason,
		Status:       model.CorrectionStatusSkipped,
	})
}
