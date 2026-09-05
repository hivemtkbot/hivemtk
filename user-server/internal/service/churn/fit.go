package churn

import (
	"math"

	"gonum.org/v1/gonum/optimize"
)

// FitInput 批量拟合输入（客户级统计量集合）
type FitInput struct {
	Stats []CustomerStats
	// 初始点（可零值=默认 (1,1,1,1)）
	Init Params
}

// FitResult 拟合结果
type FitResult struct {
	Params    Params
	LogLik    float64
	Converged bool
}

// logLikelihood BG/NBD 对数似然（Fader-Hardie 2005 附录 A；lifetimes 官方同款逐项展开）
//
//	A_1 = ln Γ(r+x)/Γ(r) + r·ln α
//	A_2 = ln Γ(a+b) + ln Γ(b+x) − ln Γ(b) − ln Γ(a+b+x)
//	A_3 = −(r+x)·ln(α+T)
//	A_4 = ln a − ln(b+max(x,1)−1) − (r+x)·ln(α+tx)
//	LL  = A_1 + A_2 + logsumexp(A_3, A_4·δ(x>0))
//
// 数值要点（CDNOW 对拍得出的教训）：A_4 必须用 log a − log(b+x−1) 的显式差形式，
// 不能展开成 lgamma(a+1)+lgamma(b+x−1) 组合——后者在优化器探边时（b 大 x 大）落进
// 浮点坍缩带，似然面出现假下降谷，Nelder-Mead 会滑向 a→0,b→∞ 的伪解。
func (p Params) logLikelihood(stats []CustomerStats) float64 {
	total := 0.0
	lgR := lgamma(p.R)
	lgB := lgamma(p.B)
	lgAB := lgamma(p.A + p.B)
	logA := math.Log(p.A)
	for _, s := range stats {
		A1 := lgamma(p.R+s.X) - lgR + p.R*math.Log(p.Alpha)
		A2 := lgAB + lgamma(p.B+s.X) - lgB - lgamma(p.A+p.B+s.X)
		A3 := -(p.R + s.X) * math.Log(p.Alpha+s.T)
		bxm1 := p.B + math.Max(s.X, 1) - 1
		A4 := logA - math.Log(bxm1) - (p.R+s.X)*math.Log(p.Alpha+s.Tx)
		m := math.Max(A3, A4)
		term := A1 + A2 + m + math.Log(math.Exp(A3-m)+math.Exp(A4-m)*b2f(s.X > 0))
		total += term
	}
	return total
}

func b2f(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// Fit MLE 拟合四参数。
//
// 实现：log-param 变换（四参数恒正，消除边界约束）+ Nelder-Mead 单纯形。
// 与 lifetimes 官方一致（scipy minimize method='Nelder-Mead'，初始 log(1,1,1,1)=0）。
// a 无下界约束（论文模型 a∈(0,∞)，CDNOW 全局最优 a≈0.79<1——旧版强加 a>1.001
// 是错误约束，会把全局最优排除在可行域外）。
func Fit(in FitInput) FitResult {
	if len(in.Stats) == 0 {
		return FitResult{}
	}
	init := []float64{1, 1, 1, 1} // log 空间 (r,α,a,b)=(1,1,1,1)，与 lifetimes 默认一致
	if in.Init.R > 0 && in.Init.Alpha > 0 && in.Init.A > 0 && in.Init.B > 0 {
		init = []float64{
			math.Log(in.Init.R), math.Log(in.Init.Alpha),
			math.Log(in.Init.A), math.Log(in.Init.B),
		}
	}
	problem := optimize.Problem{
		Func: func(x []float64) float64 {
			p := Params{
				R:     math.Exp(x[0]),
				Alpha: math.Exp(x[1]),
				A:     math.Exp(x[2]),
				B:     math.Exp(x[3]),
			}
			// log-param 下 |x| 无界时 Exp 上溢/下溢 → 返回大惩罚把单纯形推回
			if math.IsInf(p.R, 0) || math.IsInf(p.Alpha, 0) || math.IsInf(p.A, 0) || math.IsInf(p.B, 0) ||
				p.R == 0 || p.Alpha == 0 || p.A == 0 || p.B == 0 {
				return math.Inf(1)
			}
			return -p.logLikelihood(in.Stats)
		},
	}
	res, err := optimize.Minimize(problem, init, &optimize.Settings{}, &optimize.NelderMead{})
	if err != nil && res == nil {
		return FitResult{Converged: false}
	}
	x := res.X
	return FitResult{
		Params: Params{
			R:     math.Exp(x[0]),
			Alpha: math.Exp(x[1]),
			A:     math.Exp(x[2]),
			B:     math.Exp(x[3]),
		},
		LogLik:    -res.F,
		Converged: res.Status != optimize.Failure,
	}
}

// lgamma 对数伽马包装（math.Lgamma 返回 (lg, sign)，取对数值）
func lgamma(x float64) float64 {
	lg, _ := math.Lgamma(x)
	return lg
}
