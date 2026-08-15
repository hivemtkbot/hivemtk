package confidence


import (
	"context"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
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

	signals, err := a.collector.Collect(ctx, in)
	if err != nil {
		return nil, err
	}

	calibratedIntentConf := signals.IntentConf
	if len(in.RawLogits) > 0 && a.calibrator != nil {
		calibrated := a.calibrator.Calibrate(in.RawLogits)
		if calibrated > 0 {
			calibratedIntentConf = calibrated
			signals.IntentConf = calibrated
		}
	}

	vetoCtx := &VetoContext{
		IntentType:        in.IntentType,
		CustomerMessage:   in.Text,
		LastNTurns:        in.LastTurns,
		ExpectedEntities:  in.ExpectedEntities,
		ExtractedEntities: in.ExtractedEntities,
	}
	vetoTriggered, vetoReason := a.vetoChain.Check(signals, vetoCtx)

	aggregatedConf := a.aggregator.Aggregate(signals)
	if vetoTriggered {
		aggregatedConf = 0 
	}

	threshold := a.calc.Calculate(&ThresholdInput{
		IntentType:        in.IntentType,
		CustomerLevel:     inferCustomerLevel(in),
		AgentAvailability: inferAgentAvailability(in),
		Now:               time.Now(),
	})

	policy := a.calc.policyEngine.GetPolicy(in.IntentType)
	band := a.calc.DetermineBand(aggregatedConf, threshold, policy)
	if vetoTriggered {
		band = dto.BandHandoff 
	}

	decision := &dto.ConfidenceDecision{
		SignalID:         uuid.New().String(),
		AggregatedConf:   aggregatedConf,
		DynamicThreshold: threshold,
		DecisionBand:     band,
		VetoTriggered:    vetoReason,
		Signals:          *signals,
		CalculatedAt:     time.Now(),
	}

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

