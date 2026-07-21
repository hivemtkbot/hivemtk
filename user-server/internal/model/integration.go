package model

import (
	"time"
)

// IntegrationAccount 第三方对接账号
type IntegrationAccount struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform     string     `gorm:"type:varchar(50);index;not null" json:"platform"` // crm_xiaoshouyi, crm_fenxiangxiao, ecommerce_taobao, ecommerce_jd
	AccountName  string     `gorm:"type:varchar(100)" json:"account_name"`
	APIKey       string     `gorm:"type:varchar(200)" json:"api_key"`
	APISecret    string     `gorm:"type:varchar(200)" json:"api_secret"`
	RefreshToken string     `gorm:"type:text" json:"refresh_token"`
	AccessToken  string     `gorm:"type:text" json:"access_token"`
	TokenExpires *time.Time `json:"token_expires"`
	WebhookURL   string     `gorm:"type:varchar(500)" json:"webhook_url"`
	Config       string     `gorm:"type:text" json:"config"` // JSON config
	Status       int        `gorm:"default:1" json:"status"` // 1-启用 0-禁用
	LastSyncAt   *time.Time `json:"last_sync_at"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (IntegrationAccount) TableName() string {
	return "integration_accounts"
}

// SyncLog 同步日志
type SyncLog struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform     string     `gorm:"type:varchar(50);index" json:"platform"`
	SyncType     string     `gorm:"type:varchar(50)" json:"sync_type"` // customer, order, product, etc.
	Status       int        `gorm:"default:0" json:"status"`           // 0-进行中 1-成功 2-失败
	RecordCount  int        `gorm:"default:0" json:"record_count"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	StartTime    time.Time  `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	CreatedAt    time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (SyncLog) TableName() string {
	return "sync_logs"
}

// ExternalCustomer 外部客户（CRM 对接）
type ExternalCustomer struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform      string     `gorm:"type:varchar(50);index" json:"platform"`
	ExternalID    string     `gorm:"type:varchar(100);index" json:"external_id"` // 外部系统客户 ID
	Name          string     `gorm:"type:varchar(100)" json:"name"`
	Phone         string     `gorm:"type:varchar(50);index" json:"phone"`
	Email         string     `gorm:"type:varchar(100)" json:"email"`
	Company       string     `gorm:"type:varchar(200)" json:"company"`
	Position      string     `gorm:"type:varchar(100)" json:"position"`
	Industry      string     `gorm:"type:varchar(100)" json:"industry"`
	Level         string     `gorm:"type:varchar(50)" json:"level"` // 客户级别
	Source        string     `gorm:"type:varchar(100)" json:"source"`
	OwnerID       string     `gorm:"type:varchar(100)" json:"owner_id"` // 负责人 ID
	OwnerName     string     `gorm:"type:varchar(100)" json:"owner_name"`
	Status        string     `gorm:"type:varchar(50)" json:"status"` // 潜在客户、意向客户、成交客户等
	Tags          string     `gorm:"type:text" json:"tags"`          // JSON 数组
	LastContactAt *time.Time `json:"last_contact_at"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ExternalCustomer) TableName() string {
	return "external_customers"
}

// ExternalOrder 外部订单（电商对接）
type ExternalOrder struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform       string     `gorm:"type:varchar(50);index" json:"platform"`
	OrderID        string     `gorm:"type:varchar(100);unique;not null" json:"order_id"` // 外部订单号
	OrderNo        string     `gorm:"type:varchar(100);index" json:"order_no"`           // 内部订单号
	UserID         string     `gorm:"type:varchar(100);index" json:"user_id"`            // 用户 ID
	UserName       string     `gorm:"type:varchar(100)" json:"user_name"`
	UserPhone      string     `gorm:"type:varchar(50)" json:"user_phone"`
	TotalAmount    int64      `gorm:"type:bigint;default:0" json:"total_amount"`    // 订单总金额（分）
	PayAmount      int64      `gorm:"type:bigint;default:0" json:"pay_amount"`      // 实付金额（分）
	DiscountAmount int64      `gorm:"type:bigint;default:0" json:"discount_amount"` // 折扣金额（分）
	Status         string     `gorm:"type:varchar(50)" json:"status"`               // 待付款、已付款、发货中、已完成、已取消
	PayTime        *time.Time `json:"pay_time"`
	ShipTime       *time.Time `json:"ship_time"`
	CompleteTime   *time.Time `json:"complete_time"`
	Items          string     `gorm:"type:text" json:"items"` // JSON 数组
	ShippingAddr   string     `gorm:"type:text" json:"shipping_addr"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ExternalOrder) TableName() string {
	return "external_orders"
}

// ExternalProduct 外部商品（电商对接）
type ExternalProduct struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform      string    `gorm:"type:varchar(50);index" json:"platform"`
	ProductID     string    `gorm:"type:varchar(100);index" json:"product_id"`
	Name          string    `gorm:"type:varchar(200)" json:"name"`
	CategoryID    string    `gorm:"type:varchar(100)" json:"category_id"`
	CategoryName  string    `gorm:"type:varchar(100)" json:"category_name"`
	Price         int64     `gorm:"type:bigint;default:0" json:"price"`          // 商品价格（分）
	OriginalPrice int64     `gorm:"type:bigint;default:0" json:"original_price"` // 商品原价（分）
	Stock         int       `gorm:"default:0" json:"stock"`
	Sales         int       `gorm:"default:0" json:"sales"`  // 销量
	Images        string    `gorm:"type:text" json:"images"` // JSON 数组
	Status        int       `gorm:"default:1" json:"status"` // 1-上架 0-下架
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (ExternalProduct) TableName() string {
	return "external_products"
}

// WebhookEvent Webhook 事件
type WebhookEvent struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform    string     `gorm:"type:varchar(50);index" json:"platform"`
	EventID     string     `gorm:"type:varchar(100);unique" json:"event_id"`
	EventType   string     `gorm:"type:varchar(50)" json:"event_type"` // customer.created, order.paid, etc.
	RawData     string     `gorm:"type:text" json:"raw_data"`
	Processed   bool       `gorm:"default:false" json:"processed"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (WebhookEvent) TableName() string {
	return "webhook_events"
}
