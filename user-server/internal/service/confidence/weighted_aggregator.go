package confidence

// weighted_aggregator.go 5 维信号加权聚合器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.6
//
// 公式：conf = Σ w_i * signal_i（权重已归一化 Σw_i = 1）
// 默认权重：
//   - IntentConf:  0.30
//   - EntityComp:  0.15
//   - CtxRelev:    0.15
//   - RAGQual:     0.20
//   - LLMEntropy:  0.20

import (
	"marketing/internal/dto"
)

// SignalWeights 5 维信号权重
type SignalWeights struct {
	IntentConf float64 // 0.30
	EntityComp float64 // 0.15
	CtxRelev   float64 // 0.15
	RAGQual    float64 // 0.20
	LLMEntropy float64 // 0.20
}

// DefaultSignalWeights 默认权重（来自设计文档 §15.2.2）
func DefaultSignalWeights() SignalWeights {
	return SignalWeights{
		IntentConf: 0.30,
		EntityComp: 0.15,
		CtxRelev:   0.15,
		RAGQual:    0.20,
		LLMEntropy: 0.20,
	}
}

// WeightedAggregator 5 维信号加权聚合器
type WeightedAggregator struct {
	weights SignalWeights
}

// NewWeightedAggregator 创建聚合器
//
// 自动归一化权重，使 Σw_i = 1
func NewWeightedAggregator(w SignalWeights) *WeightedAggregator {
	normalizeWeights(&w)
	return &WeightedAggregator{weights: w}
}

// NewDefaultWeightedAggregator 使用默认权重创建聚合器
func NewDefaultWeightedAggregator() *WeightedAggregator {
	return NewWeightedAggregator(DefaultSignalWeights())
}

// Aggregate 加权聚合
//
// signals 为 nil 时返回 0
// 公式：conf = Σ w_i * signal_i
func (a *WeightedAggregator) Aggregate(signals *dto.FiveSignals) float64 {
	if signals == nil {
		return 0
	}
	conf := a.weights.IntentConf*signals.IntentConf +
		a.weights.EntityComp*signals.EntityComp +
		a.weights.CtxRelev*signals.CtxRelev +
		a.weights.RAGQual*signals.RAGQual +
		a.weights.LLMEntropy*signals.LLMEntropy
	return clamp01(conf)
}

// UpdateWeights 热更新权重（运营后台调参）
//
// 自动归一化
func (a *WeightedAggregator) UpdateWeights(w SignalWeights) {
	normalizeWeights(&w)
	a.weights = w
}

// Weights 返回当前权重（只读）
func (a *WeightedAggregator) Weights() SignalWeights {
	return a.weights
}

// normalizeWeights 归一化权重
func normalizeWeights(w *SignalWeights) {
	sum := w.IntentConf + w.EntityComp + w.CtxRelev + w.RAGQual + w.LLMEntropy
	if sum > 0 {
		w.IntentConf /= sum
		w.EntityComp /= sum
		w.CtxRelev /= sum
		w.RAGQual /= sum
		w.LLMEntropy /= sum
	}
}
