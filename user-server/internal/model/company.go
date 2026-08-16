package model

import "time"

// Company 公司维度（USR-CM-02）
// B2B 业务必备：多个客户可以属于同一公司
type Company struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(200);not null;index" json:"name"`
	Domain    string    `gorm:"type:varchar(200);index" json:"domain"`
	Industry  string    `gorm:"type:varchar(50)" json:"industry"`
	Size      string    `gorm:"type:varchar(20)" json:"size"` // 'startup' | 'smb' | 'enterprise'
	Metadata  string    `gorm:"type:jsonb" json:"metadata"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

func (Company) TableName() string { return "companies" }

// CompanyEvent 公司维度事件（用于按公司聚合）
type CompanyEvent struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CompanyID  uint      `gorm:"not null;index" json:"company_id"`
	EventType  string    `gorm:"type:varchar(50);not null;index" json:"event_type"`
	ActorID    string    `gorm:"type:varchar(50);index" json:"actor_id"`
	Payload    string    `gorm:"type:jsonb" json:"payload"`
	OccurredAt time.Time `gorm:"autoCreateTime;index" json:"occurred_at"`
}

func (CompanyEvent) TableName() string { return "company_events" }
