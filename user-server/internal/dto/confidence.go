package dto

import (
	"time"
)

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

	RawIntentConf     float64        `json:"raw_intent_conf"`
	RawLogits         []float64      `json:"raw_logits,omitempty"`
	ExtractedEntities map[string]any `json:"extracted_entities,omitempty"`
	ExpectedEntities  map[string]any `json:"expected_entities,omitempty"`
	LLMLogprobs       []float64      `json:"llm_logprobs,omitempty"`
	RAGChunks         []RAGChunk     `json:"rag_chunks,omitempty"`
	RAGExecuted       bool           `json:"rag_executed,omitempty"`
	LastTurns         []string       `json:"last_turns,omitempty"`

	CustomerLevel     string  `json:"customer_level,omitempty"`
	AgentAvailability float64 `json:"agent_availability,omitempty"`
}

// ConfidenceDecision 置信度决策结果
//
// 由 ConfidenceAggregator.Aggregate 返回，包含 5 维信号快照、聚合置信度、动态阈值和决策区间
type ConfidenceDecision struct {
	SignalID         string      `json:"signal_id"`
	AggregatedConf   float64     `json:"aggregated_conf"`
	DynamicThreshold float64     `json:"dynamic_threshold"`
	DecisionBand     string      `json:"decision_band"`
	VetoTriggered    string      `json:"veto_triggered,omitempty"`
	Signals          FiveSignals `json:"signals"`
	CalculatedAt     time.Time   `json:"calculated_at"`
}

// 决策区间常量
const (
	BandHandoff     = "handoff"
	BandLLMFallback = "llm_fallback"
	BandReview      = "review"
	BandAuto        = "auto"
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

// ThresholdPolicyRequest 阈值策略 Upsert 请求 DTO
type ThresholdPolicyRequest struct {
	PolicyID                string  `json:"policy_id" binding:"required,min=1,max=64"`
	IntentType              string  `json:"intent_type" binding:"required,min=1,max=64"`
	BaseThreshold           float64 `json:"base_threshold" binding:"required,min=0,max=1"`
	CustomerLevelWeight     float64 `json:"customer_level_weight" binding:"min=0,max=1"`
	TimeslotWeight          float64 `json:"timeslot_weight" binding:"min=0,max=1"`
	AgentAvailabilityWeight float64 `json:"agent_availability_weight" binding:"min=0,max=1"`
	BandHandoffUpper        float64 `json:"band_handoff_upper" binding:"min=0,max=1"`
	BandFallbackUpper       float64 `json:"band_fallback_upper" binding:"min=0,max=1"`
	BandReviewUpper         float64 `json:"band_review_upper" binding:"min=0,max=1"`
	ReviewSLASeconds        int     `json:"review_sla_seconds" binding:"min=1"`
	Version                 int     `json:"version" binding:"min=1"`
}
