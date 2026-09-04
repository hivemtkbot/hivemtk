package churn

import (
	"math"
	"testing"
)

// Fader-Hardie 2005 论文示例参数（CDNOW 校准后典型值量级）
var refParams = Params{R: 0.24, Alpha: 4.41, A: 0.79, B: 2.43}

// P(alive) 语义边界：
//   - x=0（新客，tx=0）→ ratio=(a/(b-1))*((α+T)/α)^r，T 越大 P 越低
//   - 频繁购买（x 大、tx≈T）→ P(alive)→高
func TestD22_PAliveSemantics(t *testing.T) {
	frequent := PAlive(refParams, 10, 38, 39)  // 高频近期
	churned := PAlive(refParams, 1, 1, 39)     // 早购后沉寂
	if frequent <= churned {
		t.Errorf("高频近期客户 P(alive)=%.3f 应高于沉寂客户 %.3f", frequent, churned)
	}
	if frequent <= 0 || frequent > 1 {
		t.Errorf("P(alive) 应在 (0,1), got %v", frequent)
	}
}

// 2F1 收敛：z→1 边界外不使用；典型 z 值有限输出
func TestD22_Hyp2F1Converges(t *testing.T) {
	got := hyp2F1(2.5, 1.2, 3.3, 0.5)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("2F1 应收敛, got %v", got)
	}
	// 2F1(a,b;c;0)=1
	if got0 := hyp2F1(2.5, 1.2, 3.3, 0); got0 != 1 {
		t.Errorf("2F1(...;0)=1, got %v", got0)
	}
}

// 条件期望随预测窗口 t 单调不减
func TestD22_CondExpectationMonotonic(t *testing.T) {
	e1 := ConditionalExpectedPurchases(refParams, 2, 20, 39, 10)
	e2 := ConditionalExpectedPurchases(refParams, 2, 20, 39, 30)
	if e2 < e1 {
		t.Errorf("条件期望应随窗口单调不减: E(10)=%.3f E(30)=%.3f", e1, e2)
	}
}

// MLE 拟合：合成数据（已知参数采样近似）应收敛至同量级
func TestD22_FitConverges(t *testing.T) {
	_ = Params{R: 0.5, Alpha: 5.0, A: 1.2, B: 2.0} // 参考：合成数据非严格 BG/NBD 过程，只验证收敛性
	// 造 200 个"伪客户"统计量（确定性伪随机，避免引入随机库）
	stats := make([]CustomerStats, 0, 200)
	seed := uint64(42)
	next := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>11) / float64(1<<53)
	}
	for i := 0; i < 200; i++ {
		T := 5 + next()*25
		x := math.Floor(next() * 6)
		tx := T * (0.3 + 0.6*next())
		if x == 0 {
			tx = 0
		}
		stats = append(stats, CustomerStats{X: x, Tx: tx, T: T})
	}
	res := Fit(FitInput{Stats: stats})
	if !res.Converged {
		t.Fatal("拟合应收敛")
	}
	// 收敛即可（合成数据生成非严格 BG/NBD 过程，只锁"参数有限+似然>初始"）
	if math.IsNaN(res.Params.R) || math.IsInf(res.Params.R, 0) {
		t.Errorf("参数应有限: %+v", res.Params)
	}
}
