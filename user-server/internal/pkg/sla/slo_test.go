package sla

import (
	"testing"
	"time"
)

func TestSLOTracker_DefineAndRecord(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	tr.Define(SLO{
		Name:      "test_define_slo",
		Service:   "test",
		SLITarget: 0.99,
		Window:    1 * time.Hour,
	})
	for i := 0; i < 100; i++ {
		tr.Record("test_define_slo", true)
	}
	tr.Record("test_define_slo", false)

	state := tr.State("test_define_slo")
	if state.TotalEvents != 101 {
		t.Errorf("total: got %d", state.TotalEvents)
	}
	if state.SuccessEvents != 100 {
		t.Errorf("success: got %d", state.SuccessEvents)
	}
	if state.Achievement < 0.989 {
		t.Errorf("achievement too low: %f", state.Achievement)
	}
}

func TestSLOTracker_ErrorBudget(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	tr.Define(SLO{
		Name:      "test_budget",
		Service:   "test",
		SLITarget: 0.99,
		Window:    1 * time.Hour,
	})
	for i := 0; i < 990; i++ {
		tr.Record("test_budget", true)
	}
	for i := 0; i < 5; i++ {
		tr.Record("test_budget", false)
	}
	state := tr.State("test_budget")
	if state.AllowedFailure != 10 {
		t.Errorf("allowed: got %d want 10", state.AllowedFailure)
	}
	if state.Remaining != 5 {
		t.Errorf("remaining: got %d want 5", state.Remaining)
	}
	if state.BudgetUsed > 0.6 {
		t.Errorf("budget used: got %f want ~0.5", state.BudgetUsed)
	}
}

func TestSLOTracker_BudgetExhausted(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	tr.Define(SLO{
		Name:      "test_exhausted",
		Service:   "test",
		SLITarget: 0.99,
		Window:    1 * time.Hour,
	})
	for i := 0; i < 90; i++ {
		tr.Record("test_exhausted", true)
	}
	for i := 0; i < 20; i++ {
		tr.Record("test_exhausted", false)
	}
	state := tr.State("test_exhausted")
	if state.BudgetUsed != 1.0 {
		t.Errorf("budget used should be 1.0, got %f", state.BudgetUsed)
	}
	if state.Remaining != 0 {
		t.Errorf("remaining should be 0, got %d", state.Remaining)
	}
}

func TestSLOTracker_BreachCallback(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	tr.Define(SLO{
		Name:      "test_breach",
		Service:   "test",
		SLITarget: 0.99,
		Window:    1 * time.Hour,
	})
	breachCount := 0
	tr.OnBreach(func(s SLO, st SLOState) {
		breachCount++
	})
	for i := 0; i < 99; i++ {
		tr.Record("test_breach", true)
	}
	for i := 0; i < 100; i++ {
		tr.Record("test_breach", false)
	}
	if breachCount == 0 {
		t.Error("expected breach callback to fire")
	}
}

func TestSLOTracker_AllStates(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	tr.Define(SLO{Name: "a", Service: "x", SLITarget: 0.99})
	tr.Define(SLO{Name: "b", Service: "x", SLITarget: 0.95})
	tr.Define(SLO{Name: "c", Service: "y", SLITarget: 0.999})
	states := tr.AllStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}
}

func TestSLOTracker_UnknownSLO(t *testing.T) {
	InitMetrics()
	tr := NewSLOTracker()
	state := tr.State("nonexistent")
	if state.Name != "" {
		t.Error("expected empty state for unknown SLO")
	}
	tr.Record("nonexistent", true)
}
