// Package metrics - AI Agent 性能指标 (2026-07-31 AI 智能体性能优化 T21)
//
// 五层架构归属: L2 网关层
// 设计依据: docs/operations/AI_AGENT_PERF_MONITORING.md
//
// 5 个核心指标:
//   - ai_agent_wall_time_seconds   端到端 wall time (Histogram)
//   - ai_agent_lcp_time_seconds    流式首字时间 LCP (Histogram)
//   - ai_agent_layer_decision_total 双层架构决策计数 (Counter)
//   - ai_agent_llm_call_total      LLM 调用计数 (Counter)
//   - ai_agent_fallback_total      降级链触发计数 (Counter)
//
// 暴露方式: 通过 /metrics 端点 (Prometheus 格式)
// 标签规范: agent_type=ai_sales|smart_cs, layer=layer1|layer2, intent=greeting|...
//
// 解决 import cycle: 与 tuning_metrics.go 一样, 本包不依赖 service,
// service → metrics 是单向依赖。
package metrics

// ============================================================================
// AI 智能体 5 指标 (T21)
// ============================================================================

// RecordAIAgentWallTime 记录 AI Agent 端到端 wall time
//
// labels: agent_type|layer|intent
// unit: seconds
func RecordAIAgentWallTime(agentType, layer, intent string, seconds float64) {
	GlobalMetrics.AIAgentWallTime.Observe(agentType+"|"+layer+"|"+intent, seconds)
}

// RecordAIAgentLCPTime 记录 AI Agent 流式首字时间 LCP
//
// labels: agent_type|stream_mode
// unit: seconds
func RecordAIAgentLCPTime(agentType, streamMode string, seconds float64) {
	GlobalMetrics.AIAgentLCPTime.Observe(agentType+"|"+streamMode, seconds)
}

// RecordAIAgentLayerDecision 记录双层架构决策
//
// labels: layer|reason
// reason: faq_hit|sop_hit|confidence_high|fallback|layer1_disabled|no_faq|no_sop|...
func RecordAIAgentLayerDecision(layer, reason string) {
	GlobalMetrics.AIAgentLayerDecision.Inc(layer + "|" + reason)
}

// RecordAIAgentLLMCall 记录 LLM 调用
//
// labels: scenario|model|result
// result: success|failed|timeout|fallback
func RecordAIAgentLLMCall(scenario, model, result string) {
	GlobalMetrics.AIAgentLLMCall.Inc(scenario + "|" + model + "|" + result)
}

// RecordAIAgentFallback 记录降级链触发
//
// labels: from_layer|to_layer|reason
// reason: timeout|error|rate_limit|low_quality
func RecordAIAgentFallback(fromLayer, toLayer, reason string) {
	GlobalMetrics.AIAgentFallback.Inc(fromLayer + "|" + toLayer + "|" + reason)
}
