package model

import "time"

// CustomerSegment 客户分群（R46：Builder/RfmMatrix"保存分群"此前为假按钮——真持久化落此表）
type CustomerSegment struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	RulesJSON   string    `gorm:"type:text" json:"rules_json"` // 规则树/RFM 快照 JSON
	Trigger     string    `gorm:"type:varchar(50)" json:"trigger"`
	Size        int64     `gorm:"default:0" json:"size"` // 创建时规模估算
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CustomerSegment) TableName() string { return "customer_segments" }
