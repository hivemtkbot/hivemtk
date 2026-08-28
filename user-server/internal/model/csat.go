package model

import "time"

// CSATSurvey 客户满意度调查（对标 libredesk CSAT：会话关闭→触发→评分→回流统计）
type CSATSurvey struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID   string     `gorm:"type:varchar(120);uniqueIndex;not null" json:"session_id"` // 一会话一调查
	OneID       string     `gorm:"type:varchar(64);index" json:"one_id"`
	Score       int        `gorm:"default:0" json:"score"` // 1-5；0=未评分
	Comment     string     `gorm:"type:text" json:"comment"`
	Status      string     `gorm:"type:varchar(20);default:'pending';index" json:"status"` // pending/sent/responded
	TriggeredBy string     `gorm:"type:varchar(20);default:'manual'" json:"triggered_by"`  // manual/auto
	SentAt      *time.Time `json:"sent_at,omitempty"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CSATSurvey) TableName() string { return "csat_surveys" }

// CSAT 状态枚举
const (
	CSATStatusPending   = "pending"
	CSATStatusSent      = "sent"
	CSATStatusResponded = "responded"
)

// CSATTemplateKey CSAT 模板配置 KV 键
const CSATTemplateKey = "csat.template"

// CSATDefaultTemplate 默认调查模板
const CSATDefaultTemplate = `{"question":"请您为本次服务打分（1-5 分）","low_threshold":3}`
