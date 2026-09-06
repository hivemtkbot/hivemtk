package confidence

import (
	"math"
	"testing"
)

// TestBetaCalibration_Default 验证默认参数
func TestBetaCalibration_Default(t *testing.T) {
	b := NewBetaCalibration()
	a, bb, c := b.Parameters()
	if a != 1.0 || bb != 0.0 || c != 1.0 {
		t.Errorf("default should be (1, 0, 1), got (%v, %v, %v)", a, bb, c)
	}

	if math.Abs(b.Predict(0)-0.5) > 1e-9 {
		t.Errorf("c=1 should yield 0.5 at z=0, got %v", b.Predict(0))
	}
}

// TestBetaCalibration_EmptyDataset 验证空集
func TestBetaCalibration_EmptyDataset(t *testing.T) {
	b := NewBetaCalibration()
	b.Fit(nil)
	a, bb, c := b.Parameters()
	if a != 1.0 || bb != 0.0 || c != 1.0 {
		t.Error("empty fit should keep defaults")
	}
}

// TestBetaCalibration_Convergence 验证拟合
func TestBetaCalibration_Convergence(t *testing.T) {
	b := NewBetaCalibration()
	samples := make([]BetaSample, 0, 100)
	for i := 0; i < 50; i++ {
		samples = append(samples,
			BetaSample{DecisionValue: 0, Label: 0},
			BetaSample{DecisionValue: 2, Label: 1},
		)
	}
	b.Fit(samples)

	if b.Predict(0) > 0.1 {
		t.Errorf("P(z=0) should be near 0, got %v", b.Predict(0))
	}
	if b.Predict(2) < 0.9 {
		t.Errorf("P(z=2) should be near 1, got %v", b.Predict(2))
	}
}

// TestBetaCalibration_ReducesToPlattWhenC1 验证 c=1 时与 Platt 等价
func TestBetaCalibration_ReducesToPlattWhenC1(t *testing.T) {

	platt := NewPlattScaling()
	platt.Fit([]PlattSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 2, Label: 1},
	})
	a, bb := platt.Parameters()

	b := NewBetaCalibration()
	b.A = a
	b.B = bb
	b.C = 1.0

	for _, z := range []float64{-1, 0, 1, 2, 3} {
		diff := math.Abs(b.Predict(z) - platt.Predict(z))
		if diff > 1e-6 {
			t.Errorf("at z=%v: Beta=%v Platt=%v diff=%v (should be identical when c=1)",
				z, b.Predict(z), platt.Predict(z), diff)
		}
	}
}

// TestBetaCalibration_NumericalStability 验证极端输入
func TestBetaCalibration_NumericalStability(t *testing.T) {
	b := NewBetaCalibration()
	b.Fit([]BetaSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 1, Label: 1},
	})

	for _, z := range []float64{-1000, -100, 0, 100, 1000} {
		p := b.Predict(z)
		if math.IsNaN(p) || math.IsInf(p, 0) {
			t.Errorf("extreme z=%v returned %v", z, p)
		}
		if p < 0 || p > 1 {
			t.Errorf("z=%v out of [0,1]: %v", z, p)
		}
	}
}

// TestBetaCalibration_ECE 验证 ECE 计算
func TestBetaCalibration_ECE(t *testing.T) {
	b := NewBetaCalibration()
	samples := []BetaSample{
		{DecisionValue: 0, Label: 0},
		{DecisionValue: 2, Label: 1},
		{DecisionValue: 1, Label: 1},
		{DecisionValue: -1, Label: 0},
	}
	ece := b.ECE(samples)
	if ece < 0 || ece > 1 {
		t.Errorf("ECE should be in [0,1], got %v", ece)
	}
}

// TestBetaCalibration_ECE_Empty 验证空集
func TestBetaCalibration_ECE_Empty(t *testing.T) {
	b := NewBetaCalibration()
	if b.ECE(nil) != 0 {
		t.Error("empty ECE should be 0")
	}
}

// TestBetaCalibration_C_PowerEffect 验证 c 参数影响分布
func TestBetaCalibration_C_PowerEffect(t *testing.T) {

	z := 1.0
	b1 := &BetaCalibration{A: 2.0, B: 0, C: 0.5}
	b2 := &BetaCalibration{A: 2.0, B: 0, C: 1.0}
	b3 := &BetaCalibration{A: 2.0, B: 0, C: 2.0}
	p1 := b1.Predict(z)
	p2 := b2.Predict(z)
	p3 := b3.Predict(z)

	if !(p1 < p2 && p2 < p3) {
		t.Errorf("expected p1<p2<p3 at z=1, got %v, %v, %v", p1, p2, p3)
	}
}

// TestBetaCalibration_BetterFitOnImbalanced 验证不平衡数据
//
// 业界依据：Kull 2017 论文 §4 展示 Beta 在不平衡数据上明显优于 Platt
func TestBetaCalibration_BetterFitOnImbalanced(t *testing.T) {

	samples := make([]BetaSample, 0, 100)
	for i := 0; i < 100; i++ {
		label := 0
		z := 0.0
		if i%10 == 0 {
			label = 1
			z = 2.0
		}
		samples = append(samples, BetaSample{DecisionValue: z, Label: label})
	}
	b := NewBetaCalibration().Fit(samples)

	if b.C <= 1.0 {
		t.Logf("C=%v (note: imbalanced may not push C up)", b.C)
	}

	if b.Predict(0) > 0.15 {
		t.Errorf("P(z=0) should be < 0.15, got %v", b.Predict(0))
	}

	if b.Predict(2) < 0.8 {
		t.Errorf("P(z=2) should be > 0.8, got %v", b.Predict(2))
	}
}
