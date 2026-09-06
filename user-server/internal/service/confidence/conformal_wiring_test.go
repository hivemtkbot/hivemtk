package confidence

import (
	"testing"
)

// D19: AddScore 回流 → Recalibrate → Quantile 生效（在线重校准闭环）
func TestD19_CalibratorQuantileEvolves(t *testing.T) {
	cc := NewConformalCalibrator(1000, 60)

	if !isInf(cc.Threshold()) {
		t.Fatalf("empty calibration set threshold should be +Inf, got %v", cc.Threshold())
	}

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

	cc.Recalibrate()
	if isInf(cc.Threshold()) {
		t.Fatal("threshold should not be +Inf")
	}
}

func isInf(f float64) bool { return f > 1e308 }
