package service

import (
	"testing"
	"time"
)

func TestAgentLoopGuard_NoTripWithinBudget(t *testing.T) {
	g := newAgentLoopGuard(10*time.Second, 1000, 1.0)
	for i := 0; i < 5; i++ {
		if r := g.check(); r != stopReasonNone {
			t.Fatalf("预算内不应触发，实际 %s", r)
		}
		g.charge(100, 0.05)
	}
}

func TestAgentLoopGuard_TokenBudgetTrips(t *testing.T) {
	g := newAgentLoopGuard(time.Minute, 500, 0)
	g.charge(400, 0.01)
	if r := g.check(); r != stopReasonNone {
		t.Fatalf("未达上限不应触发，实际 %s", r)
	}
	g.charge(150, 0.01) // 累计 550 ≥ 500
	if r := g.check(); r != stopReasonTokenBudget {
		t.Fatalf("期望 token_budget_exhausted，实际 %s", r)
	}
}

func TestAgentLoopGuard_CostBudgetTrips(t *testing.T) {
	g := newAgentLoopGuard(time.Minute, 0, 0.10)
	g.charge(10, 0.06)
	g.charge(10, 0.06) // 0.12 ≥ 0.10
	if r := g.check(); r != stopReasonCostBudget {
		t.Fatalf("期望 cost_budget_exhausted，实际 %s", r)
	}
}

func TestAgentLoopGuard_WallClockTrips(t *testing.T) {
	g := newAgentLoopGuard(20*time.Millisecond, 0, 0)
	time.Sleep(50 * time.Millisecond)
	if r := g.check(); r != stopReasonWallClock {
		t.Fatalf("期望 wall_clock_timeout，实际 %s", r)
	}
}

func TestAgentLoopGuard_CostDriftTrips(t *testing.T) {
	g := newAgentLoopGuard(time.Minute, 0, 0) // 关闭预算，仅测漂移
	costs := []float64{0.01, 0.01, 0.01, 0.03, 0.04, 0.03}
	for _, c := range costs {
		if r := g.check(); r == stopReasonCostDrift {
			t.Fatalf("第 %d 轮即触发漂移，过早", len(g.iterCosts)+1)
		}
		g.charge(0, c)
	}
	if r := g.check(); r != stopReasonCostDrift {
		t.Fatalf("近3轮均值≥首3轮2.5x 应触发漂移，实际 %s", r)
	}
}

func TestAgentLoopGuard_CostDriftNoFalsePositiveOnZeroBase(t *testing.T) {
	g := newAgentLoopGuard(time.Minute, 0, 0)
	// 首3轮成本为0（缓存命中），后3轮非零：不得误报（除零保护）
	for _, c := range []float64{0, 0, 0, 0.5, 0.5, 0.5} {
		g.charge(0, c)
	}
	if r := g.check(); r == stopReasonCostDrift {
		t.Fatalf("零基准漂移不应触发")
	}
}

func TestAgentLoopGuard_CheckPriorityOrder(t *testing.T) {
	// 时间 > token > 美元：同时超限时返回时间维度
	g := newAgentLoopGuard(20*time.Millisecond, 10, 0.01)
	g.charge(1000, 1.0)
	time.Sleep(50 * time.Millisecond)
	if r := g.check(); r != stopReasonWallClock {
		t.Fatalf("期望按优先级返回 wall_clock_timeout，实际 %s", r)
	}
}

func TestSetAgentLoopMaxTotalCostUSD(t *testing.T) {
	old := agentLoopMaxTotalCostUSD
	defer func() { agentLoopMaxTotalCostUSD = old }()
	SetAgentLoopMaxTotalCostUSD(2.0)
	if agentLoopMaxTotalCostUSD != 2.0 {
		t.Fatalf("注入失败: %v", agentLoopMaxTotalCostUSD)
	}
	SetAgentLoopMaxTotalCostUSD(-1)
	if agentLoopMaxTotalCostUSD != 2.0 {
		t.Fatalf("非法注入不应生效")
	}
}
