package confidence

// weighted_aggregator_test.go 5 维信号加权聚合器单元测试
//
// 覆盖：
//  1. 默认权重（0.30/0.15/0.15/0.20/0.20）
//  2. nil signals 返回 0
//  3. 全 1 信号返回 1
//  4. 全 0 信号返回 0
//  5. 加权计算正确性
//  6. 权重归一化
//  7. 热更新权重
//  8. Weights 只读访问

import (
	"testing"

	"marketing/internal/dto"
)

// TestDefaultSignalWeights 默认权重验证
func TestDefaultSignalWeights(t *testing.T) {
	w := DefaultSignalWeights()
	if !approxEqual(w.IntentConf, 0.30) {
		t.Errorf("IntentConf 默认权重应为 0.30, got %v", w.IntentConf)
	}
	if !approxEqual(w.EntityComp, 0.15) {
		t.Errorf("EntityComp 默认权重应为 0.15, got %v", w.EntityComp)
	}
	if !approxEqual(w.CtxRelev, 0.15) {
		t.Errorf("CtxRelev 默认权重应为 0.15, got %v", w.CtxRelev)
	}
	if !approxEqual(w.RAGQual, 0.20) {
		t.Errorf("RAGQual 默认权重应为 0.20, got %v", w.RAGQual)
	}
	if !approxEqual(w.LLMEntropy, 0.20) {
		t.Errorf("LLMEntropy 默认权重应为 0.20, got %v", w.LLMEntropy)
	}
	// 默认权重已归一化（和=1）
	sum := w.IntentConf + w.EntityComp + w.CtxRelev + w.RAGQual + w.LLMEntropy
	if !approxEqual(sum, 1.0) {
		t.Errorf("默认权重和应为 1.0, got %v", sum)
	}
}

// TestWeightedAggregator_DefaultWeights 通过聚合器验证默认权重
func TestWeightedAggregator_DefaultWeights(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	w := a.Weights()
	if !approxEqual(w.IntentConf, 0.30) {
		t.Errorf("IntentConf=%v want 0.30", w.IntentConf)
	}
	if !approxEqual(w.RAGQual, 0.20) {
		t.Errorf("RAGQual=%v want 0.20", w.RAGQual)
	}
}

// TestWeightedAggregator_Aggregate_NilSignals nil 返回 0
func TestWeightedAggregator_Aggregate_NilSignals(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	got := a.Aggregate(nil)
	if !approxEqual(got, 0.0) {
		t.Errorf("nil signals 应返回 0, got %v", got)
	}
}

// TestWeightedAggregator_Aggregate_AllOnes 全 1 信号返回 1
func TestWeightedAggregator_Aggregate_AllOnes(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 1.0, EntityComp: 1.0, CtxRelev: 1.0, RAGQual: 1.0, LLMEntropy: 1.0,
	}
	got := a.Aggregate(signals)
	if !approxEqual(got, 1.0) {
		t.Errorf("全 1 信号应返回 1.0, got %v", got)
	}
}

// TestWeightedAggregator_Aggregate_AllZeros 全 0 信号返回 0
func TestWeightedAggregator_Aggregate_AllZeros(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{}
	got := a.Aggregate(signals)
	if !approxEqual(got, 0.0) {
		t.Errorf("全 0 信号应返回 0, got %v", got)
	}
}

// TestWeightedAggregator_Aggregate_WeightedSum 加权计算正确性
// IntentConf=0.8, EntityComp=0.6, CtxRelev=0.7, RAGQual=0.5, LLMEntropy=0.9
// 期望：0.30*0.8 + 0.15*0.6 + 0.15*0.7 + 0.20*0.5 + 0.20*0.9
//
//	= 0.24 + 0.09 + 0.105 + 0.10 + 0.18 = 0.715
func TestWeightedAggregator_Aggregate_WeightedSum(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 0.8, EntityComp: 0.6, CtxRelev: 0.7, RAGQual: 0.5, LLMEntropy: 0.9,
	}
	got := a.Aggregate(signals)
	expected := 0.30*0.8 + 0.15*0.6 + 0.15*0.7 + 0.20*0.5 + 0.20*0.9
	if !approxEqual(got, expected) {
		t.Errorf("加权计算=%v want %v", got, expected)
	}
}

// TestWeightedAggregator_Aggregate_Clamp01 超 [0,1] 应被 clip
func TestWeightedAggregator_Aggregate_Clamp01(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	// 信号全 > 1，理论上会被 clip 到 1
	signals := &dto.FiveSignals{
		IntentConf: 1.5, EntityComp: 1.5, CtxRelev: 1.5, RAGQual: 1.5, LLMEntropy: 1.5,
	}
	got := a.Aggregate(signals)
	if !approxEqual(got, 1.0) {
		t.Errorf("超 1 信号应被 clip 到 1.0, got %v", got)
	}
}

// TestWeightedAggregator_NormalizeWeights 权重归一化
// 输入未归一化的权重，应自动归一化
func TestWeightedAggregator_NormalizeWeights(t *testing.T) {
	// 输入和=2，应归一化为和=1
	w := SignalWeights{
		IntentConf: 0.60, EntityComp: 0.30, CtxRelev: 0.30, RAGQual: 0.40, LLMEntropy: 0.40,
	}
	a := NewWeightedAggregator(w)
	got := a.Weights()
	sum := got.IntentConf + got.EntityComp + got.CtxRelev + got.RAGQual + got.LLMEntropy
	if !approxEqual(sum, 1.0) {
		t.Errorf("归一化后权重和应为 1.0, got %v", sum)
	}
	// 各权重应减半
	if !approxEqual(got.IntentConf, 0.30) {
		t.Errorf("归一化后 IntentConf=0.30, got %v", got.IntentConf)
	}
}

// TestWeightedAggregator_NormalizeWeights_ZeroSum 权重全 0 不应 panic
func TestWeightedAggregator_NormalizeWeights_ZeroSum(t *testing.T) {
	w := SignalWeights{} // 全 0
	a := NewWeightedAggregator(w)
	got := a.Weights()
	// 全 0 时 sum=0，不归一化，保持 0
	sum := got.IntentConf + got.EntityComp + got.CtxRelev + got.RAGQual + got.LLMEntropy
	if !approxEqual(sum, 0.0) {
		t.Errorf("全 0 权重应保持 0, got %v", sum)
	}
}

// TestWeightedAggregator_UpdateWeights 热更新权重
func TestWeightedAggregator_UpdateWeights(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	// 更新为 IntentConf=1.0 其他=0
	a.UpdateWeights(SignalWeights{IntentConf: 1.0})
	w := a.Weights()
	if !approxEqual(w.IntentConf, 1.0) {
		t.Errorf("更新后 IntentConf 应为 1.0, got %v", w.IntentConf)
	}
	if !approxEqual(w.EntityComp, 0.0) {
		t.Errorf("更新后 EntityComp 应为 0, got %v", w.EntityComp)
	}
	// 验证聚合行为
	signals := &dto.FiveSignals{IntentConf: 0.7, EntityComp: 1.0, CtxRelev: 1.0, RAGQual: 1.0, LLMEntropy: 1.0}
	got := a.Aggregate(signals)
	if !approxEqual(got, 0.7) {
		t.Errorf("IntentConf 权重 1.0 时聚合应=0.7, got %v", got)
	}
}

// TestWeightedAggregator_UpdateWeights_Normalize 热更新后自动归一化
func TestWeightedAggregator_UpdateWeights_Normalize(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	a.UpdateWeights(SignalWeights{
		IntentConf: 2.0, EntityComp: 1.0, CtxRelev: 1.0, RAGQual: 1.0, LLMEntropy: 1.0,
	}) // sum=6
	w := a.Weights()
	sum := w.IntentConf + w.EntityComp + w.CtxRelev + w.RAGQual + w.LLMEntropy
	if !approxEqual(sum, 1.0) {
		t.Errorf("热更新后应归一化到 1.0, got %v", sum)
	}
	if !approxEqual(w.IntentConf, 2.0/6.0) {
		t.Errorf("IntentConf 应=2/6 ≈ 0.333, got %v", w.IntentConf)
	}
}

// TestWeightedAggregator_Weights 返回当前权重（只读）
func TestWeightedAggregator_Weights(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	w1 := a.Weights()
	w1.IntentConf = 999 // 修改返回值不应影响内部状态
	w2 := a.Weights()
	if !approxEqual(w2.IntentConf, 0.30) {
		t.Errorf("外部修改 Weights 返回值不应影响内部状态, got %v", w2.IntentConf)
	}
}

// TestWeightedAggregator_Aggregate_PartialSignals 部分信号非 0
func TestWeightedAggregator_Aggregate_PartialSignals(t *testing.T) {
	a := NewDefaultWeightedAggregator()
	// 只有 IntentConf=0.9，其他全 0
	signals := &dto.FiveSignals{IntentConf: 0.9}
	got := a.Aggregate(signals)
	expected := 0.30 * 0.9 // 0.27
	if !approxEqual(got, expected) {
		t.Errorf("部分信号聚合=%v want %v", got, expected)
	}
}
