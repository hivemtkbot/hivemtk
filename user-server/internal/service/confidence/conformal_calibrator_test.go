package confidence

import (
	"math"
	"sync"
	"testing"
)

// TestConformalCalibrator_EmptyThreshold 验证空集返回 +Inf
func TestConformalCalibrator_EmptyThreshold(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	if !math.IsInf(cc.Threshold(), 1) {
		t.Errorf("empty should return +Inf, got %v", cc.Threshold())
	}
}

// TestConformalCalibrator_NilSafe 验证 nil 安全
func TestConformalCalibrator_NilSafe(t *testing.T) {
	var cc *ConformalCalibrator
	cc.AddScore(0.5)
	if cc.Threshold() != math.Inf(1) {
		t.Error("nil threshold should be +Inf")
	}
	cc.Recalibrate()
	snap := cc.Snapshot()
	if snap.SampleSize != 0 {
		t.Error("nil snapshot should be empty")
	}
}

// TestConformalCalibrator_AddScore 验证添加分数
func TestConformalCalibrator_AddScore(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	cc.AddScore(0.1)
	cc.AddScore(0.2)
	cc.AddScore(0.3)
	snap := cc.Snapshot()
	if snap.SampleSize != 3 {
		t.Errorf("expected 3 samples, got %d", snap.SampleSize)
	}
}

// TestConformalCalibrator_SlidingWindow 验证滑动窗口
func TestConformalCalibrator_SlidingWindow(t *testing.T) {
	cc := NewConformalCalibrator(5, 0) // 窗口 5
	for i := 0; i < 10; i++ {
		cc.AddScore(float64(i) / 10.0)
	}
	snap := cc.Snapshot()
	if snap.SampleSize != 5 {
		t.Errorf("expected 5 samples (sliding window), got %d", snap.SampleSize)
	}
}

// TestConformalCalibrator_Recalculate 验证重算
func TestConformalCalibrator_Recalculate(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	// 加 100 条递增 → 触发 Recalibrate
	for i := 0; i < 100; i++ {
		cc.AddScore(float64(i) / 100.0)
	}
	snap := cc.Snapshot()
	if snap.SampleSize != 100 {
		t.Errorf("expected 100 samples, got %d", snap.SampleSize)
	}
	// quantile 应在 0.9 附近（90 分位）
	if snap.Quantile < 0.85 || snap.Quantile > 0.95 {
		t.Errorf("quantile should be near 0.9, got %v", snap.Quantile)
	}
	if snap.CoverageRate != 0.9 {
		t.Errorf("coverage rate should be 0.9, got %v", snap.CoverageRate)
	}
}

// TestConformalCalibrator_Recalculate_Explicit 验证显式重算
func TestConformalCalibrator_Recalculate_Explicit(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	cc.AddScore(0.1)
	cc.AddScore(0.2)
	cc.AddScore(0.3)
	cc.Recalibrate()
	snap := cc.Snapshot()
	if snap.SampleSize != 3 {
		t.Errorf("expected 3 samples, got %d", snap.SampleSize)
	}
	if snap.Quantile <= 0 {
		t.Error("quantile should be > 0 after Recalibrate")
	}
}

// TestConformalCalibrator_Reset 验证重置
func TestConformalCalibrator_Reset(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	for i := 0; i < 10; i++ {
		cc.AddScore(float64(i) / 10.0)
	}
	cc.Reset()
	snap := cc.Snapshot()
	if snap.SampleSize != 0 {
		t.Errorf("expected 0 after reset, got %d", snap.SampleSize)
	}
}

// TestConformalCalibrator_NaNInf 验证 NaN/Inf 被拒绝
func TestConformalCalibrator_NaNInf(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	cc.AddScore(math.NaN())
	cc.AddScore(math.Inf(1))
	cc.AddScore(math.Inf(-1))
	cc.AddScore(0.5) // 唯一有效
	snap := cc.Snapshot()
	if snap.SampleSize != 1 {
		t.Errorf("only 0.5 should be accepted, got %d samples", snap.SampleSize)
	}
}

// TestConformalCalibrator_ConcurrentSafety 验证并发
func TestConformalCalibrator_ConcurrentSafety(t *testing.T) {
	cc := NewConformalCalibrator(1000, 0)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			cc.AddScore(float64(i%10) / 10.0)
		}(i)
		go func() {
			defer wg.Done()
			_ = cc.Threshold()
		}()
	}
	wg.Wait()
}

// TestConformalCalibrator_PredictorImmutability 验证 Predictor 返回的引用稳定性
func TestConformalCalibrator_PredictorImmutability(t *testing.T) {
	cc := NewConformalCalibrator(100, 0)
	cc.AddScore(0.5)
	cc.AddScore(0.7)
	p1 := cc.Predictor()
	cc.AddScore(0.9) // 不应触发 Recalibrate（只 1 条）
	cc.Recalibrate()  // 显式重算
	p2 := cc.Predictor()
	if p1 == p2 {
		t.Error("after Recalibrate, Predictor should return new instance")
	}
}
