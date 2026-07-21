package metrics

// tuning_metrics.go 置信度/拟人度/反馈学习 业务指标埋点
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md P3 监控指标
//
// 解决 import cycle：
//   middleware 包某些文件（app_key_auth）依赖 service 包；
//   service 包需要调用本包的 Record*/Observe* 函数 → 本包不依赖 service / middleware，
//   因此 service → metrics 是单向依赖，无环。
//
// 私域独立部署：无 merchant_id 字段

// ============================================================================
// 置信度
// ============================================================================

// RecordConfidenceScored 记录置信度打分
//
// 每次 SalesEngine.shouldTransferByConfidence 调用 +1
func RecordConfidenceScored(scenario string) {
	GlobalMetrics.ConfidenceScoredTotal.Inc(scenario)
}

// RecordConfidenceDecision 记录置信度决策
//
// decision: auto_reply / llm_fallback / review_queue / transfer
func RecordConfidenceDecision(scenario, decision string) {
	switch decision {
	case "transfer":
		GlobalMetrics.ConfidenceTransferTotal.Inc(scenario)
	case "auto_reply":
		GlobalMetrics.ConfidenceAutoReplyTotal.Inc(scenario)
	}
}

// ============================================================================
// 拟人度
// ============================================================================

// RecordHumanizeScored 记录拟人度评估
//
// evaluator: rule / llm / chain
// strategy: full / boundary / sampled
func RecordHumanizeScored(evaluator, strategy string) {
	GlobalMetrics.HumanizeScoredTotal.Inc(evaluator + "|" + strategy)
}

// RecordHumanizeRegenerate 记录拟人度重生成触发
func RecordHumanizeRegenerate(evaluator string) {
	GlobalMetrics.HumanizeRegenerateTotal.Inc(evaluator)
}

// ObserveHumanizeScore 观察拟人度评分值
func ObserveHumanizeScore(evaluator string, score float64) {
	GlobalMetrics.HumanizeScoreValue.Observe(evaluator, score)
}

// ============================================================================
// 反馈学习
// ============================================================================

// RecordFeedbackEvent 记录反馈事件
//
// eventType: explicit / implicit / champion
func RecordFeedbackEvent(eventType, signalKey string) {
	GlobalMetrics.FeedbackEventsTotal.Inc(eventType + "|" + signalKey)
}

// RecordBanditSample 记录 Bandit 采样
func RecordBanditSample(armKey string) {
	GlobalMetrics.FeedbackBanditSamplesTotal.Inc(armKey)
}

// ObserveBanditReward 观察 Bandit 奖励
func ObserveBanditReward(armKey string, reward float64) {
	GlobalMetrics.FeedbackBanditRewardsTotal.Observe(armKey, reward)
}

// RecordPromptCandidate 记录 Prompt 候选生成
func RecordPromptCandidate(scenario string) {
	GlobalMetrics.FeedbackPromptCandidatesTotal.Inc(scenario)
}
