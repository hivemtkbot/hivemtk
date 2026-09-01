package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// GeoCompetitor 竞品网站配置
type GeoCompetitor struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string         `gorm:"type:varchar(100);not null" json:"name"`
	Domain    string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"domain"`
	Paths     datatypes.JSON `gorm:"column:paths;type:jsonb" json:"paths"` // ["","/product","/pricing"]
	Category  string         `gorm:"type:varchar(50);index" json:"category"`
	Priority  int            `gorm:"default:5" json:"priority"`
	Status    string         `gorm:"type:varchar(20);default:active;index" json:"status"`
	Notes     string         `gorm:"type:text" json:"notes"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoCompetitor) TableName() string { return "geo_competitors" }
