package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TagSource 标签来源常量
const (
	TagSourceAuto   TagSource = "auto"   
	TagSourceManual TagSource = "manual" 
)

// TagSource 标签来源
type TagSource string

// TagCategory 标签分类
type TagCategory string

// TagCategory 标签分类常量
const (
	TagCategoryDemographic   TagCategory = "demographic"   
	TagCategoryBehavioral    TagCategory = "behavioral"    
	TagCategoryTransactional TagCategory = "transactional" 
	TagCategoryPsychographic TagCategory = "psychographic" 
)

// CustomerTag 客户标签模型
type CustomerTag struct {
	ID        string      `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name      string      `gorm:"type:varchar(50);index;not null" json:"name"`
	Category  TagCategory `gorm:"type:varchar(32);index" json:"category"`
	Source    TagSource   `gorm:"type:varchar(20);not null" json:"source"`
	Rule      string      `gorm:"type:text" json:"rule"` 
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 返回表名
func (CustomerTag) TableName() string {
	return "customer_tags"
}

// BeforeCreate 创建前钩子 - 自动生成 ID
func (t *CustomerTag) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}

	return nil
}

