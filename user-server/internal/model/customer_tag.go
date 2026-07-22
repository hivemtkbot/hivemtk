package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TagSource 标签来源常量
const (
	TagSourceAuto   TagSource = "auto"   // 系统自动打标签
	TagSourceManual TagSource = "manual" // 手动打标签
)

// TagSource 标签来源
type TagSource string

// TagCategory 标签分类
type TagCategory string

// TagCategory 标签分类常量
const (
	TagCategoryDemographic   TagCategory = "demographic"   // 人口统计学标签
	TagCategoryBehavioral    TagCategory = "behavioral"    // 行为标签
	TagCategoryTransactional TagCategory = "transactional" // 交易标签
	TagCategoryPsychographic TagCategory = "psychographic" // 心理/偏好标签
)

// CustomerTag 客户标签模型
type CustomerTag struct {
	ID        string      `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name      string      `gorm:"type:varchar(50);index;not null" json:"name"`
	Category  TagCategory `gorm:"type:varchar(32);index" json:"category"`
	Source    TagSource   `gorm:"type:varchar(20);not null" json:"source"`
	Rule      string      `gorm:"type:text" json:"rule"` // JSON string defining the tagging rule
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 返回表名
func (CustomerTag) TableName() string {
	return "customer_tags"
}

// BeforeCreate 创建前钩子 - 自动生成 ID
func (t *CustomerTag) BeforeCreate(tx *gorm.DB) error {
	// 生成 ID
	if t.ID == "" {
		t.ID = uuid.New().String()
	}

	return nil
}
