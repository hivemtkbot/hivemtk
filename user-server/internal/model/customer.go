package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UnifiedID Prefix constants
const (
	unifiedIDPrefixPhone       = "phone:"
	unifiedIDPrefixEmail       = "email:"
	unifiedIDPrefixWechat      = "wechat:"
	unifiedIDPrefixDouyin      = "douyin:"
	unifiedIDPrefixXiaohongshu = "xiaohongshu:"
)

// Customer 客户模型 - CDP 统一客户数据
type Customer struct {
	ID            string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	UnifiedID     string    `gorm:"type:varchar(64);uniqueIndex" json:"unified_id"`
	Phone         string    `gorm:"type:varchar(20);index" json:"phone"`
	Email         string    `gorm:"type:varchar(100);index" json:"email"`
	WechatOpenID  string    `gorm:"type:varchar(64);index" json:"wechat_open_id"`
	DouyinOpenID  string    `gorm:"type:varchar(64);index" json:"douyin_open_id"`
	XiaohongshuID string    `gorm:"type:varchar(64);index" json:"xiaohongshu_id"`
	Tags          string    `gorm:"type:text" json:"tags"` // JSON array of tag strings
	RFMScore      int       `gorm:"default:0" json:"rfm_score"`
	ChurnRisk     string    `gorm:"type:varchar(20);default:'low'" json:"churn_risk"` // low, medium, high
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 返回表名
func (Customer) TableName() string {
	return "customers"
}

// BeforeCreate 创建前钩子 - 自动生成 ID 和 UnifiedID
func (c *Customer) BeforeCreate(tx *gorm.DB) error {
	// 生成 ID
	if c.ID == "" {
		c.ID = uuid.New().String()
	}

	// 生成 UnifiedID（内联原 generateUnifiedID 逻辑）
	if c.UnifiedID == "" {
		c.UnifiedID = GenerateCustomerUnifiedID(c)
	}

	return nil
}

// GenerateCustomerUnifiedID 根据优先级生成统一 ID（包级函数）
// 优先级：Phone > Email > WechatOpenID > DouyinOpenID > XiaohongshuID
func GenerateCustomerUnifiedID(c *Customer) string {
	if c.Phone != "" {
		return unifiedIDPrefixPhone + c.Phone
	}
	if c.Email != "" {
		return unifiedIDPrefixEmail + c.Email
	}
	if c.WechatOpenID != "" {
		return unifiedIDPrefixWechat + c.WechatOpenID
	}
	if c.DouyinOpenID != "" {
		return unifiedIDPrefixDouyin + c.DouyinOpenID
	}
	if c.XiaohongshuID != "" {
		return unifiedIDPrefixXiaohongshu + c.XiaohongshuID
	}
	// 如果都没有，生成随机 UUID
	return uuid.New().String()
}

// GetCustomerTags 获取标签数组（包级函数）
func GetCustomerTags(c *Customer) []string {
	if c.Tags == "" {
		return []string{}
	}
	var tags []string
	err := json.Unmarshal([]byte(c.Tags), &tags)
	if err != nil {
		return []string{}
	}
	return tags
}

// SetCustomerTags 设置标签数组（包级函数）
func SetCustomerTags(c *Customer, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	data, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	c.Tags = string(data)
	return nil
}
