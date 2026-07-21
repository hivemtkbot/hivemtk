package model

import (
	"time"
)

// WeComAccount 企业微信账号
type WeComAccount struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	CorpID        string     `gorm:"type:varchar(100);not null" json:"corp_id"`             // 企业 ID
	CorpSecret    string     `gorm:"type:varchar(200);not null" json:"corp_secret"`         // 企业密钥
	AgentID       int        `json:"agent_id"`                                              // 应用 ID
	AgentSecret   string     `gorm:"type:varchar(200)" json:"agent_secret"`                 // 应用密钥
	AccessToken   string     `gorm:"type:text" json:"access_token"`                         // 访问令牌
	TokenExpires  time.Time  `json:"token_expires"`                                         // 令牌过期时间
	Status        int        `gorm:"default:1" json:"status"`                               // 状态 1-启用 0-禁用
	LoginState    string     `gorm:"type:varchar(20);default:'offline'" json:"login_state"` // online/offline/banned
	FriendCount   int        `gorm:"default:0" json:"friend_count"`
	GroupCount    int        `gorm:"default:0" json:"group_count"`
	DailyMsgQuota int        `gorm:"default:500" json:"daily_msg_quota"`
	DailyMsgUsed  int        `gorm:"default:0" json:"daily_msg_used"`
	QuotaResetAt  *time.Time `json:"quota_reset_at"`
	RiskLevel     string     `gorm:"type:varchar(20);default:'normal'" json:"risk_level"` // normal/warning/banned
	RiskMessage   string     `gorm:"type:text" json:"risk_message"`
	Weight        int        `gorm:"default:100" json:"weight"` // 路由权重 0-100
	LastSyncAt    *time.Time `json:"last_sync_at"`              // 最后同步时间
	LastActiveAt  *time.Time `json:"last_active_at"`            // 最后活跃时间
	LastErrorAt   *time.Time `json:"last_error_at"`
	LastErrorMsg  string     `gorm:"type:text" json:"last_error_msg"`
	TotalSent     int64      `gorm:"default:0" json:"total_sent"`
	TotalReceived int64      `gorm:"default:0" json:"total_received"`
	ErrorCount    int        `gorm:"default:0" json:"error_count"`
	// Phase1 新增：用于接收企微回调（webhook 验签 + 加解密）
	CallbackToken  string    `gorm:"type:varchar(100)" json:"callback_token"`   // 回调 Token（企微管理端"接收事件服务器"配置）
	EncodingAESKey string    `gorm:"type:varchar(200)" json:"encoding_aes_key"` // 43 字符 EncodingAESKey（用于消息解密）
	WebhookEnabled bool      `gorm:"default:false" json:"webhook_enabled"`      // 是否启用 webhook 接收
	WebhookPath    string    `gorm:"type:varchar(200)" json:"webhook_path"`     // 自定义回调路径（默认 /api/webhook/wecom/{id}）
	AIAgentEnabled bool      `gorm:"default:false" json:"ai_agent_enabled"`     // 是否启用 智能体自动回复
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (WeComAccount) TableName() string {
	return "wecom_accounts"
}

// WeComCustomer 企业微信客户
type WeComCustomer struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID      uint      `gorm:"index" json:"account_id"`
	ExternalUserID string    `gorm:"type:varchar(100);index;not null" json:"external_user_id"` // 外部联系人 ID
	Name           string    `gorm:"type:varchar(100)" json:"name"`
	Nickname       string    `gorm:"type:varchar(100)" json:"nickname"`
	Avatar         string    `gorm:"type:varchar(500)" json:"avatar"`
	Gender         int       `json:"gender"` // 0-未知 1-男 2-女
	Type           int       `json:"type"`   // 0-微信 1-企业微信
	UnionID        string    `gorm:"type:varchar(100)" json:"union_id"`
	EmployeeID     string    `gorm:"type:varchar(100);index" json:"employee_id"` // 跟进员工 ID
	EmployeeName   string    `gorm:"type:varchar(100)" json:"employee_name"`
	AddTime        time.Time `json:"add_time"`
	Source         string    `gorm:"type:varchar(50)" json:"source"` // 来源
	Tags           string    `gorm:"type:text" json:"tags"`          // 标签 JSON 数组
	Remark         string    `gorm:"type:varchar(500)" json:"remark"`
	Description    string    `gorm:"type:text" json:"description"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (WeComCustomer) TableName() string {
	return "wecom_customers"
}

// WeComGroup 企业微信客户群
type WeComGroup struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID   uint      `gorm:"index" json:"account_id"`
	ChatID      string    `gorm:"type:varchar(100);unique;not null" json:"chat_id"`
	Name        string    `gorm:"type:varchar(100)" json:"name"`
	OwnerID     string    `gorm:"type:varchar(100);index" json:"owner_id"` // 群主 ID
	OwnerName   string    `gorm:"type:varchar(100)" json:"owner_name"`
	MemberCount int       `gorm:"default:0" json:"member_count"`
	MemberLimit int       `gorm:"default:500" json:"member_limit"`
	Status      int       `gorm:"default:1" json:"status"` // 1-正常 0-已解散
	CreateTime  time.Time `json:"create_time"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (WeComGroup) TableName() string {
	return "wecom_groups"
}

// WeComGroupMember 企业微信客户群成员
type WeComGroupMember struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupID   uint      `gorm:"index;not null" json:"group_id"`
	ChatID    string    `gorm:"type:varchar(100);index" json:"chat_id"`
	UserID    string    `gorm:"type:varchar(100);index" json:"user_id"`
	UserName  string    `gorm:"type:varchar(100)" json:"user_name"`
	JoinTime  time.Time `json:"join_time"`
	Type      int       `json:"type"` // 1-微信用户 2-企业微信用户
	IsOwner   bool      `gorm:"default:false" json:"is_owner"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (WeComGroupMember) TableName() string {
	return "wecom_group_members"
}

// WeComMessage 企业微信消息记录
type WeComMessage struct {
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID uint       `gorm:"index" json:"account_id"`
	MsgID     string     `gorm:"type:varchar(100);unique" json:"msg_id"`
	MsgType   string     `gorm:"type:varchar(20)" json:"msg_type"` // text, image, video, file, link, mpnews,小程序
	ToUser    string     `gorm:"type:text" json:"to_user"`         // 接收用户 ID
	ToParty   string     `gorm:"type:text" json:"to_party"`        // 接收部门 ID
	ToTag     string     `gorm:"type:text" json:"to_tag"`          // 接收标签 ID
	Content   string     `gorm:"type:text" json:"content"`
	MediaID   string     `gorm:"type:varchar(200)" json:"media_id"`
	Status    int        `gorm:"default:0" json:"status"` // 0-待发送 1-发送成功 2-发送失败
	SendTime  *time.Time `json:"send_time"`
	ErrorMsg  string     `gorm:"type:text" json:"error_msg"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (WeComMessage) TableName() string {
	return "wecom_messages"
}

// WeComTag 企业微信标签
type WeComTag struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID     uint      `gorm:"index" json:"account_id"`
	TagID         string    `gorm:"type:varchar(100);not null" json:"tag_id"`
	TagName       string    `gorm:"type:varchar(50);not null" json:"tag_name"`
	GroupID       string    `gorm:"type:varchar(100)" json:"group_id"` // 标签组 ID
	GroupName     string    `gorm:"type:varchar(50)" json:"group_name"`
	CustomerCount int       `gorm:"default:0" json:"customer_count"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (WeComTag) TableName() string {
	return "wecom_tags"
}
