package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GeoKnowledgeDocument GEO 品牌知识库文档模型（迁移自 AIGEOTOOLS kb Document）
type GeoKnowledgeDocument struct {
	ID          string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Title       string `gorm:"type:varchar(500)" json:"title"`
	Content     string `gorm:"type:text" json:"content"`
	DocType     string `gorm:"type:varchar(50)" json:"doc_type"`
	SourceLevel string `gorm:"column:source_level;size:2;index" json:"source_level"`
	SourceURL   string `gorm:"column:source_url;size:512" json:"source_url"`
	Metadata    string `gorm:"type:text" json:"metadata"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoKnowledgeDocument) TableName() string {
	return "geo_knowledge_documents"
}

func (m *GeoKnowledgeDocument) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
