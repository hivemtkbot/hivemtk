package confidence


import "math"

// TemperatureScaler 温度缩放器
type TemperatureScaler struct {
	temperature float64
}

// NewTemperatureScaler 创建温度缩放器
//
// t <= 0 时使用 1.0（无缩放，避免除零）
func NewTemperatureScaler(t float64) *TemperatureScaler {
	if t <= 0 {
		t = 1.0
	}
	return &TemperatureScaler{temperature: t}
}

// Scale 对 logits 应用温度缩放，返回归一化概率分布
//
// 数值稳定：减去 max(z/T) 防止 exp 溢出
// 空 logits 返回 nil
func (ts *TemperatureScaler) Scale(logits []float64) []float64 {
	if len(logits) == 0 {
		return nil
	}
	scaled := make([]float64, len(logits))
	maxVal := math.Inf(-1)
	for _, z := range logits {
		scaledVal := z / ts.temperature
		if scaledVal > maxVal {
			maxVal = scaledVal
		}
	}
	sumExp := 0.0
	for i, z := range logits {
		scaled[i] = math.Exp(z/ts.temperature - maxVal)
		sumExp += scaled[i]
	}
	if sumExp == 0 {
		return scaled
	}
	for i := range scaled {
		scaled[i] /= sumExp
	}
	return scaled
}

// ScaleTop1 仅返回 top-1 概率（性能优化路径）
//
// 用于运行时在线校准（Calibrator.Calibrate 调用）
func (ts *TemperatureScaler) ScaleTop1(logits []float64) float64 {
	probs := ts.Scale(logits)
	if len(probs) == 0 {
		return 0
	}
	maxP := probs[0]
	for _, p := range probs[1:] {
		if p > maxP {
			maxP = p
		}
	}
	return maxP
}

// SetTemperature 更新温度（热重载，无需重启）
//
// 遵循项目规则 5：热重载平滑启动，不重启服务
func (ts *TemperatureScaler) SetTemperature(t float64) {
	if t > 0 {
		ts.temperature = t
	}
}

// Temperature 返回当前温度
func (ts *TemperatureScaler) Temperature() float64 {
	return ts.temperature
}

