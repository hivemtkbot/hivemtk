package confidence

// temperature_scaler_test.go 温度缩放器单元测试
//
// 覆盖：
//  1. T=1 等价于 softmax
//  2. T>1 软化分布（最大值降低）
//  3. T<1 锐化分布（最大值升高）
//  4. 空 logits 返回 nil
//  5. 数值稳定（极大值不溢出）
//  6. ScaleTop1 一致性
//  7. SetTemperature 热更新
//  8. 默认 T=1.0

import (
	"math"
	"testing"
)

// 接近相等（float64 比较）
func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestTemperatureScaler_DefaultT1(t *testing.T) {
	ts := NewTemperatureScaler(0) // 0 应被替换为 1.0
	if !approxEqual(ts.Temperature(), 1.0) {
		t.Errorf("默认温度应为 1.0, got %v", ts.Temperature())
	}
}

func TestTemperatureScaler_T1_EqualsSoftmax(t *testing.T) {
	// logits = [2.0, 1.0, 0.5]
	// softmax: exp(2)/sum, exp(1)/sum, exp(0.5)/sum
	ts := NewTemperatureScaler(1.0)
	logits := []float64{2.0, 1.0, 0.5}
	probs := ts.Scale(logits)

	// 期望值（手算）
	exp2 := math.Exp(2.0)
	exp1 := math.Exp(1.0)
	exp05 := math.Exp(0.5)
	sum := exp2 + exp1 + exp05
	expected := []float64{exp2 / sum, exp1 / sum, exp05 / sum}

	if len(probs) != 3 {
		t.Fatalf("len(probs)=%d want=3", len(probs))
	}
	for i, p := range probs {
		if !approxEqual(p, expected[i]) {
			t.Errorf("probs[%d]=%v want=%v", i, p, expected[i])
		}
	}
	// 概率和应为 1
	sumP := 0.0
	for _, p := range probs {
		sumP += p
	}
	if !approxEqual(sumP, 1.0) {
		t.Errorf("概率和=%v want=1.0", sumP)
	}
}

func TestTemperatureScaler_T2_SoftensDistribution(t *testing.T) {
	// T=2 应软化分布：top-1 概率降低
	logits := []float64{2.0, 1.0, 0.5}
	ts1 := NewTemperatureScaler(1.0)
	ts2 := NewTemperatureScaler(2.0)

	p1 := ts1.ScaleTop1(logits)
	p2 := ts2.ScaleTop1(logits)

	if p2 >= p1 {
		t.Errorf("T=2 应使 top-1 概率降低: p1=%v p2=%v", p1, p2)
	}
}

func TestTemperatureScaler_T05_SharpensDistribution(t *testing.T) {
	// T=0.5 应锐化分布：top-1 概率升高
	logits := []float64{2.0, 1.0, 0.5}
	ts1 := NewTemperatureScaler(1.0)
	ts05 := NewTemperatureScaler(0.5)

	p1 := ts1.ScaleTop1(logits)
	p05 := ts05.ScaleTop1(logits)

	if p05 <= p1 {
		t.Errorf("T=0.5 应使 top-1 概率升高: p1=%v p05=%v", p1, p05)
	}
}

func TestTemperatureScaler_EmptyLogits(t *testing.T) {
	ts := NewTemperatureScaler(1.0)
	if probs := ts.Scale(nil); probs != nil {
		t.Errorf("空 logits 应返回 nil, got %v", probs)
	}
	if probs := ts.Scale([]float64{}); probs != nil {
		t.Errorf("空 logits 应返回 nil, got %v", probs)
	}
	// ScaleTop1 空 logits 返回 0
	if p := ts.ScaleTop1(nil); p != 0 {
		t.Errorf("ScaleTop1(nil)=%v want=0", p)
	}
}

func TestTemperatureScaler_NumericalStability_LargeValues(t *testing.T) {
	// 极大值不应导致 NaN 或 Inf
	ts := NewTemperatureScaler(1.0)
	logits := []float64{1000.0, 999.0, 998.0}
	probs := ts.Scale(logits)
	for i, p := range probs {
		if math.IsNaN(p) || math.IsInf(p, 0) {
			t.Errorf("probs[%d] 不是有限数: %v", i, p)
		}
		if p < 0 || p > 1 {
			t.Errorf("probs[%d]=%v 超出 [0,1]", i, p)
		}
	}
}

func TestTemperatureScaler_ScaleTop1_Consistency(t *testing.T) {
	// ScaleTop1 应等于 Scale 结果中的最大值
	ts := NewTemperatureScaler(1.5)
	logits := []float64{1.0, 2.0, 3.0, 0.5}
	probs := ts.Scale(logits)
	top1 := ts.ScaleTop1(logits)

	maxP := probs[0]
	for _, p := range probs[1:] {
		if p > maxP {
			maxP = p
		}
	}
	if !approxEqual(top1, maxP) {
		t.Errorf("ScaleTop1=%v, Scale 中的 max=%v", top1, maxP)
	}
}

func TestTemperatureScaler_SetTemperature_HotReload(t *testing.T) {
	ts := NewTemperatureScaler(1.0)
	ts.SetTemperature(2.5)
	if !approxEqual(ts.Temperature(), 2.5) {
		t.Errorf("SetTemperature 后 Temperature=%v want=2.5", ts.Temperature())
	}
	// 0 不应改变温度
	ts.SetTemperature(0)
	if !approxEqual(ts.Temperature(), 2.5) {
		t.Errorf("SetTemperature(0) 不应改变温度: %v want=2.5", ts.Temperature())
	}
	// 负数不应改变温度
	ts.SetTemperature(-1)
	if !approxEqual(ts.Temperature(), 2.5) {
		t.Errorf("SetTemperature(-1) 不应改变温度: %v want=2.5", ts.Temperature())
	}
}
