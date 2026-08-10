package model

import "time"

// SecurityAudit 安全审计记录：一次审计任务的结果快照。
// 前端「系统设置 → 安全审计」列表/详情/立即审计使用。
type SecurityAudit struct {
	ID          uint                `gorm:"primaryKey" json:"id"`
	AuditName   string              `gorm:"column:audit_name;size:128;not null" json:"audit_name"`
	RiskLevel   string              `gorm:"column:risk_level;size:32" json:"risk_level"` // low/medium/high/critical
	Score       int                 `gorm:"column:score" json:"score"`
	TotalChecks int                 `gorm:"column:total_checks" json:"total_checks"`
	Passed      int                 `gorm:"column:passed" json:"passed"`
	Failed      int                 `gorm:"column:failed" json:"failed"`
	Warnings    int                 `gorm:"column:warnings" json:"warnings"`
	Status      string              `gorm:"column:status;size:32;default:'done'" json:"status"`
	StartedAt   time.Time           `gorm:"column:started_at" json:"started_at"`
	FinishedAt  *time.Time          `gorm:"column:finished_at" json:"finished_at"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	Items       []SecurityAuditItem `gorm:"foreignKey:AuditID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
}

// TableName 指定审计记录表名
func (SecurityAudit) TableName() string { return "security_audits" }

// SecurityAuditItem 审计项：单次审计中的一个具体检查点结果。
type SecurityAuditItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AuditID   uint      `gorm:"column:audit_id;index;not null" json:"audit_id"`
	Name      string    `gorm:"column:name;size:128" json:"name"`
	Category  string    `gorm:"column:category;size:64" json:"category"`
	Level     string    `gorm:"column:level;size:32" json:"level"`   // critical/high/medium/low
	Result    string    `gorm:"column:result;size:32" json:"result"` // pass/fail/warn
	Message   string    `gorm:"column:message;type:text" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定审计项表名
func (SecurityAuditItem) TableName() string { return "security_audit_items" }
