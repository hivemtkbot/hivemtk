package dto

// intent.go 销冠域 - 意图识别 DTO
//
// 从 service 包迁入（P2-6 DTO 层补全）：
//   - RecognizeResult：意图识别结果，由 IntentRecognizer.Recognize 返回，
//     被 SalesResponse.Intent 引用，被 controller/intent_controller.go 使用

// RecognizeResult 识别结果
type RecognizeResult struct {
	IntentType      string         `json:"intent_type"`
	IntentName      string         `json:"intent_name"`
	Confidence      float64        `json:"confidence"`
	ConfidenceLevel string         `json:"confidence_level"`
	IntentSubtype   string         `json:"intent_subtype"`
	Entities        map[string]any `json:"entities"`
	Sentiment       string         `json:"sentiment"`
	Method          string         `json:"method"` // rule / llm
	LLMModel        string         `json:"llm_model,omitempty"`
	CostTokens      int            `json:"cost_tokens"`
	LatencyMs       int            `json:"latency_ms"`
	Platform        string         `json:"platform,omitempty"`
}
