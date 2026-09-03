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

// 本文件实现 Agent Loop 的统一护栏对象（对标 OpenLegion/RunGuard/LangGraph 生产实践）：
//   - 步数上限（既有 maxIterations）
//   - wall-clock 总超时（既有 agentLoopTotalTimeout）
//   - token 预算（既有 agentLoopMaxTotalTokens）
//   - 美元成本预算（本轮新增：token 预算不足以约束"贵模型×多轮"场景，
//     业界共识预算必须以美元计价 —— RunGuard《OpenAI Agents SDK Cost Control》）
//   - 成本漂移检测（本轮新增：后 3 轮均价 ≥ 前 3 轮均价 2.5 倍时提前熔断，
//     在总预算耗尽前拦截上下文膨胀型故障）
//
// 设计原则（agentpatterns.tech 生产模式）：
//   - 先检查后消费（check before spend）：每轮 LLM 调用前统一检查
//   - 单一退出路径：所有护栏触发都收敛为结构化 stop_reason

// agentLoopStopReason Agent Loop 结构化停止原因
type agentLoopStopReason string

const (
	stopReasonNone        agentLoopStopReason = ""          // 未触发，继续迭代
	stopReasonCompleted   agentLoopStopReason = "completed" // 正常产出最终回复
	stopReasonWallClock   agentLoopStopReason = "wall_clock_timeout"
	stopReasonTokenBudget agentLoopStopReason = "token_budget_exhausted"
	stopReasonCostBudget  agentLoopStopReason = "cost_budget_exhausted"
	stopReasonCostDrift   agentLoopStopReason = "cost_drift_detected"
	stopReasonLLMError    agentLoopStopReason = "llm_error"
	stopReasonEmptyFinal  agentLoopStopReason = "empty_final_content"
)

// agentLoopDriftFactor 成本漂移熔断倍率：近 3 轮均成本 ≥ 首轮 3 轮均值 × 该倍数即触发。
// 业界参考值 2.5x（RunGuard drift_factor 默认），在预算耗尽前拦截上下文累积型故障。
// seed: agent_llm.agent_loop_drift_factor
func agentLoopDriftFactor() float64 {
	return GlobalConfigParam().GetFloat(context.Background(), "agent_llm", "agent_loop_drift_factor", 2.5)
}

// agentLoopMaxTotalCostUSD Agent Loop 单次运行累计美元成本上限。
//
// 取值依据：本地推理单请求成本≈$0.001 量级；云端最坏路径 gpt-4o($0.03/1k)×50k tokens≈$1.5。
// $0.50 为云端兜底合理值（RunGuard 建议"最坏合理任务成本 ×2 以内"），
// 运行期可经 SetAgentLoopMaxTotalCostUSD 注入调整；≤0 表示禁用该护栏。
var agentLoopMaxTotalCostUSD = 0.50

// SetAgentLoopMaxTotalCostUSD 注入美元成本预算；≤0 时保持默认
func SetAgentLoopMaxTotalCostUSD(usd float64) {
	if usd <= 0 {
		return
	}
	agentLoopMaxTotalCostUSD = usd
}

// agentLoopGuard Agent Loop 统一护栏状态机（纯逻辑，可独立单元测试）
type agentLoopGuard struct {
	startedAt    time.Time
	totalTimeout time.Duration
	maxTokens    int
	maxCostUSD   float64

	maxRepeatCalls   int     // 工具调用循环检测的最大重复次数
	costDriftFactor2 float64 // 单轮成本 vs 历史均值 的漂移因子（默认 5.0，从 LoopGuard 迁移）
	iterationCount   int     // 当前迭代次数（给 remaining API 用）

	usedTokens int
	usedCost   float64
	iterCosts  []float64 // 每轮 LLM 调用成本序列（漂移检测用）

	pendingEstimateTokens int     // ChargeEstimated 先行扣款后待冲抵的 token
	pendingEstimateCost   float64 // ChargeEstimated 先行扣款后待冲抵的美元成本
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

// check 在下一次 LLM 调用前检查全部预算维度，返回触发的 stop_reason（未触发返回空）。
// 检查顺序即熔断优先级：时间 > token > 美元 > 漂移。
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

// charge 记录一轮 LLM 调用的消耗（事后结算）
// 若之前有 ChargeEstimated 先行扣款，先冲抵预估值再累加真实值
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

// ChargeEstimated 预估先行扣款（工具调用返回前先占位，防预算穿透）
// estimatedTokens/estimatedCostUSD 为预估值，事后通过 charge/ChargeSettle 用真实值修正
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

// ChargeSettle 事后结算：用真实值冲抵预估值的差额
// actualTokens/actualCostUSD 为 LLM 返回的真实 usage；预估值偏高则相当于"退回"预算
func (g *agentLoopGuard) ChargeSettle(actualTokens int, actualCostUSD float64) {
	g.charge(actualTokens, actualCostUSD)
}

// costDrifted 成本漂移检测：已产生 ≥6 轮且近 3 轮均成本 ≥ 首 3 轮均值 × agentLoopDriftFactor。
// 首 3 轮均值为 0 时（如全程缓存命中）不触发，避免除零误报。
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

// RemainingTokens 剩余 token 预算（给 P1-4 预算注入 Prompt 用）
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

// RemainingCostUSD 剩余美元预算（给 P1-4 用）
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

// Iteration 当前迭代计数（从 runAgentLoop 注入）
func (g *agentLoopGuard) Iteration(i int) {
	g.iterationCount = i
}

// Finish 收尾，返回当前状态对应的 stop_reason
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
