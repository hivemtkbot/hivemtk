package dto

// LeadMiningConfig 线索发掘全局配置（请求/响应 DTO）。
// 与 model.LeadMiningConfig 字段一一对应，由 service 层完成转换，
// controller 不再直接引用 model（五层架构 L3 约束）。
type LeadMiningConfig struct {
	ID             int64    `json:"id"`
	Enabled        bool     `json:"enabled"`
	Keywords       []string `json:"keywords"`    
	Tags           []string `json:"tags"`        
	Requirement    string   `json:"requirement"` 
	Channels       []string `json:"channels"`    
	MinIntentScore int      `json:"min_intent_score"`
	Model          string   `json:"model"` 
	CreatedAt      int64    `json:"created_at"`
	UpdatedAt      int64    `json:"updated_at"`
}

