package model

import "time"

// WhatsAppMessageQueue WhatsApp 消息队列中的单条消息记录
type WhatsAppMessageQueue struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	QueueID   string    `gorm:"type:varchar(64);index" json:"queue_id"`
	MessageID string    `gorm:"type:varchar(64);index" json:"message_id"`
	Content   string    `gorm:"type:text" json:"content"`
	Status    string    `gorm:"type:varchar(20);index" json:"status"`
	Platform  string    `gorm:"type:varchar(32)" json:"platform"`
	Recipient string    `gorm:"type:varchar(64);index" json:"recipient"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (WhatsAppMessageQueue) TableName() string {
	return "whatsapp_message_queues"
}

// WhatsAppQueueStatus WhatsApp 消息队列的整体状态
type WhatsAppQueueStatus struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	QueueID   string    `gorm:"type:varchar(64);uniqueIndex" json:"queue_id"`
	Total     int       `json:"total"`
	Sent      int       `json:"sent"`
	Failed    int       `json:"failed"`
	Status    string    `gorm:"type:varchar(20);index" json:"status"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (WhatsAppQueueStatus) TableName() string {
	return "whatsapp_queue_statuses"
}
