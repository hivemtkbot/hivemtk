package model

import "time"

// SalesEvent 销售事件流持久化（H2 技术债修复）
//
// 取代原 SalesDashboard 纯内存 slice 存储（重启丢数据、无界增长 OOM 风险）。
// 记录订单 / 跟进 / AI 谈单 / 订单草稿四类销售事件，供销售工作台与业绩聚合
// 做 DB 权威统计；与实时驾驶舱（dashboard_sse_stats）互补。
type SalesEvent struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType   string    `gorm:"type:varchar(30);index" json:"event_type"` // order / followup / ai_deal / order_draft
	OrderID     string    `gorm:"type:varchar(64);index" json:"order_id"`
	DraftID     string    `gorm:"type:varchar(64);index" json:"draft_id"`
	CustomerID  string    `gorm:"type:varchar(64);index" json:"customer_id"`
	OwnerID     string    `gorm:"type:varchar(64);index" json:"owner_id"` // 销售/负责人
	ProductName string    `gorm:"type:varchar(200)" json:"product_name"`
	Amount      float64   `json:"amount"`
	Action      string    `gorm:"type:varchar(20)" json:"action"`     // 草稿动作：created/confirmed/cancelled/expired
	Channel     string    `gorm:"type:varchar(30)" json:"channel"`    // 跟进渠道
	Result      string    `gorm:"type:varchar(20)" json:"result"`     // 跟进结果：converted/contacted/...
	Intent      string    `gorm:"type:varchar(50)" json:"intent"`     // AI 谈单意图
	IsAI        bool      `json:"is_ai"`                              // 跟进是否 AI 执行
	IsAIHandled bool      `json:"is_ai_handled"`                      // 订单是否 AI 独立成单
	Replied     bool      `json:"replied"`                            // AI 谈单是否回复
	Transferred bool      `json:"transferred"`                        // AI 谈单是否转人工
	CostTokens  int       `json:"cost_tokens"`
	LatencyMs   int       `json:"latency_ms"`
	SalesName   string    `gorm:"type:varchar(100)" json:"sales_name"` // 销售档案：姓名
	Team        string    `gorm:"type:varchar(100)" json:"team"`       // 销售档案：团队
	Tags        string    `gorm:"type:text" json:"tags"`               // 销售档案：标签（逗号分隔）
	JoinedAt    *time.Time `json:"joined_at,omitempty"`                // 销售档案：入职时间
	Confidence  float64   `json:"confidence"`
	Source      string    `gorm:"type:varchar(30)" json:"source"`
	OccurredAt  time.Time `gorm:"index" json:"occurred_at"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SalesEvent) TableName() string { return "sales_events" }

// 销售事件类型常量
const (
	SalesEventTypeOrder       = "order"
	SalesEventTypeFollowUp    = "followup"
	SalesEventTypeAIDeal      = "ai_deal"
	SalesEventTypeOrderDraft  = "order_draft"
	SalesEventTypeSalesProfile = "sales_profile"
)
