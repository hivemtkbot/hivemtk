package selflearning

// rag_self_supervisor.go RAG 自我监督器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.2
//
// 职责：
//   1. 采集 RAG 5 维监督指标（v1.1 §7.2）：
//        - recall_precision    召回精度（LLM-as-Judge 判定召回是否相关）
//        - recall_coverage     召回覆盖（关键信息是否被召回）
//        - generation_fidelity 生成忠实度（生成内容是否基于召回）
//        - answer_relevance    答案相关性（答案是否回应客户问题）
//        - asset_effectiveness 资产包效能（资产包对回复质量的贡献）
//   2. 按小时分桶聚合到 self_supervision_signals 表
//   3. 超阈值自动告警（status=warning/alert）
//   4. 触发 SelfCorrectionDispatcher 派发修复策略
//
// LLM-as-Judge 采样：
//   - 为避免成本爆炸，按比例采样（默认 10%）
//   - 采样指标：generation_fidelity / answer_relevance
//   - 其余指标基于规则计算（如 recall_precision 基于反馈信号）

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"marketing/internal/dto"
	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// RAGSelfSupervisor RAG 自我监督器
type RAGSelfSupervisor struct {
	switchSvc     *SwitchService
	signalRepo    repository.SelfSupervisionSignalRepository
	logRepo       repository.SelfLearningLogRepository
	dispatcher    *SelfCorrectionDispatcher // 反向引用 dispatcher，触发修复
	llmDispatcher LLMDispatcher
	// LLM-as-Judge 采样比例（0.0-1.0，默认 0.1）
	// 使用 atomic.Uint64 存储float64的位表示，保证并发安全读写
	llmJudgeSampleRate atomic.Uint64
	// 指标阈值（从开关读取，初始化时取默认值）
	defaultThresholds map[string]float64
}

// NewRAGSelfSupervisor 创建 RAG 自我监督器
func NewRAGSelfSupervisor(
	switchSvc *SwitchService,
	signalRepo repository.SelfSupervisionSignalRepository,
	logRepo repository.SelfLearningLogRepository,
	dispatcher *SelfCorrectionDispatcher,
	llmDispatcher LLMDispatcher,
) *RAGSelfSupervisor {
	s := &RAGSelfSupervisor{
		switchSvc:     switchSvc,
		signalRepo:    signalRepo,
		logRepo:       logRepo,
		dispatcher:    dispatcher,
		llmDispatcher: llmDispatcher,
		defaultThresholds: map[string]float64{
			model.SupervisionMetricRecallPrecision:    0.8,  // 召回精度阈值
			model.SupervisionMetricRecallCoverage:     0.7,  // 召回覆盖阈值
			model.SupervisionMetricGenerationFidelity: 0.85, // 生成忠实度阈值
			model.SupervisionMetricAnswerRelevance:    0.8,  // 答案相关性阈值
			model.SupervisionMetricAssetEffectiveness: 0.6,  // 资产包效能阈值
		},
	}
	s.llmJudgeSampleRate.Store(math.Float64bits(0.1))
	return s
}

// SetLLMJudgeSampleRate 设置 LLM-as-Judge 采样比例
func (s *RAGSelfSupervisor) SetLLMJudgeSampleRate(rate float64) {
	if rate >= 0 && rate <= 1 {
		s.llmJudgeSampleRate.Store(math.Float64bits(rate))
	}
}

// GetLLMJudgeSampleRate 获取 LLM-as-Judge 采样比例
func (s *RAGSelfSupervisor) GetLLMJudgeSampleRate() float64 {
	return math.Float64frombits(s.llmJudgeSampleRate.Load())
}

// ============================================================================
// 5 维监督指标采集（dialogue.ended 触发）
// ============================================================================

// CollectMetrics 采集本次会话的监督指标（RAG 4 维 + 资产包效能共享维度）
//
// 触发：dialogue.ended 事件
// 输入：DialogueEndedPayload（包含 used_corpus_ids / used_asset_ids / last_customer_msg / last_ai_reply / aggregated_reward / outcome）
// 输出：4 个 RAG 信号（target_type=rag）+ N 个资产包信号（target_type=asset，N=len(used_asset_ids)）
//
// 采集策略（5 个指标维度，分属 RAG 与 Asset 两类 target_type）：
//
//	RAG 类（target_type=rag，受 EnableRAG 开关控制）：
//	  1. recall_precision    基于反馈信号计算（reward >= 0 视为相关）
//	  2. recall_coverage     基于客户最后消息的关键词在召回中的覆盖率
//	  3. generation_fidelity LLM-as-Judge（采样判定）
//	  4. answer_relevance    LLM-as-Judge（采样判定）
//	Asset 类（target_type=asset，受 EnableAsset 开关控制）：
//	  5. asset_effectiveness 基于资产包使用与转化的相关性（每 asset_id 一个信号，属共享指标）
func (s *RAGSelfSupervisor) CollectMetrics(ctx context.Context, payload *event.DialogueEndedPayload) error {
	if payload == nil {
		return fmt.Errorf("payload is nil")
	}
	snap, err := s.switchSvc.GetStatus(ctx)
	if err != nil {
		return err
	}
	if !snap.EnableRAG && !snap.EnableAsset {
		return nil
	}

	// 当前小时 bucket
	bucketHour := time.Now().Truncate(time.Hour)

	// 1. recall_precision（基于反馈）
	if snap.EnableRAG {
		recallPrecision := 0.5 // 默认值
		if payload.AggregatedReward > 0 {
			recallPrecision = 0.85
		} else if payload.AggregatedReward < 0 {
			recallPrecision = 0.4
		}
		threshold := s.getThreshold(model.SupervisionMetricRecallPrecision, snap)
		if err := s.upsertSignal(ctx, model.SupervisionTargetRAG, "", model.SupervisionMetricRecallPrecision, bucketHour, recallPrecision, threshold, 1, payload.TraceID); err != nil {
			log.Printf("[rag_supervisor] upsert recall_precision failed: %v", err)
		}
	}

	// 2. recall_coverage（基于关键词覆盖率）
	if snap.EnableRAG && len(payload.UsedCorpusIDs) > 0 {
		coverage := s.computeCoverage(payload.LastCustomerMsg, payload.LastAIReply)
		threshold := s.getThreshold(model.SupervisionMetricRecallCoverage, snap)
		if err := s.upsertSignal(ctx, model.SupervisionTargetRAG, "", model.SupervisionMetricRecallCoverage, bucketHour, coverage, threshold, 1, payload.TraceID); err != nil {
			log.Printf("[rag_supervisor] upsert recall_coverage failed: %v", err)
		}
	}

	// 3. generation_fidelity（LLM-as-Judge 采样）
	if snap.EnableRAG && s.shouldLLMJudge() && s.llmDispatcher != nil {
		fidelity, err := s.judgeGenerationFidelity(ctx, payload.LastCustomerMsg, payload.LastAIReply, payload.UsedCorpusIDs)
		if err == nil {
			threshold := s.getThreshold(model.SupervisionMetricGenerationFidelity, snap)
			if err := s.upsertSignal(ctx, model.SupervisionTargetRAG, "", model.SupervisionMetricGenerationFidelity, bucketHour, fidelity, threshold, 1, payload.TraceID); err != nil {
				log.Printf("[rag_supervisor] upsert generation_fidelity failed: %v", err)
			}
		}
	}

	// 4. answer_relevance（LLM-as-Judge 采样）
	if snap.EnableRAG && s.shouldLLMJudge() && s.llmDispatcher != nil {
		relevance, err := s.judgeAnswerRelevance(ctx, payload.LastCustomerMsg, payload.LastAIReply)
		if err == nil {
			threshold := s.getThreshold(model.SupervisionMetricAnswerRelevance, snap)
			if err := s.upsertSignal(ctx, model.SupervisionTargetRAG, "", model.SupervisionMetricAnswerRelevance, bucketHour, relevance, threshold, 1, payload.TraceID); err != nil {
				log.Printf("[rag_supervisor] upsert answer_relevance failed: %v", err)
			}
		}
	}

	// 5. asset_effectiveness（基于资产包使用 + 转化结果）
	if snap.EnableAsset && len(payload.UsedAssetIDs) > 0 {
		effectiveness := 0.5
		if payload.Outcome == "converted" {
			effectiveness = 0.85
		} else if payload.Outcome == "abandoned" {
			effectiveness = 0.35
		}
		for _, assetID := range payload.UsedAssetIDs {
			threshold := s.getThreshold(model.SupervisionMetricAssetEffectiveness, snap)
			if err := s.upsertSignal(ctx, model.SupervisionTargetAsset, assetID, model.SupervisionMetricAssetEffectiveness, bucketHour, effectiveness, threshold, 1, payload.TraceID); err != nil {
				log.Printf("[rag_supervisor] upsert asset_effectiveness failed: asset=%s err=%v", assetID, err)
			}
		}
	}
	return nil
}

// ============================================================================
// 监督告警扫描（cron.hourly 触发）
// ============================================================================

// ScanAlerts 扫描告警（每小时）
//
// 行为：
//  1. 查询最近 1h 的 warning/alert 信号
//  2. 对 alert 状态的信号，触发 SelfCorrectionDispatcher 派发修复策略
//  3. 写入 self_learning_logs（scenario=rag_supervision）
func (s *RAGSelfSupervisor) ScanAlerts(ctx context.Context) (int, error) {
	snap, err := s.switchSvc.GetStatus(ctx)
	if err != nil {
		return 0, err
	}
	if !snap.EnableRAG {
		return 0, nil
	}
	since := time.Now().Add(-1 * time.Hour)
	alerts, err := s.signalRepo.ListAlerts(ctx, since, 100)
	if err != nil {
		return 0, err
	}
	dispatchedCount := 0
	for _, alert := range alerts {
		// 仅 RAG/Asset 监督信号触发派发
		if alert.TargetType != model.SupervisionTargetRAG && alert.TargetType != model.SupervisionTargetAsset {
			continue
		}
		if alert.Status != model.SupervisionStatusAlert {
			continue
		}
		// 触发 dispatcher
		if s.dispatcher != nil {
			if err := s.dispatcher.DispatchFromSignal(ctx, alert); err != nil {
				log.Printf("[rag_supervisor] dispatch failed: signal=%s err=%v", alert.SignalID, err)
				continue
			}
			dispatchedCount++
		}
	}
	if dispatchedCount > 0 {
		log.Printf("[rag_supervisor] dispatched %d correction actions from alerts", dispatchedCount)
	}
	return dispatchedCount, nil
}

// ============================================================================
// 看板查询（供 Controller 调用）
// ============================================================================

// GetDashboard 获取监督看板
//
// range: "24h" / "7d" / "30d"
func (s *RAGSelfSupervisor) GetDashboard(ctx context.Context, rangeStr string) (*dto.SupervisionDashboardResponse, error) {
	var from time.Time
	now := time.Now()
	switch rangeStr {
	case "7d":
		from = now.Add(-7 * 24 * time.Hour)
	case "30d":
		from = now.Add(-30 * 24 * time.Hour)
	default:
		from = now.Add(-24 * time.Hour)
		rangeStr = "24h"
	}

	// RAG 4 维指标
	ragMetrics := []*dto.SupervisionMetricItem{}
	for _, metricName := range []string{
		model.SupervisionMetricRecallPrecision,
		model.SupervisionMetricRecallCoverage,
		model.SupervisionMetricGenerationFidelity,
		model.SupervisionMetricAnswerRelevance,
	} {
		avg, count, err := s.signalRepo.AggregateByRange(ctx, model.SupervisionTargetRAG, metricName, from, now)
		if err != nil {
			continue
		}
		ragMetrics = append(ragMetrics, &dto.SupervisionMetricItem{
			Name:         metricName,
			DisplayName:  metricDisplayName(metricName),
			Value:        avg,
			Threshold:    s.defaultThresholds[metricName],
			SampleCount:  count,
			LastSampleAt: now,
		})
	}

	// Asset 1 维指标（资产包效能按 target_id 分别展示）
	assetMetrics := []*dto.SupervisionMetricItem{}
	assetAvg, assetCount, aerr := s.signalRepo.AggregateByRange(ctx, model.SupervisionTargetAsset, model.SupervisionMetricAssetEffectiveness, from, now)
	if aerr != nil {
		log.Printf("[rag_supervisor] dashboard aggregate asset_effectiveness failed: err=%v", aerr)
	}
	assetMetrics = append(assetMetrics, &dto.SupervisionMetricItem{
		Name:         model.SupervisionMetricAssetEffectiveness,
		DisplayName:  metricDisplayName(model.SupervisionMetricAssetEffectiveness),
		Value:        assetAvg,
		Threshold:    s.defaultThresholds[model.SupervisionMetricAssetEffectiveness],
		SampleCount:  assetCount,
		LastSampleAt: now,
	})

	// 当前告警
	alerts, lerr := s.signalRepo.ListAlerts(ctx, from, 50)
	if lerr != nil {
		log.Printf("[rag_supervisor] dashboard list_alerts failed: err=%v", lerr)
	}
	alertItems := []*dto.SupervisionAlertItem{}
	for _, a := range alerts {
		severity := "warning"
		if a.Status == model.SupervisionStatusAlert {
			severity = "critical"
		}
		alertItems = append(alertItems, &dto.SupervisionAlertItem{
			AlertID:     a.SignalID,
			MetricName:  a.MetricName,
			Severity:    severity,
			Message:     fmt.Sprintf("metric=%s value=%.3f threshold=%.3f target=%s", a.MetricName, a.Value, a.Threshold, a.TargetID),
			TriggeredAt: a.BucketHour,
		})
	}

	return &dto.SupervisionDashboardResponse{
		Range:        rangeStr,
		From:         from,
		To:           now,
		RAGMetrics:   ragMetrics,
		AssetMetrics: assetMetrics,
		Alerts:       alertItems,
		UpdatedAt:    now,
	}, nil
}

// ============================================================================
// 内部方法
// ============================================================================

// upsertSignal 写入或更新监督信号
func (s *RAGSelfSupervisor) upsertSignal(ctx context.Context, targetType model.SupervisionTargetType, targetID, metricName string, bucket time.Time, value, threshold float64, sampleCount int, traceID string) error {
	signal := &model.SelfSupervisionSignal{
		SignalID:    GenSignalID(targetType, targetID, metricName, bucket),
		TargetType:  targetType,
		TargetID:    targetID,
		MetricName:  metricName,
		BucketHour:  bucket,
		Value:       value,
		Baseline:    0.5, // 默认基线
		Threshold:   threshold,
		SampleCount: int64(sampleCount),
		TraceIDs:    []string{traceID},
	}
	return s.signalRepo.UpsertSignal(ctx, signal)
}

// computeCoverage 基于关键词覆盖计算召回覆盖度
//
// 简化实现：客户消息中的关键词在 AI 回复中出现的比例
func (s *RAGSelfSupervisor) computeCoverage(customerMsg, aiReply string) float64 {
	if customerMsg == "" || aiReply == "" {
		return 0
	}
	// 简化：分词（按空格 + 标点），统计关键词在 AI 回复中出现比例
	words := strings.Fields(strings.ToLower(customerMsg))
	if len(words) == 0 {
		return 0
	}
	aiLower := strings.ToLower(aiReply)
	matched := 0
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		if strings.Contains(aiLower, w) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(words))
}

// shouldLLMJudge 是否触发 LLM-as-Judge（按采样比例）
func (s *RAGSelfSupervisor) shouldLLMJudge() bool {
	return rand.Float64() < s.GetLLMJudgeSampleRate()
}

// judgeGenerationFidelity LLM-as-Judge 评估生成忠实度
//
// Prompt 模板：给定客户消息 + AI 回复 + 召回语料 ID，判定 AI 回复是否基于召回语料
// 返回值：0.0-1.0（1.0 表示完全忠实）
func (s *RAGSelfSupervisor) judgeGenerationFidelity(ctx context.Context, customerMsg, aiReply string, usedCorpusIDs []string) (float64, error) {
	if s.llmDispatcher == nil {
		return 0.5, nil
	}
	prompt := fmt.Sprintf(`你是评估专家。请评估以下 AI 回复是否忠实于召回语料（不编造信息）。

客户消息：%s
AI 回复：%s
使用的语料 ID：%v

请输出 0-1 之间的分数（1.0=完全忠实，0.5=部分编造，0.0=完全编造）。仅输出数字。`,
		customerMsg, aiReply, usedCorpusIDs)
	content, _, err := s.llmDispatcher.Dispatch(ctx, "rag_supervision", prompt, "", false, 100)
	if err != nil {
		return 0.5, err
	}
	return parseScore(content)
}

// judgeAnswerRelevance LLM-as-Judge 评估答案相关性
func (s *RAGSelfSupervisor) judgeAnswerRelevance(ctx context.Context, customerMsg, aiReply string) (float64, error) {
	if s.llmDispatcher == nil {
		return 0.5, nil
	}
	prompt := fmt.Sprintf(`你是评估专家。请评估以下 AI 回复与客户问题的相关性。

客户消息：%s
AI 回复：%s

请输出 0-1 之间的分数（1.0=完全相关，0.5=部分相关，0.0=完全无关）。仅输出数字。`,
		customerMsg, aiReply)
	content, _, err := s.llmDispatcher.Dispatch(ctx, "rag_supervision", prompt, "", false, 100)
	if err != nil {
		return 0.5, err
	}
	return parseScore(content)
}

// parseScore 解析 LLM 返回的分数
func parseScore(content string) (float64, error) {
	content = strings.TrimSpace(content)
	score, err := strconv.ParseFloat(content, 64)
	if err != nil {
		return 0.5, fmt.Errorf("parse score failed: %w", err)
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, nil
}

// getThreshold 获取指标阈值
func (s *RAGSelfSupervisor) getThreshold(metricName string, snap *SwitchSnapshot) float64 {
	if v, ok := s.defaultThresholds[metricName]; ok {
		return v
	}
	return 0.7
}

// metricDisplayName 指标中文名
func metricDisplayName(metricName string) string {
	switch metricName {
	case model.SupervisionMetricRecallPrecision:
		return "召回精度"
	case model.SupervisionMetricRecallCoverage:
		return "召回覆盖"
	case model.SupervisionMetricGenerationFidelity:
		return "生成忠实度"
	case model.SupervisionMetricAnswerRelevance:
		return "答案相关性"
	case model.SupervisionMetricAssetEffectiveness:
		return "资产包效能"
	}
	return metricName
}
