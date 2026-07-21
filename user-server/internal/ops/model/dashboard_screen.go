package model

import (
	"time"
)

// DashboardScreen 数据大屏
type DashboardScreen struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Code      string    `gorm:"type:varchar(50);unique" json:"code"` // 大屏编码，用于访问
	Layout    string    `json:"layout"`                              // 布局配置 (JSON)
	Widgets   string    `json:"widgets"`                             //  widget 配置 (JSON)
	Theme     string    `gorm:"type:varchar(20);default:'dark'" json:"theme"`
	IsPublic  bool      `gorm:"default:false" json:"is_public"`
	ViewCount int       `gorm:"default:0" json:"view_count"`
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (DashboardScreen) TableName() string {
	return "dashboard_screens"
}

// DashboardWidget Widget 定义
type DashboardWidget struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ScreenID   uint      `gorm:"index;not null" json:"screen_id"`
	WidgetType string    `gorm:"type:varchar(50);not null" json:"widget_type"` // chart, table, indicator, map
	Title      string    `gorm:"type:varchar(100)" json:"title"`
	Config     string    `gorm:"type:text" json:"config"` // Widget 配置 (JSON)
	DataSource string    `gorm:"type:varchar(50)" json:"data_source"`
	X          int       `gorm:"default:0" json:"x"`
	Y          int       `gorm:"default:0" json:"y"`
	Width      int       `gorm:"default:4" json:"width"`
	Height     int       `gorm:"default:3" json:"height"`
	SortOrder  int       `gorm:"default:0" json:"sort_order"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (DashboardWidget) TableName() string {
	return "dashboard_widgets"
}
