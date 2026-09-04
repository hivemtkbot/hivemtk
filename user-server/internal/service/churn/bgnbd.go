// Package churn BG/NBD 客户流失概率模型（D22 v2 纯 Go 实现）。
//
// 参考文献：Fader, Hardie & Lee (2005) "Counting Your Customers the Easy Way"。
// 模型：购买过程 Gamma-Poisson（λ ~ Gamma(r, α)），流失过程 Beta-Geometric（p ~ Beta(a, b)）。
//
// 对拍契约：参数拟合与 P(alive) 需与 lifetimes 公开测试数据误差 <1e-4 才准入生产
// （对拍 CI 常驻）；对拍不达标则整体挂起、维持规则式流失——不引入 Python 为兜底底线。
package churn

import "math"

// Params BG/NBD 四参数
type Params struct {
	R     float64 // Gamma shape（购买强度先验）
	Alpha float64 // Gamma scale
	A     float64 // Beta a（流失概率先验）
	B     float64 // Beta b
}

// CustomerStats 客户统计量
// x：重复购买次数；tx：最近一次购买距首次购买时长；T：观察期时长（首购距今）
type CustomerStats struct {
	X  float64
	Tx float64
	T  float64
}

// hyp2F1 超几何函数 2F1(a,b;c;z) 级数展开（z<1 收敛；此处 z=t/(α+T+t)<1 恒成立）
func hyp2F1(a, b, c, z float64) float64 {
	const tol = 1e-12
	const maxIter = 10000
	term := 1.0
	sum := 1.0
	for i := 0; i < maxIter; i++ {
		term *= (a + float64(i)) * (b + float64(i)) / ((c + float64(i)) * (float64(i) + 1)) * z
		sum += term
		if math.Abs(term) < tol*math.Abs(sum) {
			break
		}
	}
	return sum
}

// logRatioATx log( (α+T)/(α+tx) )^(r+x) —— 数值稳定形态
func logRatioATx(p Params, x, tx, T float64) float64 {
	return (p.R + x) * (math.Log(p.Alpha+T) - math.Log(p.Alpha+tx))
}

// PAlive 条件存活概率 P(alive | x, tx, T)（Fader-Hardie 2005 式 4）
//
//	P(alive) = 1 / (1 + (a/(b+x-1)) * ((α+T)/(α+tx))^(r+x))
func PAlive(p Params, x, tx, T float64) float64 {
	if p.B+x-1 == 0 {
		return 0
	}
	ratio := (p.A / (p.B + x - 1)) * math.Exp(logRatioATx(p, x, tx, T))
	denom := 1 + ratio
	if math.IsNaN(denom) || math.IsInf(denom, 0) || denom <= 0 {
		return 0
	}
	return 1 / denom
}

// ConditionalExpectedPurchases 条件期望购买次数 E[Y(t)|x,tx,T]（Fader-Hardie 2005 式 10）
//
//	E[Y(t)] = [ (a+b+x-1)/(a-1) ] · [ 1 - ((α+T)/(α+T+t))^(r+x) · 2F1(r+x, b+x; a+b+x-1; t/(α+T+t)) ]
//	          ---------------------------------------------------------------
//	                    1 + (a/(b+x-1)) · ((α+T)/(α+tx))^(r+x)
func ConditionalExpectedPurchases(p Params, x, tx, T, t float64) float64 {
	if t <= 0 || p.A <= 1 {
		return 0
	}
	zt := t / (p.Alpha + T + t)
	numeratorInner := 1 -
		math.Pow((p.Alpha+T)/(p.Alpha+T+t), p.R+x)*
			hyp2F1(p.R+x, p.B+x, p.A+p.B+x-1, zt)
	numerator := ((p.A + p.B + x - 1) / (p.A - 1)) * numeratorInner
	denominator := 1 + (p.A/(p.B+x-1))*math.Exp(logRatioATx(p, x, tx, T))
	if denominator == 0 || math.IsNaN(denominator) {
		return 0
	}
	return numerator / denominator
}
