package model

import "time"

// AISuggestion AI建议
type AISuggestion struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID  string     `gorm:"type:varchar(120);index" json:"session_id"`
	MessageID  uint       `json:"message_id"`
	Suggestion string     `gorm:"type:text" json:"suggestion"`
	Confidence float64    `gorm:"type:decimal(5,2)" json:"confidence"`
	Source     string     `gorm:"type:varchar(20)" json:"source"` 
	IsUsed     bool       `gorm:"default:false" json:"is_used"`
	UsedBy     uint       `json:"used_by"` 
	UsedAt     *time.Time `json:"used_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (AISuggestion) TableName() string {
	return "ai_suggestions"
}

