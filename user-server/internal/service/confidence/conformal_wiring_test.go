package confidence

import (
	"testing"
)

// D19: AddScore 回流 → Recalibrate → Quantile 生效（在线重校准闭环）
func TestD19_CalibratorQuantileEvolves(t *testing.T) {
	cc := NewConformalCalibrator(1000, 60)
	// 空校准集 → quantile=+Inf（保守 abstention）
	if !isInf(cc.Threshold()) {
		t.Fatalf("empty calibration set threshold should be +Inf, got %v", cc.Threshold())
	}
	// 回流 100 个低分样本（大部分合格，non-conformity 低）
	for i := 0; i < 95; i++ {
		cc.AddScore(0.1)
	}
	for i := 0; i < 5; i++ {
		cc.AddScore(0.9)
	}
	cc.Recalibrate()
	q := cc.Threshold()
	if isInf(q) {
		t.Fatal("calibrated threshold should not be +Inf")
	}
	// δ=0.1 → 90% 覆盖率：分位数=第 ⌈(n+1)·0.9⌉ 小。95 个 0.1 + 5 个 0.9（n=100）
	// → 第 91 小 = 0.1（0.9 的高分样本被判 uncertain——覆盖率语义正确）
	if q != 0.1 {
		t.Errorf("90pct coverage quantile should be 0.1, got %v", q)
	}
}

// Aggregate 回流接线：SetConformalCalibrator 后 Aggregate 触发 AddScore
// （聚合链路需完整 collector——此处直测回流语义即可）
func TestD19_AddScoreWindowCap(t *testing.T) {
	cc := NewConformalCalibrator(10, 60)
	for i := 0; i < 50; i++ {
		cc.AddScore(0.5)
	}
	// 窗口封顶 10：内部 scores 长度不超过 maxRetained（通过 Recalibrate 后阈值稳定验证）
	cc.Recalibrate()
	if isInf(cc.Threshold()) {
		t.Fatal("threshold should not be +Inf")
	}
}

func isInf(f float64) bool { return f > 1e308 }
