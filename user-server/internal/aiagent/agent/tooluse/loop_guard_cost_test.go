package tooluse

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestStopReasonOf_Classification A-3：错误 → 结构化 StopReason 映射
func TestStopReasonOf_Classification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want StopReason
	}{
		{"nil→completed", nil, StopReasonCompleted},
		{"loop→loop_limit", ErrLoopDetected, StopReasonLoopLimit},
		{"wrapped loop", fmt.Errorf("ctx: %w", ErrLoopDetected), StopReasonLoopLimit},
		{"approval denied", ErrApprovalDenied, StopReasonApprovalDenied},
		{"deadline→time_limit", context.DeadlineExceeded, StopReasonTimeLimit},
		{"other→error", errors.New("boom"), StopReasonError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StopReasonOf(c.err); got != c.want {
				t.Fatalf("StopReasonOf(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

// TestLoopGuard_RecordCost_BudgetTrip A-2：累计成本达到预算即熔断
func TestLoopGuard_RecordCost_BudgetTrip(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{Enabled: true, CostBudgetUSD: 3.0})

	if r := guard.RecordCost("t1", 1.0); r != StopReasonNone {
		t.Fatalf("round1 should not trip, got %q", r)
	}
	if r := guard.RecordCost("t1", 1.0); r != StopReasonNone {
		t.Fatalf("round2 (total 2.0 < 3.0) should not trip, got %q", r)
	}

	if r := guard.RecordCost("t1", 1.0); r != StopReasonCostLimit {
		t.Fatalf("round3 (total == budget) should trip cost_limit, got %q", r)
	}
	if guard.UsedCost("t1") < 3.0 {
		t.Fatalf("used cost should accumulate, got %f", guard.UsedCost("t1"))
	}
	if sr := guard.TraceStopReason("t1"); sr != StopReasonCostLimit {
		t.Fatalf("last stop reason = %q, want cost_limit", sr)
	}

	if r := guard.RecordCost("t2", 1.0); r != StopReasonNone {
		t.Fatalf("independent trace should not trip, got %q", r)
	}
}

// TestLoopGuard_RecordCost_DriftDetection A-2 简化漂移：单轮成本 > 已有轮次均值 5x 即熔断
func TestLoopGuard_RecordCost_DriftDetection(t *testing.T) {

	guard2 := NewLoopGuard(LoopGuardConfig{Enabled: true})
	_ = guard2.RecordCost("b", 0.02)
	_ = guard2.RecordCost("b", 0.02)
	if r := guard2.RecordCost("b", 0.10); r != StopReasonNone {
		t.Fatalf("exactly 5x average should NOT trip (strict >), got %q", r)
	}

	guard3 := NewLoopGuard(LoopGuardConfig{Enabled: true})
	_ = guard3.RecordCost("c", 0.02)
	_ = guard3.RecordCost("c", 0.02)
	if r := guard3.RecordCost("c", 0.11); r != StopReasonCostLimit {
		t.Fatalf(">5x average should trip cost_limit, got %q", r)
	}
	if sr := guard3.TraceStopReason("c"); sr != StopReasonCostLimit {
		t.Fatalf("stop reason = %q, want cost_limit", sr)
	}
}

// TestLoopGuard_RecordCost_DriftZeroBaseline 历史均值为 0（如全缓存命中）时不触发漂移
func TestLoopGuard_RecordCost_DriftZeroBaseline(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{Enabled: true})
	if r := guard.RecordCost("z", 0.5); r != StopReasonNone {
		t.Fatalf("first round must never drift-trip, got %q", r)
	}
}

// TestLoopGuard_RecordCost_Disabled 预算未配置/未启用时零干预；非正成本忽略
func TestLoopGuard_RecordCost_Disabled(t *testing.T) {
	guard := NewLoopGuard(DefaultLoopGuardConfig())
	for i := 0; i < 10; i++ {
		if r := guard.RecordCost("x", 100.0); r != StopReasonNone {
			t.Fatalf("no budget configured should never trip, got %q", r)
		}
	}
	if r := guard.RecordCost("x", -1); r != StopReasonNone {
		t.Fatalf("non-positive cost should be ignored, got %q", r)
	}
	var nilGuard *LoopGuard
	if r := nilGuard.RecordCost("y", 1); r != StopReasonNone {
		t.Fatalf("nil guard should be safe, got %q", r)
	}
}

// TestLoopGuard_CheckAndRecord_SetsLoopLimit 循环命中时同步产出结构化 stop reason
func TestLoopGuard_CheckAndRecord_SetsLoopLimit(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{Enabled: true, MaxRepeatCount: 2})
	args := map[string]any{"id": "1"}
	for i := 0; i < 2; i++ {
		if err := guard.CheckAndRecord("lt", "tool.a", args); err != nil {
			t.Fatalf("call %d should pass: %v", i+1, err)
		}
	}
	if err := guard.CheckAndRecord("lt", "tool.a", args); !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("3rd identical call should be rejected, got %v", err)
	}
	if sr := guard.TraceStopReason("lt"); sr != StopReasonLoopLimit {
		t.Fatalf("stop reason = %q, want loop_limit", sr)
	}
}

// TestLoopGuard_FinishTrace 结束后清理该 trace 全部状态
func TestLoopGuard_FinishTrace(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{Enabled: true, CostBudgetUSD: 0.01})
	_ = guard.RecordCost("ft", 0.02)
	if guard.UsedCost("ft") <= 0 {
		t.Fatal("cost should be recorded before finish")
	}
	guard.FinishTrace("ft")
	if c := guard.UsedCost("ft"); c != 0 {
		t.Fatalf("cost should be cleared after FinishTrace, got %f", c)
	}
	if sr := guard.TraceStopReason("ft"); sr != StopReasonNone {
		t.Fatalf("stop reason should be cleared, got %q", sr)
	}
}
