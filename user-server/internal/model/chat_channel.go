package model

import "time"

// ChatChannelStatus 渠道状态
type ChatChannelStatus string

const (
	ChatChannelStatusActive   ChatChannelStatus = "active"   // 启用
	ChatChannelStatusDisabled ChatChannelStatus = "disabled" // 禁用
)

// ChatChannel 客服 Web Widget 渠道（多通道设计）
//
// 设计理念（ADR-010）：
//   - 单部署实例可服务多个外部网站（多 channel）
//   - 每个 channel 对应一个 app_key + 一组白名单 origin
//   - channel 可绑定默认 RAG 产品（智能体）
//   - 私域部署：所有 channel 归属本实例，无 merchant_id
type ChatChannel struct {
	ID                  uint              `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID           string            `gorm:"type:varchar(50);uniqueIndex;not null" json:"channel_id"`        // 渠道 ID（UUID）
	ChannelName         string            `gorm:"type:varchar(100);not null" json:"channel_name"`                 // 渠道名称（如"官网首页"）
	AppKey              string            `gorm:"type:varchar(64);uniqueIndex;not null" json:"app_key"`           // 公开凭证（32 字符）
	AppSecretHash       string            `gorm:"type:varchar(128);not null" json:"-"`                            // 内部凭证（不返回前端）
	AllowedOrigins      string            `gorm:"type:text" json:"allowed_origins"`                               // 跨域白名单（逗号分隔，* 表示允许所有）
	DefaultRAGProductID uint              `gorm:"index" json:"default_rag_product_id"`                            // 默认 RAG 产品 ID
	WelcomeMessage      string            `gorm:"type:text" json:"welcome_message"`                               // 欢迎语
	WidgetColor         string            `gorm:"type:varchar(20);default:'#1989fa'" json:"widget_color"`         // 浮标颜色
	WidgetPosition      string            `gorm:"type:varchar(20);default:'bottom-right'" json:"widget_position"` // 浮标位置
	WidgetTitle         string            `gorm:"type:varchar(100);default:'在线客服'" json:"widget_title"`           // 聊天窗标题
	Status              ChatChannelStatus `gorm:"type:varchar(20);default:'active'" json:"status"`                // active/disabled
	VisitorCount        int64             `gorm:"default:0" json:"visitor_count"`                                 // 累计访客数
	SessionCount        int64             `gorm:"default:0" json:"session_count"`                                 // 累计会话数
	AutoAssign          bool              `gorm:"default:true" json:"auto_assign"`                                // 是否自动分配坐席
	ConfidenceThreshold float64           `gorm:"type:decimal(4,2);default:0.70" json:"confidence_threshold"`     // AI 自动回复阈值
	CreatedBy           uint              `json:"created_by"`                                                     // 创建人 user_id
	CreatedAt           time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time         `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (ChatChannel) TableName() string {
	return "chat_channels"
}

// IsActive 渠道是否启用
func (c *ChatChannel) IsActive() bool {
	return c.Status == ChatChannelStatusActive
}

// AllowedOriginsList 解析允许的 origin 列表
func (c *ChatChannel) AllowedOriginsList() []string {
	if c.AllowedOrigins == "" {
		return []string{}
	}
	result := []string{}
	current := ""
	for _, ch := range c.AllowedOrigins {
		if ch == ',' || ch == ';' || ch == ' ' || ch == '\n' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
