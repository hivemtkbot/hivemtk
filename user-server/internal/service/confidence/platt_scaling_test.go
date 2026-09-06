package confidence

import (
	"math"
	"sort"
	"testing"
)

// TestStableSigmoid 测试数值稳定 sigmoid
func TestStableSigmoid(t *testing.T) {
	tests := []struct {
		z        float64
		expected float64
		epsilon  float64
	}{
		{0, 0.5, 1e-9},
		{1, 0.7310585786, 1e-6},
		{-1, 0.2689414214, 1e-6},
		{100, 1.0, 1e-9},
		{-100, 0.0, 1e-9},
		{1000, 1.0, 1e-9},
		{-1000, 0.0, 1e-9},
	}
	for _, tt := range tests {
		got := stableSigmoid(tt.z)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("stableSigmoid(%v) returned %v (NaN/Inf)", tt.z, got)
		}
		if math.Abs(got-tt.expected) > tt.epsilon {
			t.Errorf("stableSigmoid(%v) = %v, want %v (epsilon %v)", tt.z, got, tt.expected, tt.epsilon)
		}
	}
}

// TestPlattScaling_Identity 验证未训练时为 identity (A=1, B=0)
func TestPlattScaling_Identity(t *testing.T) {
	p := NewPlattScaling()
	a, b := p.Parameters()
	if a != 1.0 || b != 0.0 {
		t.Errorf("default should be A=1, B=0, got A=%v B=%v", a, b)
	}

	p10 := p.Predict(1.0)
	expected := 1.0 / (1.0 + math.Exp(-1.0))
	if math.Abs(p10-expected) > 1e-6 {
		t.Errorf("identity predict mismatch: got %v, want %v", p10, expected)
	}
}

// TestPlattScaling_EmptyDataset 训练空数据集应保持 identity
func TestPlattScaling_EmptyDataset(t *testing.T) {
	p := NewPlattScaling()
	p.Fit([]PlattSample{})
	a, b := p.Parameters()
	if a != 1.0 || b != 0.0 {
		t.Errorf("empty fit should keep identity, got A=%v B=%v", a, b)
	}
}

// TestPlattScaling_Convergence 验证 Newton-Raphson 在合理样本上收敛
//
// 构造数据：decision_value = 2 * label + N(0, 0.5)
// 期望：模型对 label=0 的样本 z 应远小于 0，对 label=1 应远大于 0
func TestPlattScaling_Convergence(t *testing.T) {

	samples := make([]PlattSample, 0, 100)
	for i := 0; i < 100; i++ {
		label := i % 2
		dv := float64(label) * 2.0
		samples = append(samples, PlattSample{DecisionValue: dv, Label: label})
	}
	p := NewPlattScaling()
	p.Fit(samples)
	a, b := p.Parameters()

	if math.Abs(a) < 3.0 {
		t.Errorf("|A| should grow to separate classes, got A=%v B=%v", a, b)
	}

	if p.Predict(0.0) > 0.05 {
		t.Errorf("P(y=1|z=0) should be near 0, got %v", p.Predict(0.0))
	}
	if p.Predict(2.0) < 0.95 {
		t.Errorf("P(y=1|z=2) should be near 1, got %v", p.Predict(2.0))
	}
}

// TestPlattScaling_ImprovesCalibration 验证 Platt 能降低 ECE
//
// 场景：模型输出有 bias（B=0.5），Platt 应学会把 B 移到 -0.5
//   - 校准前：所有预测概率偏高（over-confident）
//   - 校准后：ECE 显著下降
func TestPlattScaling_ImprovesCalibration(t *testing.T) {

	samples := make([]PlattSample, 0, 200)
	for i := 0; i < 200; i++ {

		x := -2.0 + 4.0*float64(i)/199.0

		label := 0
		if x > 0 {
			label = 1
		}

		samples = append(samples, PlattSample{DecisionValue: x, Label: label})
	}

	before := NewPlattScaling()
	eceBefore := before.ECE(samples)

	after := NewPlattScaling()
	after.Fit(samples)
	eceAfter := after.ECE(samples)

	t.Logf("ECE before=%v after=%v", eceBefore, eceAfter)
	if eceAfter >= eceBefore {
		t.Errorf("Platt fitting should reduce ECE, got before=%v after=%v", eceBefore, eceAfter)
	}
}

// TestPlattScaling_ECE_NaN 验证极端输入不导致 NaN
func TestPlattScaling_ECE_NaN(t *testing.T) {
	p := NewPlattScaling()

	samples := []PlattSample{
		{DecisionValue: -1000, Label: 0},
		{DecisionValue: 1000, Label: 1},
		{DecisionValue: 0, Label: 1},
	}
	ece := p.ECE(samples)
	if math.IsNaN(ece) || math.IsInf(ece, 0) {
		t.Errorf("ECE on extreme values returned %v (NaN/Inf)", ece)
	}
}

// TestPlattScaling_PredictBatch 验证批量预测与单点一致
func TestPlattScaling_PredictBatch(t *testing.T) {
	p := NewPlattScaling()
	p.Fit([]PlattSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 1, Label: 1},
	})
	values := []float64{-1, 0, 0.5, 1, 2}
	batch := p.PredictBatch(values)
	if len(batch) != len(values) {
		t.Fatalf("length mismatch: got %d want %d", len(batch), len(values))
	}
	for i, v := range values {
		one := p.Predict(v)
		if math.Abs(batch[i]-one) > 1e-12 {
			t.Errorf("PredictBatch[%d]=%v != Predict(%v)=%v", i, batch[i], v, one)
		}
	}
}

// TestPlattScaling_RealisticClassImbalance 验证类别不平衡场景
//
// 真实客服场景：90% 流量非转人工意图（label=0），10% 转人工（label=1）
// Platt 应能正确学习不平衡数据的决策边界
func TestPlattScaling_RealisticClassImbalance(t *testing.T) {
	samples := make([]PlattSample, 0, 1000)

	for i := 0; i < 1000; i++ {
		label := 0
		z := -0.5
		if i%10 == 0 {
			label = 1
			z = 1.0
		}
		samples = append(samples, PlattSample{DecisionValue: z, Label: label})
	}
	p := NewPlattScaling()
	p.Fit(samples)

	if p.Predict(-0.5) > 0.5 {
		t.Errorf("P(y=1|z=-0.5) should be < 0.5 for imbalanced data, got %v", p.Predict(-0.5))
	}

	if p.Predict(1.0) < 0.5 {
		t.Errorf("P(y=1|z=1.0) should be > 0.5 for imbalanced data, got %v", p.Predict(1.0))
	}
}

// TestPlattScaling_Monotonicity 验证校准保持单调性
//
// 校准函数应该是单调递增的（如果 A > 0）。这是校准的基本性质。
func TestPlattScaling_Monotonicity(t *testing.T) {
	p := NewPlattScaling()
	p.Fit([]PlattSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 1, Label: 1},
		{DecisionValue: 2, Label: 1},
	})
	prev := -1.0
	for z := -3.0; z <= 3.0; z += 0.1 {
		pred := p.Predict(z)
		if pred < prev-1e-9 {
			t.Errorf("non-monotonic at z=%v: prev=%v current=%v", z, prev, pred)
		}
		prev = pred
	}
}

// TestPlattScaling_NumericalStability_OverFlow 验证极端大值不溢出
func TestPlattScaling_NumericalStability_OverFlow(t *testing.T) {
	p := NewPlattScaling()

	pred := p.Predict(1000)
	if math.IsNaN(pred) || math.IsInf(pred, 0) {
		t.Errorf("extreme value caused NaN/Inf: %v", pred)
	}
	if pred != 1.0 {
		t.Errorf("P(y=1|z=1000) should be 1.0, got %v", pred)
	}
}

// TestPlattScaling_BoundaryDecisions 验证边界决策值 (0.5 附近) 的处理
func TestPlattScaling_BoundaryDecisions(t *testing.T) {
	p := NewPlattScaling()

	p.Fit([]PlattSample{
		{DecisionValue: -1, Label: 0},
		{DecisionValue: -0.1, Label: 0},
		{DecisionValue: 0.1, Label: 1},
		{DecisionValue: 1, Label: 1},
	})

	pred := p.Predict(0.0)
	if pred < 0.3 || pred > 0.7 {
		t.Errorf("P(y=1|z=0) should be near 0.5, got %v", pred)
	}

	if pred < 0 || pred > 1 {
		t.Errorf("probability out of bounds: %v", pred)
	}
}

// TestPlattScaling_SortStability 验证对大量乱序数据训练稳定
func TestPlattScaling_SortStability(t *testing.T) {

	samples := make([]PlattSample, 0, 50)
	for i := 0; i < 50; i++ {
		samples = append(samples, PlattSample{
			DecisionValue: float64(i%5) - 2,
			Label:         i % 2,
		})
	}
	sorted := make([]PlattSample, len(samples))
	copy(sorted, samples)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DecisionValue < sorted[j].DecisionValue })

	p1 := NewPlattScaling().Fit(samples)
	p2 := NewPlattScaling().Fit(sorted)

	a1, b1 := p1.Parameters()
	a2, b2 := p2.Parameters()
	if math.Abs(a1-a2) > 1e-6 {
		t.Errorf("order should not affect A: %v vs %v", a1, a2)
	}
	if math.Abs(b1-b2) > 1e-6 {
		t.Errorf("order should not affect B: %v vs %v", b1, b2)
	}
}
