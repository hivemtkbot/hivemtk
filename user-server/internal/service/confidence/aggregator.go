package confidence

// aggregator.go 置信度总聚合器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.10
//
// 协调 SignalCollector / Calibrator / WeightedAggregator / VetoChain /
// ThresholdPolicyEngine / DynamicThresholdCalculator，输出最终 ConfidenceDecision
//
// 主流程：
//   1. 采集 5 维信号         (SignalCollector.Collect)
//   2. 温度缩放校准 IntentConf (Calibrator.Calibrate)
//   3. 一票否决检查           (VetoChain.Check)
//   4. 加权聚合               (WeightedAggregator.Aggregate)
//   5. 计算动态阈值           (DynamicThresholdCalculator.Calculate)
//   6. 决定决策区间           (DynamicThresholdCalculator.DetermineBand)
//   7. 构造决策               (ConfidenceDecision)
//   8. 异步持久化信号快照      (signalRepo.Create)

import (
	"context"
	"time"

	"github.com/google/uuid"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

// ConfidenceAggregator 置信度总聚合器
type ConfidenceAggregator struct {
	collector  *SignalCollector
	calibrator *Calibrator
	aggregator *WeightedAggregator
	vetoChain  *VetoChain
	calc       *DynamicThresholdCalculator
	signalRepo *repository.ConfidenceSignalRepository
}

// NewConfidenceAggregator 创建总聚合器
func NewConfidenceAggregator(
	collector *SignalCollector,
	calibrator *Calibrator,
	aggregator *WeightedAggregator,
	vetoChain *VetoChain,
	calc *DynamicThresholdCalculator,
	signalRepo *repository.ConfidenceSignalRepository,
) *ConfidenceAggregator {
	return &ConfidenceAggregator{
		collector:  collector,
		calibrator: calibrator,
		aggregator: aggregator,
		vetoChain:  vetoChain,
		calc:       calc,
		signalRepo: signalRepo,
	}
}

// Aggregate 主入口：采集信号 → 校准 → 聚合 → 否决 → 阈值 → 决策
//
// 输入：dto.SignalCollectionInput（包含 session/customer/message/intent/raw 信号/last turns）
// 输出：dto.ConfidenceDecision（包含 signal_id/aggregated_conf/dynamic_threshold/decision_band/veto_triggered/signals）
func (a *ConfidenceAggregator) Aggregate(ctx context.Context, in *dto.SignalCollectionInput) (*dto.ConfidenceDecision, error) {
	if a == nil {
		return nil, ErrAggregatorNotInitialized
	}

	// 1. 采集 5 维信号
	signals, err := a.collector.Collect(ctx, in)
	if err != nil {
		return nil, err
	}

	// 2. 温度缩放校准 IntentConf
	//    若有 RawLogits，用 Calibrator.Calibrate 替换 IntentConf
	calibratedIntentConf := signals.IntentConf
	if len(in.RawLogits) > 0 && a.calibrator != nil {
		calibrated := a.calibrator.Calibrate(in.RawLogits)
		if calibrated > 0 {
			calibratedIntentConf = calibrated
			signals.IntentConf = calibrated
		}
	}

	// 3. 一票否决检查
	vetoCtx := &VetoContext{
		IntentType:        in.IntentType,
		CustomerMessage:   in.Text,
		LastNTurns:        in.LastTurns,
		ExpectedEntities:  in.ExpectedEntities,
		ExtractedEntities: in.ExtractedEntities,
	}
	vetoTriggered, vetoReason := a.vetoChain.Check(signals, vetoCtx)

	// 4. 加权聚合
	aggregatedConf := a.aggregator.Aggregate(signals)
	if vetoTriggered {
		aggregatedConf = 0 // 一票否决强制置零
	}

	// 5. 计算动态阈值
	threshold := a.calc.Calculate(&ThresholdInput{
		IntentType:        in.IntentType,
		CustomerLevel:     inferCustomerLevel(in),
		AgentAvailability: inferAgentAvailability(in),
		Now:               time.Now(),
	})

	// 6. 决定决策区间
	policy := a.calc.policyEngine.GetPolicy(in.IntentType)
	band := a.calc.DetermineBand(aggregatedConf, threshold, policy)
	if vetoTriggered {
		band = dto.BandHandoff // 否决强制转人工
	}

	// 7. 构造决策
	decision := &dto.ConfidenceDecision{
		SignalID:         uuid.New().String(),
		AggregatedConf:   aggregatedConf,
		DynamicThreshold: threshold,
		DecisionBand:     band,
		VetoTriggered:    vetoReason,
		Signals:          *signals,
		CalculatedAt:     time.Now(),
	}

	// 8. 异步持久化信号快照（不阻塞主链路）
	a.saveSignalAsync(ctx, in, decision, signals, calibratedIntentConf)
	return decision, nil
}

// saveSignalAsync 异步保存信号快照（不阻塞主链路）
//
// 失败仅记录日志，不影响主流程
func (a *ConfidenceAggregator) saveSignalAsync(
	_ context.Context,
	in *dto.SignalCollectionInput,
	dec *dto.ConfidenceDecision,
	sig *dto.FiveSignals,
	calibratedIntentConf float64,
) {
	if a.signalRepo == nil {
		return
	}
	record := &model.ConfidenceSignal{
		SignalID:             dec.SignalID,
		SessionID:            in.SessionID,
		CustomerID:           in.CustomerID,
		MessageID:            in.MessageID,
		IntentType:           in.IntentType,
		IntentConf:           in.RawIntentConf,
		IntentConfCalibrated: calibratedIntentConf,
		EntityComp:           sig.EntityComp,
		CtxRelev:             sig.CtxRelev,
		RAGQual:              sig.RAGQual,
		LLMEntropy:           sig.LLMEntropy,
		AggregatedConf:       dec.AggregatedConf,
		VetoTriggered:        dec.VetoTriggered,
		DynamicThreshold:     dec.DynamicThreshold,
		DecisionBand:         dec.DecisionBand,
		Temperature:          1.0,
	}
	if a.calibrator != nil {
		record.Temperature = a.calibrator.CurrentTemperature()
	}
	// 异步写入（best-effort）
	// R6 修复：原 goroutine 无 recover、错误被静默吞噬。添加 recover + 错误日志。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("confidence_aggregator: async signalRepo.Create recovered from panic: %v", r)
			}
		}()
		if err := a.signalRepo.Create(context.Background(), record); err != nil {
			logger.Errorf("confidence_aggregator: async signalRepo.Create failed: %v", err)
		}
	}()
}

// inferCustomerLevel 推断客户等级
//
// 优先用 input.CustomerLevel（外部注入）
// 否则返回 "normal"
func inferCustomerLevel(in *dto.SignalCollectionInput) string {
	if in.CustomerLevel != "" {
		return in.CustomerLevel
	}
	return "normal"
}

// inferAgentAvailability 推断座席空闲比例
//
// 优先用 input.AgentAvailability（外部注入）
// 否则返回 0.5（中性值）
func inferAgentAvailability(in *dto.SignalCollectionInput) float64 {
	if in.AgentAvailability > 0 {
		return in.AgentAvailability
	}
	return 0.5
}

// ErrAggregatorNotInitialized 聚合器未初始化
var ErrAggregatorNotInitialized = &aggError{"confidence aggregator not initialized"}

// aggError 简单错误类型
type aggError struct{ msg string }

func (e *aggError) Error() string { return e.msg }
