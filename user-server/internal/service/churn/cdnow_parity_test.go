package churn

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"testing"
)

func loadCDNOW(t *testing.T) []CustomerStats {
	t.Helper()
	f, err := os.Open("testdata/cdnow_customers_summary.csv")
	if err != nil {
		t.Fatalf("打开 CDNOW 数据失败: %v", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("解析 CSV 失败: %v", err)
	}
	stats := make([]CustomerStats, 0, len(rows)-1)
	for i, r := range rows {
		if i == 0 {
			continue
		}
		x, _ := strconv.ParseFloat(r[1], 64)
		tx, _ := strconv.ParseFloat(r[2], 64)
		T, _ := strconv.ParseFloat(r[3], 64)
		stats = append(stats, CustomerStats{X: x, Tx: tx, T: T})
	}
	if len(stats) != 2357 {
		t.Fatalf("CDNOW 应 2357 客户, got %d", len(stats))
	}
	return stats
}

// TestD22_CDNOW_FitParity 对拍 lifetimes 拟合参数（同极值盆 1e-3 内；官方只断言 decimal=2）
func TestD22_CDNOW_FitParity(t *testing.T) {
	stats := loadCDNOW(t)
	res := Fit(FitInput{Stats: stats})
	if !res.Converged {
		t.Fatal("CDNOW 拟合应收敛")
	}
	expected := Params{R: 0.243, Alpha: 4.414, A: 0.793, B: 2.426}
	if d := math.Abs(res.Params.R - expected.R); d > 1e-3 {
		t.Errorf("r = %.6f, want %.3f±1e-3 (diff %.2e)", res.Params.R, expected.R, d)
	}
	if d := math.Abs(res.Params.Alpha - expected.Alpha); d > 1e-3 {
		t.Errorf("alpha = %.6f, want %.3f±1e-3 (diff %.2e)", res.Params.Alpha, expected.Alpha, d)
	}
	if d := math.Abs(res.Params.A - expected.A); d > 1e-3 {
		t.Errorf("a = %.6f, want %.3f±1e-3 (diff %.2e)", res.Params.A, expected.A, d)
	}
	if d := math.Abs(res.Params.B - expected.B); d > 1e-3 {
		t.Errorf("b = %.6f, want %.3f±1e-3 (diff %.2e)", res.Params.B, expected.B, d)
	}
}

// TestD22_CDNOW_HardieExcelConditionalExpectation 对拍 Fader-Hardie 2005 论文 Excel 表
// （lifetimes test_conditional_expectation_returns_same_value_as_Hardie_excel_sheet）：
// 参数取论文校准值，公式实现误差门槛 1e-4（D22 契约的真正落点）。
func TestD22_CDNOW_HardieExcelConditionalExpectation(t *testing.T) {
	p := Params{R: 0.243, Alpha: 4.414, A: 0.793, B: 2.426}
	got := ConditionalExpectedPurchases(p, 2, 30.43, 38.86, 39)
	const want = 1.226
	if d := math.Abs(got - want); d >= 1e-4 {
		t.Errorf("E[Y(39)|x=2,tx=30.43,T=38.86] = %.10f, want %.3f±1e-4 (diff %.2e)", got, want, d)
	}
}

// TestD22_CDNOW_PAliveFormulaParity 对拍 lifetimes conditional_probability_alive 的
// 解析公式（同参数、同输入下数值实现误差 <1e-4）：
// lifetimes 文档示例（README CDNOW 段）：x=6, tx=30.43, T=38.86 → P(alive)≈0.59
// 与论文示例一致；此处用公式交叉验证（1 - 1/(1+ratio) 与 PAlive 恒等）。
func TestD22_CDNOW_PAliveFormulaParity(t *testing.T) {
	p := Params{R: 0.243, Alpha: 4.414, A: 0.793, B: 2.426}

	x, tx, T := 6.0, 30.43, 38.86
	direct := 1 / (1 + (p.A/(p.B+x-1))*math.Exp((p.R+x)*(math.Log(p.Alpha+T)-math.Log(p.Alpha+tx))))
	got := PAlive(p, x, tx, T)
	if d := math.Abs(got - direct); d >= 1e-4 {
		t.Errorf("P(alive) 两条实现路径 diff=%.2e ≥1e-4", d)
	}

	if d := math.Abs(got - 0.707701); d >= 1e-4 {
		t.Errorf("P(alive|x=6,tx=30.43,T=38.86) = %.6f, want 0.707701±1e-4 (diff %.2e)", got, d)
	}
}
