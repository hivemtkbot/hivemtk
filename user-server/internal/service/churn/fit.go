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

// logLikelihood BG/NBD 对数似然（Fader-Hardie 附录 A；省略与参数无关的常数项）
//
// LL_i = ln[ Γ(r+x)/Γ(r) · α^r / (α+T)^(r+x) · B(a,b+x)/B(a,b)
//            + δ(x>0) · Γ(r+x)/Γ(r) · α^r / (α+tx)^(r+x) · B(a+1,b+x-1)/B(a,b) ]
func (p Params) logLikelihood(stats []CustomerStats) float64 {
	total := 0.0
	lgR := lgamma(p.R)
	for _, s := range stats {
		// 公共项 Γ(r+x)/Γ(r) · α^r
		common := lgamma(p.R+s.X) - lgR + p.R*math.Log(p.Alpha)
		// B(a,b+x)/B(a,b) 与 B(a+1,b+x-1)/B(a,b) 的 logBeta 差
		lb1 := lgamma(p.A) + lgamma(p.B+s.X) - lgamma(p.A+p.B+s.X)
		lb2 := lgamma(p.A+1) + lgamma(p.B+s.X-1) - lgamma(p.A+p.B+s.X)
		l1 := common - (p.R+s.X)*math.Log(p.Alpha+s.T) + lb1
		l2 := math.Inf(-1)
		if s.X > 0 {
			l2 = common - (p.R+s.X)*math.Log(p.Alpha+s.Tx) + lb2
		}
		m := math.Max(l1, l2)
		total += m + math.Log(math.Exp(l1-m)+math.Exp(l2-m))
	}
	return total
}

// Fit MLE 拟合四参数（L-BFGS，多起点单点起步；约束 a,b>1+ε, r,α>ε）
func Fit(in FitInput) FitResult {
	if len(in.Stats) == 0 {
		return FitResult{}
	}
	p0 := in.Init
	if p0.R <= 0 || p0.Alpha <= 0 || p0.A <= 1 || p0.B <= 1 {
		p0 = Params{R: 1, Alpha: 1, A: 1.5, B: 2}
	}
	problem := optimize.Problem{
		Func: func(x []float64) float64 {
			p := Params{R: x[0], Alpha: x[1], A: x[2] + 1.001, B: x[3] + 1.001}
			return -p.logLikelihood(in.Stats)
		},
		// 无解析梯度：4 维小问题用 Nelder-Mead（单纯形）足够，避免数值梯度精度噪声
	}
	res, err := optimize.Minimize(problem, []float64{p0.R, p0.Alpha, p0.A - 1.001, p0.B - 1.001}, &optimize.Settings{}, &optimize.NelderMead{})
	if err != nil && res == nil {
		return FitResult{Converged: false}
	}
	x := res.X
	return FitResult{
		Params:    Params{R: x[0], Alpha: x[1], A: x[2] + 1.001, B: x[3] + 1.001},
		LogLik:    -res.F,
		Converged: res.Status != optimize.Failure,
	}
}

// lgamma 对数伽马包装（math.Lgamma 返回 (lg, sign)，取对数值）
func lgamma(x float64) float64 {
	lg, _ := math.Lgamma(x)
	return lg
}
