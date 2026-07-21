package confidence

// calibrator_test.go 校准器单元测试
//
// 覆盖：
//  1. FitOnDataset 空样本集
//  2. FitOnDataset 完美校准
//  3. FitOnDataset 改善 ECE
//  4. FitOnDataset 改善 NLL
//  5. Calibrate 返回 top-1
//  6. SetTemperature 热重载
//  7. CurrentTemperature
//  8. LoadActiveFromDB nil repo
//  9. evaluate ECE/NLL 计算

import (
	"context"
	"math"
	"testing"
)

// TestCalibrator_FitOnDataset_Empty 空样本集返回错误
func TestCalibrator_FitOnDataset_Empty(t *testing.T) {
	c := NewCalibrator(nil)
	_, err := c.FitOnDataset(context.Background(), nil)
	if err != ErrEmptyCalibrationSet {
		t.Errorf("空样本集应返回 ErrEmptyCalibrationSet, got %v", err)
	}
}

// TestCalibrator_FitOnDataset_PerfectCalibration 完美校准
// 样本：logits=[10, -10], correct=0 → top-1 概率≈1，已校准
// 拟合后 ECE 应接近 0
func TestCalibrator_FitOnDataset_PerfectCalibration(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{10.0, -10.0}, CorrectIdx: 0},
		{Logits: []float64{10.0, -10.0}, CorrectIdx: 0},
		{Logits: []float64{10.0, -10.0}, CorrectIdx: 0},
		{Logits: []float64{-10.0, 10.0}, CorrectIdx: 1},
		{Logits: []float64{-10.0, 10.0}, CorrectIdx: 1},
		{Logits: []float64{-10.0, 10.0}, CorrectIdx: 1},
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("FitOnDataset 失败: %v", err)
	}
	// 完美校准：ECE_before ≈ 0
	if result.ECEBefore > 0.05 {
		t.Errorf("完美校准 ECE_before 应 < 0.05, got %v", result.ECEBefore)
	}
	if result.ECEAfter > 0.05 {
		t.Errorf("完美校准 ECE_after 应 < 0.05, got %v", result.ECEAfter)
	}
}

// TestCalibrator_FitOnDataset_ImprovesECE 校准后 ECE 应降低（或相等）
// 构造一个过度自信的样本集：所有预测都 80% 自信，但只有 50% 准确
func TestCalibrator_FitOnDataset_ImprovesECE(t *testing.T) {
	c := NewCalibrator(nil)
	// logits=[2, -2] → softmax ≈ [0.98, 0.02]，top-1 conf ≈ 0.98
	// 但准确率只有 50%（一半 correct=0，一半 correct=1 但 top-1=0 错误）
	// 这意味着 conf=0.98 但 acc=0.5 → ECE 较大
	// 校准后 T 应该 > 1，使 conf 降低
	samples := make([]CalibrationSample, 0, 40)
	for i := 0; i < 20; i++ {
		// 一半预测正确（correct=0，top-1=0）
		samples = append(samples, CalibrationSample{Logits: []float64{2.0, -2.0}, CorrectIdx: 0})
		// 一半预测错误（correct=1，top-1=0）
		samples = append(samples, CalibrationSample{Logits: []float64{2.0, -2.0}, CorrectIdx: 1})
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("FitOnDataset 失败: %v", err)
	}
	// 校准后 ECE 应 <= 校准前
	if result.ECEAfter > result.ECEBefore+1e-9 {
		t.Errorf("校准后 ECE (%v) 应 <= 校准前 (%v)", result.ECEAfter, result.ECEBefore)
	}
	// 温度应 > 1（需要软化）
	if result.Temperature < 1.0 {
		t.Errorf("过度自信样本应使 T > 1, got T=%v", result.Temperature)
	}
}

// TestCalibrator_FitOnDataset_ImprovesNLL 校准后 NLL 应降低（或相等）
func TestCalibrator_FitOnDataset_ImprovesNLL(t *testing.T) {
	c := NewCalibrator(nil)
	samples := make([]CalibrationSample, 0, 40)
	for i := 0; i < 20; i++ {
		samples = append(samples, CalibrationSample{Logits: []float64{2.0, -2.0}, CorrectIdx: 0})
		samples = append(samples, CalibrationSample{Logits: []float64{2.0, -2.0}, CorrectIdx: 1})
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("FitOnDataset 失败: %v", err)
	}
	if result.NLLAfter > result.NLLBefore+1e-9 {
		t.Errorf("校准后 NLL (%v) 应 <= 校准前 (%v)", result.NLLAfter, result.NLLBefore)
	}
}

// TestCalibrator_FitOnDataset_TemperatureRange 温度应在 [0.05, 5.0]
func TestCalibrator_FitOnDataset_TemperatureRange(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{1.0, 0.0}, CorrectIdx: 0},
		{Logits: []float64{0.0, 1.0}, CorrectIdx: 1},
		{Logits: []float64{1.0, 0.0}, CorrectIdx: 0},
		{Logits: []float64{0.0, 1.0}, CorrectIdx: 1},
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("FitOnDataset 失败: %v", err)
	}
	if result.Temperature < 0.05 || result.Temperature > 5.0 {
		t.Errorf("温度应在 [0.05, 5.0], got %v", result.Temperature)
	}
}

// TestCalibrator_FitOnDataset_SampleSize 样本数应正确记录
func TestCalibrator_FitOnDataset_SampleSize(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{1.0, 0.0}, CorrectIdx: 0},
		{Logits: []float64{0.0, 1.0}, CorrectIdx: 1},
		{Logits: []float64{1.0, 0.0}, CorrectIdx: 0},
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("FitOnDataset 失败: %v", err)
	}
	if result.SampleSize != 3 {
		t.Errorf("SampleSize=%d want 3", result.SampleSize)
	}
}

// TestCalibrator_Calibrate_ReturnsTop1 Calibrate 返回 top-1 概率
func TestCalibrator_Calibrate_ReturnsTop1(t *testing.T) {
	c := NewCalibrator(nil)
	// T=1, logits=[2,1,0] → top-1 = exp(2)/sum
	c.SetTemperature(1.0)
	logits := []float64{2.0, 1.0, 0.0}
	got := c.Calibrate(logits)
	// 手算：exp(2)/(exp(2)+exp(1)+exp(0)) = 7.389/11.107 ≈ 0.665
	expected := math.Exp(2.0) / (math.Exp(2.0) + math.Exp(1.0) + math.Exp(0.0))
	if !approxEqual(got, expected) {
		t.Errorf("Calibrate=%v want %v", got, expected)
	}
}

// TestCalibrator_Calibrate_EmptyLogits 空 logits 返回 0
func TestCalibrator_Calibrate_EmptyLogits(t *testing.T) {
	c := NewCalibrator(nil)
	if got := c.Calibrate(nil); got != 0 {
		t.Errorf("Calibrate(nil)=%v want 0", got)
	}
	if got := c.Calibrate([]float64{}); got != 0 {
		t.Errorf("Calibrate([])=%v want 0", got)
	}
}

// TestCalibrator_SetTemperature 手动设置温度
func TestCalibrator_SetTemperature(t *testing.T) {
	c := NewCalibrator(nil)
	c.SetTemperature(2.5)
	if !approxEqual(c.CurrentTemperature(), 2.5) {
		t.Errorf("SetTemperature(2.5) 后 CurrentTemperature=%v want 2.5", c.CurrentTemperature())
	}
}

// TestCalibrator_SetTemperature_AfterFit 拟合后再手动覆盖
func TestCalibrator_SetTemperature_AfterFit(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{1.0, 0.0}, CorrectIdx: 0},
		{Logits: []float64{0.0, 1.0}, CorrectIdx: 1},
	}
	result, _ := c.FitOnDataset(context.Background(), samples)
	// 拟合后温度 = result.Temperature
	if !approxEqual(c.CurrentTemperature(), result.Temperature) {
		t.Errorf("拟合后温度=%v want %v", c.CurrentTemperature(), result.Temperature)
	}
	// 手动覆盖
	c.SetTemperature(3.0)
	if !approxEqual(c.CurrentTemperature(), 3.0) {
		t.Errorf("SetTemperature(3.0) 后温度=%v want 3.0", c.CurrentTemperature())
	}
}

// TestCalibrator_CurrentTemperature 初始温度 = 1.0
func TestCalibrator_CurrentTemperature(t *testing.T) {
	c := NewCalibrator(nil)
	if !approxEqual(c.CurrentTemperature(), 1.0) {
		t.Errorf("初始温度应为 1.0, got %v", c.CurrentTemperature())
	}
}

// TestCalibrator_LoadActiveFromDB_NilRepo nil repo 直接返回 nil
func TestCalibrator_LoadActiveFromDB_NilRepo(t *testing.T) {
	c := NewCalibrator(nil)
	if err := c.LoadActiveFromDB(context.Background()); err != nil {
		t.Errorf("nil repo LoadActiveFromDB 应返回 nil, got %v", err)
	}
}

// TestCalibrator_Evaluate_ECE_Perfect 完美预测 ECE=0
func TestCalibrator_Evaluate_ECE_Perfect(t *testing.T) {
	c := NewCalibrator(nil)
	// 所有样本预测都 100% 准确
	samples := []CalibrationSample{
		{Logits: []float64{100.0, -100.0}, CorrectIdx: 0},
		{Logits: []float64{100.0, -100.0}, CorrectIdx: 0},
		{Logits: []float64{-100.0, 100.0}, CorrectIdx: 1},
	}
	ece, nll := c.evaluate(samples, 1.0)
	if ece > 0.01 {
		t.Errorf("完美预测 ECE 应 < 0.01, got %v", ece)
	}
	if nll > 0.01 {
		t.Errorf("完美预测 NLL 应 < 0.01, got %v", nll)
	}
}

// TestCalibrator_Evaluate_EmptySamples 空样本返回 (0, 0)
func TestCalibrator_Evaluate_EmptySamples(t *testing.T) {
	c := NewCalibrator(nil)
	ece, nll := c.evaluate(nil, 1.0)
	if !approxEqual(ece, 0.0) || !approxEqual(nll, 0.0) {
		t.Errorf("空样本应返回 (0, 0), got (%v, %v)", ece, nll)
	}
}

// TestCalibrator_Evaluate_ECE_Worst 最差校准 ECE 较大
// 所有预测都 98% 自信，但准确率只有 50%
func TestCalibrator_Evaluate_ECE_Worst(t *testing.T) {
	c := NewCalibrator(nil)
	samples := make([]CalibrationSample, 0, 40)
	for i := 0; i < 20; i++ {
		samples = append(samples, CalibrationSample{Logits: []float64{4.0, -4.0}, CorrectIdx: 0})
		samples = append(samples, CalibrationSample{Logits: []float64{4.0, -4.0}, CorrectIdx: 1}) // 预测错
	}
	ece, _ := c.evaluate(samples, 1.0)
	// conf≈0.98, acc=0.5, ECE ≈ |0.98-0.5| = 0.48
	if ece < 0.3 {
		t.Errorf("最差校准 ECE 应 > 0.3, got %v", ece)
	}
}

// TestCalibrator_FitOnDataset_DoesNotPanic 多分类样本
func TestCalibrator_FitOnDataset_DoesNotPanic(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{1.0, 2.0, 3.0, 0.5}, CorrectIdx: 2},
		{Logits: []float64{0.1, 0.2, 0.3, 0.4}, CorrectIdx: 3},
		{Logits: []float64{2.0, 1.0, 0.5, 0.1}, CorrectIdx: 0},
		{Logits: []float64{0.5, 2.5, 0.3, 0.1}, CorrectIdx: 1},
	}
	_, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Errorf("多分类样本不应报错: %v", err)
	}
}

// TestCalibrator_FitOnDataset_SingleClass 单一类样本（all correct=0）
func TestCalibrator_FitOnDataset_SingleClass(t *testing.T) {
	c := NewCalibrator(nil)
	samples := []CalibrationSample{
		{Logits: []float64{2.0, -1.0}, CorrectIdx: 0},
		{Logits: []float64{1.5, -0.5}, CorrectIdx: 0},
		{Logits: []float64{3.0, -2.0}, CorrectIdx: 0},
	}
	result, err := c.FitOnDataset(context.Background(), samples)
	if err != nil {
		t.Fatalf("单类样本不应报错: %v", err)
	}
	// 单类样本 NLL = -avg(log(p_0))，校准后应 <= 校准前
	if result.NLLAfter > result.NLLBefore+1e-9 {
		t.Errorf("单类样本 NLL_after (%v) 应 <= NLL_before (%v)", result.NLLAfter, result.NLLBefore)
	}
}
