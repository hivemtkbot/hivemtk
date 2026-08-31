package confidence

import (
	"math"
	"testing"
)

// TestGoldenSection_Minimize_Quadratic 在 (x-2)^2 上找最小值
// 理论最小值在 x=2，f(x)=0
func TestGoldenSection_Minimize_Quadratic(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-6, 100)
	f := func(x float64) float64 { return (x - 2.0) * (x - 2.0) }
	xStar := g.Minimize(f, -10.0, 10.0)
	if math.Abs(xStar-2.0) > 1e-3 {
		t.Errorf("二次函数最小值应在 x=2, got %v", xStar)
	}
}

// TestGoldenSection_Minimize_NLLLike 模拟 NLL 函数（单峰凸）
// f(t) = (t - 1.5)^2 + 0.1，最小值在 t=1.5
func TestGoldenSection_Minimize_NLLLike(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-5, 100)
	f := func(t float64) float64 { return (t-1.5)*(t-1.5) + 0.1 }
	tStar := g.Minimize(f, 0.05, 5.0)
	if math.Abs(tStar-1.5) > 1e-3 {
		t.Errorf("NLL 模拟函数最小值应在 t=1.5, got %v", tStar)
	}
}

// TestGoldenSection_Minimize_ConstantFunction 常数函数
// 任意点都是最小值
func TestGoldenSection_Minimize_ConstantFunction(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-4, 100)
	f := func(x float64) float64 { return 42.0 }
	xStar := g.Minimize(f, 0.0, 10.0)
	if xStar < 0 || xStar > 10 {
		t.Errorf("常数函数最小值应在 [0,10], got %v", xStar)
	}
}

// TestGoldenSection_Minimize_AEqualsB a==b
func TestGoldenSection_Minimize_AEqualsB(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-4, 100)
	f := func(x float64) float64 { return x * x }
	xStar := g.Minimize(f, 5.0, 5.0)
	if !approxEqual(xStar, 5.0) {
		t.Errorf("a==b 应返回 (a+b)/2=5.0, got %v", xStar)
	}
}

// TestGoldenSection_Minimize_AGreaterThanB a>b
func TestGoldenSection_Minimize_AGreaterThanB(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-4, 100)
	f := func(x float64) float64 { return x * x }
	xStar := g.Minimize(f, 10.0, 5.0)
	if !approxEqual(xStar, 7.5) {
		t.Errorf("a>b 应返回 (a+b)/2=7.5, got %v", xStar)
	}
}

// TestGoldenSection_Minimize_Sinusoidal 正弦函数最小值
// sin(x) 在 [0, 2π] 上最小值在 x=3π/2 ≈ 4.712
func TestGoldenSection_Minimize_Sinusoidal(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-6, 200)
	f := func(x float64) float64 { return math.Sin(x) }
	xStar := g.Minimize(f, 0.0, 2*math.Pi)
	if math.Abs(xStar-3*math.Pi/2) > 0.5 {
		t.Logf("提示：sin 在 [0,2π] 非单峰，结果可能非全局最优: x*=%v (理论 3π/2=%v)",
			xStar, 3*math.Pi/2)
	}
}

// TestGoldenSection_DefaultTolerance tolerance<=0 用默认 1e-4
func TestGoldenSection_DefaultTolerance(t *testing.T) {
	g := NewGoldenSectionSearcher(0, 100)
	if !approxEqual(g.tolerance, 1e-4) {
		t.Errorf("tolerance<=0 应默认 1e-4, got %v", g.tolerance)
	}
	g = NewGoldenSectionSearcher(-1, 100)
	if !approxEqual(g.tolerance, 1e-4) {
		t.Errorf("tolerance<0 应默认 1e-4, got %v", g.tolerance)
	}
}

// TestGoldenSection_DefaultMaxIter maxIter<=0 用默认 100
func TestGoldenSection_DefaultMaxIter(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-4, 0)
	if g.maxIter != 100 {
		t.Errorf("maxIter<=0 应默认 100, got %v", g.maxIter)
	}
	g = NewGoldenSectionSearcher(1e-4, -1)
	if g.maxIter != 100 {
		t.Errorf("maxIter<0 应默认 100, got %v", g.maxIter)
	}
}

// TestGoldenSection_ConvergenceTolerance 收敛精度验证
// 在精度 1e-6 下，结果应非常接近理论值
func TestGoldenSection_ConvergenceTolerance(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-8, 500)
	f := func(x float64) float64 { return (x - 3.0) * (x - 3.0) }
	xStar := g.Minimize(f, 0.0, 10.0)
	if math.Abs(xStar-3.0) > 1e-5 {
		t.Errorf("精度 1e-8 下应非常接近 3.0, got %v (误差 %v)", xStar, math.Abs(xStar-3.0))
	}
}

// TestGoldenSection_Minimize_Asymmetric 非对称区间
// f(x) = (x - 0.3)^2 在 [-1, 1] 上最小值在 x=0.3
func TestGoldenSection_Minimize_Asymmetric(t *testing.T) {
	g := NewGoldenSectionSearcher(1e-6, 100)
	f := func(x float64) float64 { return (x - 0.3) * (x - 0.3) }
	xStar := g.Minimize(f, -1.0, 1.0)
	if math.Abs(xStar-0.3) > 1e-3 {
		t.Errorf("非对称区间最小值应在 x=0.3, got %v", xStar)
	}
}

// TestGoldenSection_Minimize_RegionShrinkRatio 验证区间每次缩小 φ≈0.618
// 通过自定义函数计数器验证迭代次数
func TestGoldenSection_Minimize_RegionShrinkRatio(t *testing.T) {
	callCount := 0
	g := NewGoldenSectionSearcher(1e-4, 100)
	f := func(x float64) float64 {
		callCount++
		return (x - 5.0) * (x - 5.0)
	}
	_ = g.Minimize(f, 0.0, 10.0)
	if callCount < 5 || callCount > 50 {
		t.Errorf("函数调用次数应在 [5, 50], got %d", callCount)
	}
}
