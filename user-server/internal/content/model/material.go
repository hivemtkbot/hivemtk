package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// MaterialType 素材类型
type MaterialType string

const (
	MaterialTypeImage MaterialType = "image"
	MaterialTypeVideo MaterialType = "video"
	MaterialTypeAudio MaterialType = "audio"
	MaterialTypeFile  MaterialType = "file"
)

// Material 素材模型
type Material struct {
	ID          string            `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string            `gorm:"type:varchar(255);not null" json:"name"`
	Type        MaterialType      `gorm:"type:varchar(20);not null" json:"type"`
	CategoryID  string            `gorm:"type:varchar(36)" json:"category_id"`
	Category    *MaterialCategory `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	URL         string            `gorm:"type:varchar(500);not null" json:"url"`
	Size        int64             `gorm:"not null" json:"size"`
	MimeType    string            `gorm:"type:varchar(100)" json:"mime_type"`
	Hash        string            `gorm:"type:varchar(64);index" json:"hash"`
	Width       int               `gorm:"" json:"width"`
	Height      int               `gorm:"" json:"height"`
	Duration    int               `gorm:"" json:"duration"` // 视频/音频时长（秒）
	Provider    string            `gorm:"type:varchar(50)" json:"provider"`
	StoragePath string            `gorm:"type:varchar(500)" json:"storage_path"`

	// 关联信息
	LicenseID string `gorm:"type:varchar(36);index" json:"license_id"`
	UserID    string `gorm:"type:varchar(36);index" json:"user_id"`

	// 使用统计
	UsageCount int        `gorm:"default:0" json:"usage_count"`
	LastUsedAt *time.Time `gorm:"" json:"last_used_at"`

	// 状态
	Status      string `gorm:"type:varchar(20);default:'active'" json:"status"`
	Tags        string `gorm:"type:text" json:"tags"`
	Description string `gorm:"type:text" json:"description"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *Material) TableName() string {
	return "materials"
}

func (m *Material) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GetTypeName 获取类型名称
func (m *Material) GetTypeName() string {
	switch m.Type {
	case MaterialTypeImage:
		return "图片"
	case MaterialTypeVideo:
		return "视频"
	case MaterialTypeAudio:
		return "音频"
	case MaterialTypeFile:
		return "文件"
	default:
		return "未知"
	}
}

// MaterialCategory 素材分类模型
type MaterialCategory struct {
	ID          string            `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string            `gorm:"type:varchar(100);not null" json:"name"`
	Type        MaterialType      `gorm:"type:varchar(20);not null" json:"type"`
	ParentID    *string           `gorm:"type:varchar(36);index" json:"parent_id"`
	Parent      *MaterialCategory `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Icon        string            `gorm:"type:varchar(100)" json:"icon"`
	Color       string            `gorm:"type:varchar(20)" json:"color"`
	Sort        int               `gorm:"default:0" json:"sort"`
	Description string            `gorm:"type:text" json:"description"`

	// 关联信息
	LicenseID string `gorm:"type:varchar(36);index" json:"license_id"`
	UserID    string `gorm:"type:varchar(36);index" json:"user_id"`

	// 统计信息
	MaterialCount int `gorm:"default:0" json:"material_count"`

	// 状态
	Status string `gorm:"type:varchar(20);default:'active'" json:"status"`

	Children  []MaterialCategory `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Materials []Material         `gorm:"foreignKey:CategoryID" json:"materials,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (c *MaterialCategory) TableName() string {
	return "material_categories"
}

func (c *MaterialCategory) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}
