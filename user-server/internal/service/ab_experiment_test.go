package service

import (
	"fmt"
	"testing"
	"time"
)

// TestAbExperiment_AssignStable 同输入恒同输出，且输出 ∈ {control, treatment}
func TestAbExperiment_AssignStable(t *testing.T) {
	for i := 0; i < 200; i++ {
		expID := fmt.Sprintf("exp_%d", i%7)
		cust := fmt.Sprintf("cust_%d", i)
		want := Assign(expID, cust)
		if want != AbVariantControl && want != AbVariantTreatment {
			t.Fatalf("非法变体 %q", want)
		}
		for j := 0; j < 5; j++ {
			if got := Assign(expID, cust); got != want {
				t.Fatalf("分桶不稳定 (%s,%s): %s vs %s", expID, cust, got, want)
			}
		}
	}
}

// TestAbExperiment_AssignBalance1000 1000 样本均衡性：control 占比 45%-55%
func TestAbExperiment_AssignBalance1000(t *testing.T) {
	n := 1000
	control := 0
	for i := 0; i < n; i++ {
		if Assign("exp_balance", fmt.Sprintf("cust_%d", i)) == AbVariantControl {
			control++
		}
	}
	ratio := float64(control) / float64(n) * 100
	if ratio < 45 || ratio > 55 {
		t.Fatalf("分桶失衡: control = %d/%d (%.1f%%), 要求 45%%-55%%", control, n, ratio)
	}
	t.Logf("control 占比 %.1f%% (%d/%d)", ratio, control, n)
}

// TestAbExperiment_LogExposureFireAndForget 缓冲满非阻塞丢弃并计数
func TestAbExperiment_LogExposureFireAndForget(t *testing.T) {
	a := NewABExperiment(nil, 2)
	defer a.Stop()
	for i := 0; i < 5; i++ {
		a.LogExposure("exp_x", AbVariantControl, fmt.Sprintf("c%d", i), "s1")
	}
	if got := a.DroppedCount.Load(); got != 3 {
		t.Fatalf("DroppedCount = %d, want 3（buffer=2，满则丢弃）", got)
	}
}

// TestAbExperiment_MarkConversionNilDBSafe nil db 安全不 panic
func TestAbExperiment_MarkConversionNilDBSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil db 不应 panic: %v", r)
		}
	}()
	a := NewABExperiment(nil, 4)
	defer a.Stop()
	a.LogExposure("exp_y", AbVariantTreatment, "cust_1", "s1")
	a.MarkConversion("exp_y", "cust_1")
}

// TestAbExperiment_SummariesNilDBSafe nil db 返回预填两变体的零值汇总
func TestAbExperiment_SummariesNilDBSafe(t *testing.T) {
	a := NewABExperiment(nil, 4)
	defer a.Stop()
	res := a.Summaries("exp_y", time.Hour)
	if len(res) != 2 {
		t.Fatalf("应预填两变体, got %d", len(res))
	}
	for _, v := range []string{AbVariantControl, AbVariantTreatment} {
		s := res[v]
		if s.Variant != v || s.Exposed != 0 || s.Converted != 0 || s.ConversionRate != 0 {
			t.Fatalf("nil db 汇总应为零值: %s %+v", v, s)
		}
	}
}

// TestAbExperiment_StopIdempotent Stop 幂等：重复调用与停止后写入均安全
func TestAbExperiment_StopIdempotent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop 后操作不应 panic: %v", r)
		}
	}()
	a := NewABExperiment(nil, 4)
	a.LogExposure("exp_z", AbVariantControl, "c1", "s1")
	a.Stop()
	a.Stop()
	a.LogExposure("exp_z", AbVariantControl, "c2", "s1")
}
