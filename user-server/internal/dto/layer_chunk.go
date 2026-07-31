// Package dto 提供数据传输对象 (Layer 决策 / FAQ 匹配 / 流式输出)
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T11 / T12 / T13)
//
// 三个核心 DTO:
//   - LayerDecision       双层架构路由决策 (Layer1 / Layer2)
//   - FAQMatchResult      FAQ 匹配结果 (含 score + 命中后元数据)
//   - StreamChunk         WebSocket 流式输出 (start/delta/final/error 4 类)
package dto

// Layer 取值
const (
	Layer1 = "layer1" // FAQ/SOP 模板快速匹配 (零 LLM, < 100ms)
	Layer2 = "layer2" // LLM 兜底 (1-3s, 启用 Agent Loop)
)

// LayerDecisionReason 取值
const (
	ReasonFAQHit            = "faq_hit"             // FAQ 命中 (高置信度)
	ReasonSOPHit            = "sop_hit"             // SOP 模板命中
	ReasonConfidenceHigh    = "confidence_high"     // 聚合置信度足够高
	ReasonConfidenceLow     = "confidence_low"      // 聚合置信度不足,降级到 LLM
	ReasonFallback          = "fallback"            // 通用降级
	ReasonLayer1Disabled    = "layer1_disabled"     // Layer1 开关关闭
	ReasonNoFAQ             = "no_faq"              // 无 FAQ 可匹配
	ReasonNoSOP             = "no_sop"              // 无 SOP 可匹配
	ReasonIntentUnknown     = "intent_unknown"      // 意图未知
	ReasonLowConfidenceSkip = "low_confidence_skip" // FAQ 命中但置信度低于阈值
)

// LayerDecision 双层架构路由决策
type LayerDecision struct {
	Layer      string  `json:"layer"`        // layer1 / layer2
	SkipLLM    bool    `json:"skip_llm"`     // 是否跳过 LLM
	Reply      string  `json:"reply"`        // Layer1 命中时的模板回复
	Reason     string  `json:"reason"`       // 决策原因
	Confidence float64 `json:"confidence"`   // 命中置信度 0-1
	FAQID      uint    `json:"faq_id"`       // FAQ 命中 ID
	SOPID      uint    `json:"sop_id"`       // SOP 命中 ID
	Intent     string  `json:"intent"`       // 当前意图
	WallMs     int     `json:"wall_ms"`      // 决策耗时 ms
	Metadata   string  `json:"metadata"`     // 附加元数据 (JSON)
}

// FAQMatchResult FAQ 匹配结果
type FAQMatchResult struct {
	Entry     *FAQEntry `json:"entry,omitempty"`
	Score     float64   `json:"score"`     // 匹配分 (0-1)
	Rank      int       `json:"rank"`      // 排名 (0=top1)
	HitCount  int64     `json:"hit_count"` // 命中次数
	MatchType string    `json:"match_type"` // keyword / embedding / exact
}

// FAQEntry FAQ 简短 DTO (避免直接暴露 model)
type FAQEntry struct {
	ID         uint     `json:"id"`
	Question   string   `json:"question"`
	Answer     string   `json:"answer"`
	Keywords   []string `json:"keywords"`
	Category   string   `json:"category"`
	Intent     string   `json:"intent"`
	Confidence float64  `json:"confidence"`
	HitCount   int64    `json:"hit_count"`
	Enabled    *bool    `json:"enabled,omitempty"` // 2026-07-31 P1-A: 前端管理页面需要
	// Task 15: 强 1对1 改造 - FAQ 归属智能体
	//   nil  = 共享池 (Match API 兜底)
	//   &N>0 = 智能体 N 私有
	AgentID   *uint  `json:"agent_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// SOPTemplate SOP 模板 DTO (2026-07-31 P1-A: 前端 SOP 模板管理页面)
type SOPTemplate struct {
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	Intent     string  `json:"intent"`
	Stage      string  `json:"stage"`
	Template   string  `json:"template"`
	Vars       string  `json:"vars"`
	Priority   int     `json:"priority"`
	Confidence float64 `json:"confidence"`
	HitCount   int64   `json:"hit_count"`
	Enabled    *bool   `json:"enabled,omitempty"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
}

// StreamChunkType 取值
const (
	ChunkTypeStart  = "start"  // 流开始 (含 trace_id)
	ChunkTypeDelta  = "delta"  // 增量文本
	ChunkTypeFinal  = "final"  // 流结束 (含完整回复 + steps)
	ChunkTypeError  = "error"  // 错误
	ChunkTypeCancel = "cancel" // 取消
)

// StreamChunk WebSocket 流式输出
//
// 协议:
//   - {"type":"start", "trace_id":"xxx", "intent":"greeting"} -> 流开始
//   - {"type":"delta", "text":"你好"} -> 增量 chunk
//   - {"type":"final", "text":"完整回复", "steps":[...], "wall_ms":1234} -> 结束
//   - {"type":"error", "msg":"llm_timeout"} -> 错误
type StreamChunk struct {
	Type     string         `json:"type"`           // start/delta/final/error/cancel
	TraceID  string         `json:"trace_id"`       // 链路追踪 ID
	Text     string         `json:"text,omitempty"` // 增量/最终文本
	Intent   string         `json:"intent,omitempty"`
	Step     string         `json:"step,omitempty"` // 当前阶段名
	Steps    []SalesStepLog `json:"steps,omitempty"`
	WallMs   int            `json:"wall_ms,omitempty"`
	Layer    string         `json:"layer,omitempty"`     // layer1/layer2
	Model    string         `json:"model,omitempty"`     // LLM model
	Tokens   int            `json:"tokens,omitempty"`    // 累计 tokens
	Error    string         `json:"error,omitempty"`     // 错误信息
	Metadata string         `json:"metadata,omitempty"`  // 附加元数据 (JSON)
}
