package model

import "time"

// ChatChannelStatus 渠道状态
type ChatChannelStatus string

const (
	ChatChannelStatusActive   ChatChannelStatus = "active"
	ChatChannelStatusDisabled ChatChannelStatus = "disabled"
)

// ChatChannel 客服 Web Widget 渠道（多通道设计）
//
// 设计理念：
//   - 单部署实例可服务多个外部网站（多 channel）
//   - 每个 channel 对应一个 app_key + 一组白名单 origin
//   - channel 可绑定默认 RAG 产品（智能体）
//   - 私域部署：所有 channel 归属本实例，无 merchant_id
type ChatChannel struct {
	ID                  uint              `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID           string            `gorm:"type:varchar(50);uniqueIndex;not null" json:"channel_id"`
	ChannelName         string            `gorm:"type:varchar(100);not null" json:"channel_name"`
	AppKey              string            `gorm:"type:varchar(64);uniqueIndex;not null" json:"app_key"`
	AppSecretHash       string            `gorm:"type:varchar(128);not null" json:"-"`
	AllowedOrigins      string            `gorm:"type:text" json:"allowed_origins"`
	DefaultRAGProductID uint              `gorm:"index" json:"default_rag_product_id"`
	WelcomeMessage      string            `gorm:"type:text" json:"welcome_message"`
	WidgetColor         string            `gorm:"type:varchar(20);default:'#1989fa'" json:"widget_color"`
	WidgetPosition      string            `gorm:"type:varchar(20);default:'bottom-right'" json:"widget_position"`
	WidgetTitle         string            `gorm:"type:varchar(100);default:'在线客服'" json:"widget_title"`
	Status              ChatChannelStatus `gorm:"type:varchar(20);default:'active'" json:"status"`
	VisitorCount        int64             `gorm:"default:0" json:"visitor_count"`
	SessionCount        int64             `gorm:"default:0" json:"session_count"`
	AutoAssign          bool              `gorm:"default:true" json:"auto_assign"`
	ConfidenceThreshold float64           `gorm:"type:decimal(4,2);default:0.70" json:"confidence_threshold"`
	TargetLanguage      string            `gorm:"type:varchar(8);default:''" json:"target_language"`
	CreatedBy           uint              `json:"created_by"`
	CreatedAt           time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time         `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (ChatChannel) TableName() string {
	return "chat_channels"
}
