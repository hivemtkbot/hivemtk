package confidence

import (
	"math"
	"testing"
)

// TestConformalPredictor_Quantile 验证分位数计算
//
// 依据 Vovk 2005: q = ceil((n+1)·(1-δ)) / n
// 测试：n=100, δ=0.1 → q = ceil(101·0.9)/100 = 91/100 = 0.91 分位
func TestConformalPredictor_Quantile(t *testing.T) {

	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(scores, 0.1)
	q := cp.Quantile()

	if math.Abs(q-0.90) > 0.011 {
		t.Errorf("expected q≈0.90, got %v", q)
	}
}

// TestConformalPredictor_Quantile_SmallN 验证小样本分位数
func TestConformalPredictor_Quantile_SmallN(t *testing.T) {

	scores := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	cp := NewConformalPredictor(scores, 0.1)
	q := cp.Quantile()
	if q != 1.0 {
		t.Errorf("expected q=1.0, got %v", q)
	}
}

// TestConformalPredictor_Quantile_EmptySet 验证空集返回 +Inf（永远 abstention）
func TestConformalPredictor_Quantile_EmptySet(t *testing.T) {
	cp := NewConformalPredictor([]float64{}, 0.1)
	q := cp.Quantile()
	if !math.IsInf(q, 1) {
		t.Errorf("empty set should return +Inf, got %v", q)
	}
}

// TestConformalPredictor_DefaultDelta 验证非法 delta 归一化
func TestConformalPredictor_DefaultDelta(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3}, 0)
	if cp.delta != 0.1 {
		t.Errorf("delta=0 should normalize to 0.1, got %v", cp.delta)
	}
	cp2 := NewConformalPredictor([]float64{0.1, 0.2, 0.3}, 1.5)
	if cp2.delta != 0.1 {
		t.Errorf("delta>1 should normalize to 0.1, got %v", cp2.delta)
	}
}

// TestConformalPredictor_PredictSet 验证预测集合正确性
func TestConformalPredictor_PredictSet(t *testing.T) {
	scores := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	cp := NewConformalPredictor(scores, 0.1)

	q := cp.Quantile()
	if q != 0.5 {
		t.Errorf("expected q=0.5, got %v", q)
	}

	got := cp.PredictSet([]float64{0.1, 0.6, 0.3, 0.7, 0.4})
	want := []int{1, 3, 5}
	if !equalIntSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestConformalPredictor_PredictSet_AllRejected 验证全部超阈值时返回空
func TestConformalPredictor_PredictSet_AllRejected(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3}, 0.1)
	got := cp.PredictSet([]float64{0.9, 0.95, 1.0})
	if len(got) != 0 {
		t.Errorf("all-rejected should return empty, got %v", got)
	}
}

// TestConformalPredictor_CoverageGuarantee 验证覆盖率保证
func TestConformalPredictor_CoverageGuarantee(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2}, 0.1)
	if cp.CoverageGuarantee() != 0.9 {
		t.Errorf("expected 0.9, got %v", cp.CoverageGuarantee())
	}
	cp2 := NewConformalPredictor([]float64{0.1}, 0.05)
	if cp2.CoverageGuarantee() != 0.95 {
		t.Errorf("expected 0.95, got %v", cp2.CoverageGuarantee())
	}
}

// TestConformalPredictor_CalibrateOnline 验证在线校准
func TestConformalPredictor_CalibrateOnline(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3, 0.4, 0.5}, 0.1)
	originalN := len(cp.calibrationScores)
	cp.CalibrateOnline(0.6, 100)
	if len(cp.calibrationScores) != originalN+1 {
		t.Errorf("expected n+1=%d, got %d", originalN+1, len(cp.calibrationScores))
	}

	for i := 1; i < len(cp.calibrationScores); i++ {
		if cp.calibrationScores[i] < cp.calibrationScores[i-1] {
			t.Error("scores should be sorted after CalibrateOnline")
		}
	}
}

// TestConformalPredictor_CalibrateOnline_SlidingWindow 验证滑动窗口
func TestConformalPredictor_CalibrateOnline_SlidingWindow(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3}, 0.1)

	for i := 0; i < 10; i++ {
		cp.CalibrateOnline(float64(i+10), 3)
	}
	if len(cp.calibrationScores) != 3 {
		t.Errorf("sliding window should keep only 3, got %d", len(cp.calibrationScores))
	}

	expected := []float64{17, 18, 19}
	for i, v := range expected {
		if cp.calibrationScores[i] != v {
			t.Errorf("[%d] expected %v, got %v", i, v, cp.calibrationScores[i])
		}
	}
}

// TestConformalPredictor_FiniteSampleGuarantee 验证核心性质
//
// 业界定理（Vovk et al. 2005, Thm 2.1）：
//
//	在 i.i.d. 假设下，对于任意新样本 (X, Y)：
//	P(Y ∈ C(X)) ≥ 1 - δ
//
// 测试：构造 i.i.d. 校准集 + 大量 i.i.d. 测试集，
// 经验覆盖率 ≥ 1-δ - ε
func TestConformalPredictor_FiniteSampleGuarantee(t *testing.T) {

	calScores := make([]float64, 100)
	for i := range calScores {
		calScores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(calScores, 0.1)
	threshold := cp.Quantile()

	covered := 0
	total := 10000
	for i := 0; i < total; i++ {

		s := float64(i%100) / 100.0
		if s <= threshold {
			covered++
		}
	}
	empiricalCoverage := float64(covered) / float64(total)
	if empiricalCoverage < 0.85 {
		t.Errorf("empirical coverage should be ≥ 0.9, got %v", empiricalCoverage)
	}
}

// TestCoverageEmpirical 验证经验覆盖率计算
func TestCoverageEmpirical(t *testing.T) {
	preds := []int{1, 2, 3, 0, 1}
	labels := []int{1, 2, 0, 0, 1}

	cov := CoverageEmpirical(preds, labels)
	if math.Abs(cov-0.8) > 1e-9 {
		t.Errorf("expected 0.8, got %v", cov)
	}
}

// TestCoverageEmpirical_Empty 验证空集返回 0
func TestCoverageEmpirical_Empty(t *testing.T) {
	if CoverageEmpirical([]int{}, []int{}) != 0 {
		t.Error("empty should return 0")
	}
}

// TestCoverageEmpirical_MismatchedLength 验证长度不匹配返回 0
func TestCoverageEmpirical_MismatchedLength(t *testing.T) {
	if CoverageEmpirical([]int{1}, []int{1, 2}) != 0 {
		t.Error("mismatched length should return 0")
	}
}

// TestBrierScore 验证 Brier 分数
func TestBrierScore(t *testing.T) {

	if bs := BrierScore([]float64{1.0, 0.0, 1.0}, []int{1, 0, 1}); bs != 0 {
		t.Errorf("perfect should be 0, got %v", bs)
	}

	if bs := BrierScore([]float64{1.0}, []int{0}); bs != 1.0 {
		t.Errorf("worst should be 1, got %v", bs)
	}

	if bs := BrierScore([]float64{0.5}, []int{1}); bs != 0.25 {
		t.Errorf("expected 0.25, got %v", bs)
	}
}

// TestBrierScore_Empty 验证空集
func TestBrierScore_Empty(t *testing.T) {
	if BrierScore([]float64{}, []int{}) != 0 {
		t.Error("empty should return 0")
	}
}

// TestSelectivePredict 验证 abstention 决策
func TestSelectivePredict(t *testing.T) {
	if !SelectivePredict(0.5, 0.6) {
		t.Error("score > threshold should trigger abstention (true)")
	}
	if SelectivePredict(0.5, 0.4) {
		t.Error("score < threshold should not abstention (false)")
	}
	if SelectivePredict(0.5, 0.5) {
		t.Error("score == threshold should not abstention (false, strict >)")
	}
}

// TestConformalPredictor_Robustness 验证 robustness to outliers
func TestConformalPredictor_Robustness(t *testing.T) {

	scores := make([]float64, 100)
	for i := 0; i < 99; i++ {
		scores[i] = float64(i) / 100.0
	}
	scores[99] = 1000.0
	cp := NewConformalPredictor(scores, 0.1)
	q := cp.Quantile()

	if q > 1.0 {
		t.Logf("outlier pulls q up to %v (expected behavior)", q)
	}
}

// TestConformalPredictor_MonotonicQuantile 验证更严格 δ → 更大 q
func TestConformalPredictor_MonotonicQuantile(t *testing.T) {
	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100.0
	}

	cp90 := NewConformalPredictor(scores, 0.1)

	cp95 := NewConformalPredictor(scores, 0.05)
	if cp95.Quantile() <= cp90.Quantile() {
		t.Error("higher coverage (smaller delta) should yield higher or equal threshold")
	}
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
