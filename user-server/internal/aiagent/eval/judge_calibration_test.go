package eval

import "testing"

// D18: κ 计算与上岗门
func TestD18_CalibrateJudgePerfect(t *testing.T) {
	gold := []bool{true, true, false, false, true}
	got := CalibrateJudge(gold, gold)
	if got.Kappa != 1.0 || !got.Qualified && got.N >= MinCalibrationSamples {

		if got.Kappa != 1.0 {
			t.Errorf("完全一致 κ 应=1, got %v", got.Kappa)
		}
	}
}

// 完全随机/全反 → κ≤0
func TestD18_CalibrateJudgeDisagreement(t *testing.T) {
	gold := make([]bool, 40)
	judge := make([]bool, 40)
	for i := range gold {
		gold[i] = i%2 == 0
		judge[i] = i%2 != 0
	}
	got := CalibrateJudge(gold, judge)
	if got.Kappa > 0 {
		t.Errorf("完全相反 κ 应≤0, got %v", got.Kappa)
	}
	if got.Qualified {
		t.Error("完全相反不应上岗")
	}
}

// precision/recall 分开报
func TestD18_PrecisionRecallSeparate(t *testing.T) {

	gold := make([]bool, 40)
	judge := make([]bool, 40)
	for i := range gold {
		gold[i] = i < 20
		judge[i] = i < 25
	}
	got := CalibrateJudge(gold, judge)
	if got.Precision != 0.8 {
		t.Errorf("precision 应 0.8, got %v", got.Precision)
	}
	if got.Recall != 1.0 {
		t.Errorf("recall 应 1.0, got %v", got.Recall)
	}
	if got.Agreement != 0.875 {
		t.Errorf("agreement 应 0.875, got %v", got.Agreement)
	}
}

// 样本不足不上岗
func TestD18_MinSamples(t *testing.T) {
	gold := make([]bool, 10)
	got := CalibrateJudge(gold, gold)
	if got.Qualified {
		t.Error("N<30 不应上岗（即便 κ=1）")
	}
}
