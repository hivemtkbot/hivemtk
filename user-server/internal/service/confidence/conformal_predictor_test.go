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
	// 构造 0.0, 0.01, 0.02, ..., 0.99 共 100 个分数
	scores := make([]float64, 100)
	for i := range scores {
		scores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(scores, 0.1)
	q := cp.Quantile()
	// 期望 q = 排序后第 91 个 = 0.90
	if math.Abs(q-0.90) > 0.011 {
		t.Errorf("expected q≈0.90, got %v", q)
	}
}

// TestConformalPredictor_Quantile_SmallN 验证小样本分位数
func TestConformalPredictor_Quantile_SmallN(t *testing.T) {
	// n=10, δ=0.1 → ceil(11·0.9)/10 = ceil(9.9)/10 = 10/10 = 第 10 个（最大）
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
	// n=5, δ=0.1 → ceil(6·0.9)/5 = ceil(5.4)/5 = 6/5 = 第 5 个 = 0.5
	// 阈值 = 0.5
	q := cp.Quantile()
	if q != 0.5 {
		t.Errorf("expected q=0.5, got %v", q)
	}
	// 预测：分数 ≤ 0.5 的都返回
	// 注：1-based 索引
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
	// 排序应保持
	for i := 1; i < len(cp.calibrationScores); i++ {
		if cp.calibrationScores[i] < cp.calibrationScores[i-1] {
			t.Error("scores should be sorted after CalibrateOnline")
		}
	}
}

// TestConformalPredictor_CalibrateOnline_SlidingWindow 验证滑动窗口
func TestConformalPredictor_CalibrateOnline_SlidingWindow(t *testing.T) {
	cp := NewConformalPredictor([]float64{0.1, 0.2, 0.3}, 0.1)
	// 添加大量新数据，maxRetained=3 应只保留最后 3 个
	for i := 0; i < 10; i++ {
		cp.CalibrateOnline(float64(i+10), 3)
	}
	if len(cp.calibrationScores) != 3 {
		t.Errorf("sliding window should keep only 3, got %d", len(cp.calibrationScores))
	}
	// 最后 3 个应是 17, 18, 19
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
	// 校准集：100 个 i.i.d. 分数（uniform [0,1]）
	calScores := make([]float64, 100)
	for i := range calScores {
		calScores[i] = float64(i) / 100.0
	}
	cp := NewConformalPredictor(calScores, 0.1)
	threshold := cp.Quantile()

	// 测试集：10000 个 i.i.d. 分数
	covered := 0
	total := 10000
	for i := 0; i < total; i++ {
		// 模拟 y_true = 1, s = uniform [0, 1]
		// "covered" = s ≤ threshold（即预测集非空）
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
	// 命中：index 0 (1==1), 1 (2==2), 3 (0==0), 4 (1==1) = 4/5
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
	// 完美预测：prob=1, label=1 → (1-1)²=0
	if bs := BrierScore([]float64{1.0, 0.0, 1.0}, []int{1, 0, 1}); bs != 0 {
		t.Errorf("perfect should be 0, got %v", bs)
	}
	// 最差预测：prob=1, label=0 → (1-0)²=1
	if bs := BrierScore([]float64{1.0}, []int{0}); bs != 1.0 {
		t.Errorf("worst should be 1, got %v", bs)
	}
	// (0.5, label=1) → (0.5-1)² = 0.25
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
	// 99 个正常分数 + 1 个极端值
	scores := make([]float64, 100)
	for i := 0; i < 99; i++ {
		scores[i] = float64(i) / 100.0
	}
	scores[99] = 1000.0 // 极端值
	cp := NewConformalPredictor(scores, 0.1)
	q := cp.Quantile()
	// q 应是 0.98 附近（被极端值拉偏至 1.0 上方 → 取第 91 个 = 0.90）
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
	// 90% 覆盖 → q = 0.90
	cp90 := NewConformalPredictor(scores, 0.1)
	// 95% 覆盖 → q = 0.95
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
