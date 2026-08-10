package dto

// LeadMiningConfig 线索发掘全局配置（请求/响应 DTO）。
// 与 model.LeadMiningConfig 字段一一对应，由 service 层完成转换，
// controller 不再直接引用 model（五层架构 L3 约束）。
type LeadMiningConfig struct {
	ID             int64    `json:"id"`
	Enabled        bool     `json:"enabled"`
	Keywords       []string `json:"keywords"`    // 关键词（命中任一即加分）
	Tags           []string `json:"tags"`        // 命中后打给客户的标签
	Requirement    string   `json:"requirement"` // 线索要求文字描述（判定标准）
	Channels       []string `json:"channels"`    // 启用的渠道；空=全部渠道
	MinIntentScore int      `json:"min_intent_score"`
	Model          string   `json:"model"` // 可选覆盖模型，空=默认
	CreatedAt      int64    `json:"created_at"`
	UpdatedAt      int64    `json:"updated_at"`
}
