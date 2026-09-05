package service

import (
	"context"
	"time"

	"hivemtk-user/internal/pkg/metrics"
)

var (
	agentLoopStops = metrics.NewCounter(
		"hivemtk_agent_loop_stops_total",
		"Agent Loop stop_reason distribution",
		[]string{"stop_reason"},
	)
)

type agentLoopStopReason string

const (
	stopReasonNone        agentLoopStopReason = ""
	stopReasonCompleted   agentLoopStopReason = "completed"
	stopReasonWallClock   agentLoopStopReason = "wall_clock_timeout"
	stopReasonTokenBudget agentLoopStopReason = "token_budget_exhausted"
	stopReasonCostBudget  agentLoopStopReason = "cost_budget_exhausted"
	stopReasonCostDrift   agentLoopStopReason = "cost_drift_detected"
	stopReasonLLMError    agentLoopStopReason = "llm_error"
	stopReasonEmptyFinal  agentLoopStopReason = "empty_final_content"
)

func agentLoopDriftFactor() float64 {
	return GlobalConfigParam().GetFloat(context.Background(), "agent_llm", "agent_loop_drift_factor", 2.5)
}

var agentLoopMaxTotalCostUSD = 0.50

// SetAgentLoopMaxTotalCostUSD 注入美元成本预算；≤0 时保持默认
func SetAgentLoopMaxTotalCostUSD(usd float64) {
	if usd <= 0 {
		return
	}
	agentLoopMaxTotalCostUSD = usd
}

type agentLoopGuard struct {
	startedAt    time.Time
	totalTimeout time.Duration
	maxTokens    int
	maxCostUSD   float64

	maxRepeatCalls   int
	costDriftFactor2 float64
	iterationCount   int

	usedTokens int
	usedCost   float64
	iterCosts  []float64

	pendingEstimateTokens int
	pendingEstimateCost   float64
}

func newAgentLoopGuard(totalTimeout time.Duration, maxTokens int, maxCostUSD float64) *agentLoopGuard {
	return &agentLoopGuard{
		startedAt:        time.Now(),
		totalTimeout:     totalTimeout,
		maxTokens:        maxTokens,
		maxCostUSD:       maxCostUSD,
		maxRepeatCalls:   3,
		costDriftFactor2: 5.0,
	}
}

func (g *agentLoopGuard) check() agentLoopStopReason {
	if g.totalTimeout > 0 && time.Since(g.startedAt) >= g.totalTimeout {
		return stopReasonWallClock
	}
	if g.maxTokens > 0 && g.usedTokens >= g.maxTokens {
		return stopReasonTokenBudget
	}
	if g.maxCostUSD > 0 && g.usedCost >= g.maxCostUSD {
		return stopReasonCostBudget
	}
	if g.costDrifted() {
		return stopReasonCostDrift
	}
	return stopReasonNone
}

func (g *agentLoopGuard) charge(tokens int, costUSD float64) {
	if g.pendingEstimateTokens > 0 {
		g.usedTokens -= g.pendingEstimateTokens
		g.pendingEstimateTokens = 0
	}
	if g.pendingEstimateCost > 0 {
		g.usedCost -= g.pendingEstimateCost
		g.pendingEstimateCost = 0
	}
	if tokens > 0 {
		g.usedTokens += tokens
	}
	if costUSD > 0 {
		g.usedCost += costUSD
	}
	g.iterCosts = append(g.iterCosts, costUSD)
}

func (g *agentLoopGuard) ChargeEstimated(estimatedTokens int, estimatedCostUSD float64) {
	if estimatedTokens > 0 {
		g.usedTokens += estimatedTokens
		g.pendingEstimateTokens += estimatedTokens
	}
	if estimatedCostUSD > 0 {
		g.usedCost += estimatedCostUSD
		g.pendingEstimateCost += estimatedCostUSD
	}
}

func (g *agentLoopGuard) ChargeSettle(actualTokens int, actualCostUSD float64) {
	g.charge(actualTokens, actualCostUSD)
}

func (g *agentLoopGuard) costDrifted() bool {
	if len(g.iterCosts) < 6 {
		return false
	}
	first3 := avgF64(g.iterCosts[:3])
	last3 := avgF64(g.iterCosts[len(g.iterCosts)-3:])
	if first3 <= 0 {
		return false
	}
	return last3 >= first3*agentLoopDriftFactor()
}

func (g *agentLoopGuard) RemainingTokens() int {
	if g.maxTokens <= 0 {
		return -1
	}
	remain := g.maxTokens - g.usedTokens
	if remain < 0 {
		return 0
	}
	return remain
}

func (g *agentLoopGuard) RemainingCostUSD() float64 {
	if g.maxCostUSD <= 0 {
		return -1
	}
	remain := g.maxCostUSD - g.usedCost
	if remain < 0 {
		return 0
	}
	return remain
}

func (g *agentLoopGuard) Iteration(i int) {
	g.iterationCount = i
}

func (g *agentLoopGuard) Finish() agentLoopStopReason {
	if r := g.check(); r != stopReasonNone {
		return r
	}
	return stopReasonCompleted
}

func avgF64(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
