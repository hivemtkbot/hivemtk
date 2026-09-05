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
//
// v3 审计 P0-#3 增强：集成 Platt（logistic 二分类校准）+ Conformal（覆盖率保证）。
//   - Platt：补全 Temperature 之外的二分类校准标准（Platt 1999）
//   - Conformal：在聚合决策上叠加 1-δ 覆盖率保证（Vovk 2005）
//   - 两者均为可选，nil 时跳过；保持向后兼容
type ConfidenceAggregator struct {
	collector  *SignalCollector
	calibrator *Calibrator
	platt      *PlattScaling
	conformal  *ConformalPredictor
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

// SetPlatt 注入 Platt 校准器（二分类置信度）
//
// 调用时机：训练完 Platt 校准后；亦可 nil 关闭
func (a *ConfidenceAggregator) SetPlatt(p *PlattScaling) {
	a.platt = p
}

// SetConformal 注入 Conformal 预测器（覆盖率保证）
//
// 调用时机：CalibrateOnline 收集到足够非一致性分数后；亦可 nil 关闭
func (a *ConfidenceAggregator) SetConformal(c *ConformalPredictor) {
	a.conformal = c
}

// GetPlatt 返回当前 Platt（用于运行时查询 / 持久化）
func (a *ConfidenceAggregator) GetPlatt() *PlattScaling {
	return a.platt
}

// GetConformal 返回当前 Conformal
func (a *ConfidenceAggregator) GetConformal() *ConformalPredictor {
	return a.conformal
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

	if a.platt != nil {
		plattCalibrated := a.platt.Predict(calibratedIntentConf)
		if plattCalibrated > 0 && plattCalibrated <= 1 {

			mixed := 0.5*calibratedIntentConf + 0.5*plattCalibrated
			if mixed > 0 {
				calibratedIntentConf = mixed
				signals.IntentConf = mixed
			}
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

	conformalUncertain := false
	if a.conformal != nil {

		nonConformity := 1.0 - aggregatedConf
		conformalThreshold := a.conformal.Quantile()
		if SelectivePredict(conformalThreshold, nonConformity) {
			conformalUncertain = true

			if !vetoTriggered && (band == dto.BandAuto || band == dto.BandReview) {
				band = dto.BandHandoff
				logger.Ctx(ctx).Info().
					Float64("non_conformity", nonConformity).
					Float64("conformal_threshold", conformalThreshold).
					Float64("conformal_coverage", a.conformal.CoverageGuarantee()).
					Msg("[Confidence] Conformal: high uncertainty, escalate to handoff")
			}
		}
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

	_ = conformalUncertain

	a.saveSignalAsync(ctx, in, decision, signals, calibratedIntentConf)
	return decision, nil
}

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

func inferCustomerLevel(in *dto.SignalCollectionInput) string {
	if in.CustomerLevel != "" {
		return in.CustomerLevel
	}
	return "normal"
}

func inferAgentAvailability(in *dto.SignalCollectionInput) float64 {
	if in.AgentAvailability > 0 {
		return in.AgentAvailability
	}
	return 0.5
}

// ErrAggregatorNotInitialized 聚合器未初始化
var ErrAggregatorNotInitialized = &aggError{"confidence aggregator not initialized"}

type aggError struct{ msg string }

func (e *aggError) Error() string { return e.msg }
