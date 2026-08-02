package model

import "time"

// ============================================================================
// G7 反馈学习闭环：反馈记录持久化模型
// ----------------------------------------------------------------------------
// 对应 service.FeedbackLearner.RecordFeedback 的落库载体。
// service.FeedbackRecord 是进程内传输结构（含业务语义），本 ORM 模型是其
// 持久化映射，字段一一对应，避免 service 包反向依赖 model 包时引入冲突。
//
// 数据流：
//   SalesEngine.recordFeedback → FeedbackLearner.RecordFeedback
//     → INSERT feedback_records（本表）+ 更新内存缓存（intentCache/sopCache）
//     → FeedbackLearningService.ExtractProfile 周期读取 → 销冠画像快照
// ============================================================================

// FeedbackRecordORM 反馈记录持久化行
type FeedbackRecordORM struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID     string    `gorm:"type:varchar(64);index" json:"session_id"`
	CustomerID    string    `gorm:"type:varchar(64);index" json:"customer_id"`
	IntentType    string    `gorm:"type:varchar(50);index" json:"intent_type"`
	Confidence    float64   `gorm:"type:decimal(5,4);default:0" json:"confidence"`
	SOPName       string    `gorm:"type:varchar(100)" json:"sop_name"`
	AIReply       string    `gorm:"type:text" json:"ai_reply"`
	HumanReply    string    `gorm:"type:text" json:"human_reply"`      // 人工修订（可选）
	CustomerAccept bool     `gorm:"default:false" json:"customer_accept"` // 客户是否接受
	Transferred   bool      `gorm:"default:false" json:"transferred"`  // 是否转人工
	TransferReason string  `gorm:"type:varchar(200)" json:"transfer_reason"`
	Tokens        int       `gorm:"default:0" json:"tokens"`
	LatencyMs     int       `gorm:"default:0" json:"latency_ms"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 表名
func (FeedbackRecordORM) TableName() string { return "feedback_records" }
