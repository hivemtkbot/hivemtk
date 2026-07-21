package dto

// confidence.go 置信度驱动转人工 P0-3 数据传输对象
//
// 五层架构归属: L2 网关/L3 业务 之间的传输层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.1
//
// 私域独立部署: 无 merchant_id 字段

import "time"

// FiveSignals 5 维置信度信号值
//
// 信号源：
//   - IntentConf:  意图分类器置信度（经温度缩放校准）
//   - EntityComp:  实体完整性 = |extracted ∩ expected| / |expected|
//   - CtxRelev:    上下文相关性 = cosine(query, last_3_turns_mean)
//   - RAGQual:     RAG 检索质量 = mean(top-k score) * coverage_ratio
//   - LLMEntropy:  LLM 生成熵 = 1 - normalize(ShannonEntropy(top-20 logprobs))
type FiveSignals struct {
	IntentConf float64 `json:"intent_conf"`
	EntityComp float64 `json:"entity_comp"`
	CtxRelev   float64 `json:"ctx_relev"`
	RAGQual    float64 `json:"rag_qual"`
	LLMEntropy float64 `json:"llm_entropy"`
}

// SignalCollectionInput 5 维信号采集入参
//
// 由 SalesEngine / SmartCSOrchestrator 在每轮对话时构造并传给 ConfidenceAggregator
type SignalCollectionInput struct {
	SessionID  string `json:"session_id"`
	CustomerID string `json:"customer_id"`
	MessageID  string `json:"message_id"`
	Text       string `json:"text"`
	IntentType string `json:"intent_type"`

	// 各信号原始值
	RawIntentConf     float64        `json:"raw_intent_conf"`      // 意图识别器原始置信度（校准前）
	RawLogits         []float64      `json:"raw_logits,omitempty"` // 意图分类器 logits，用于温度缩放
	ExtractedEntities map[string]any `json:"extracted_entities,omitempty"`
	ExpectedEntities  map[string]any `json:"expected_entities,omitempty"`
	LLMLogprobs       []float64      `json:"llm_logprobs,omitempty"` // top-k token logprobs
	RAGChunks         []RAGChunk     `json:"rag_chunks,omitempty"`
	LastTurns         []string       `json:"last_turns,omitempty"` // 最近 3 轮对话

	// 上下文因子（用于动态阈值）
	CustomerLevel     string  `json:"customer_level,omitempty"`     // vip / normal / low
	AgentAvailability float64 `json:"agent_availability,omitempty"` // [0,1] 在线座席空闲比例
}

// ConfidenceDecision 置信度决策结果
//
// 由 ConfidenceAggregator.Aggregate 返回，包含 5 维信号快照、聚合置信度、动态阈值和决策区间
type ConfidenceDecision struct {
	SignalID         string      `json:"signal_id"`
	AggregatedConf   float64     `json:"aggregated_conf"`
	DynamicThreshold float64     `json:"dynamic_threshold"`
	DecisionBand     string      `json:"decision_band"` // handoff / llm_fallback / review / auto
	VetoTriggered    string      `json:"veto_triggered,omitempty"`
	Signals          FiveSignals `json:"signals"`
	CalculatedAt     time.Time   `json:"calculated_at"`
}

// 决策区间常量
const (
	BandHandoff     = "handoff"      // [0, 0.4) 立即转人工
	BandLLMFallback = "llm_fallback" // [0.4, 0.6) LLM 兜底回复（带前缀 + low_confidence 标记）
	BandReview      = "review"       // [0.6, 0.75) 进审核队列
	BandAuto        = "auto"         // [0.75, 1.0] 自动回复
)

// CalibrationResult 校准结果（dto 层暴露给上游）
type CalibrationResult struct {
	Temperature float64 `json:"temperature"`
	ECEBefore   float64 `json:"ece_before"`
	ECEAfter    float64 `json:"ece_after"`
	NLLBefore   float64 `json:"nll_before"`
	NLLAfter    float64 `json:"nll_after"`
	SampleSize  int     `json:"sample_size"`
}

// ABTestAnalysis A/B 测试分析结果
type ABTestAnalysis struct {
	ControlMean      float64 `json:"control_mean"`
	TreatmentMean    float64 `json:"treatment_mean"`
	Difference       float64 `json:"difference"`
	MannWhitneyU     float64 `json:"mann_whitney_u"`
	MannWhitneyP     float64 `json:"mann_whitney_p"`
	BootstrapCILower float64 `json:"bootstrap_ci_lower"`
	BootstrapCIUpper float64 `json:"bootstrap_ci_upper"`
	Significant      bool    `json:"significant"`
}
