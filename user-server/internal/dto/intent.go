package dto

// RecognizeResult 识别结果
type RecognizeResult struct {
	IntentType      string         `json:"intent_type"`
	IntentName      string         `json:"intent_name"`
	Confidence      float64        `json:"confidence"`
	ConfidenceLevel string         `json:"confidence_level"`
	IntentSubtype   string         `json:"intent_subtype"`
	Entities        map[string]any `json:"entities"`
	Sentiment       string         `json:"sentiment"`
	Method          string         `json:"method"`
	LLMModel        string         `json:"llm_model,omitempty"`
	CostTokens      int            `json:"cost_tokens"`
	LatencyMs       int            `json:"latency_ms"`
	TopKExamples    []string       `json:"top_k_examples,omitempty"`
}
