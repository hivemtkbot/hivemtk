package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// AIConfig JSON类型
type AIConfig map[string]any

func (c AIConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *AIConfig) Scan(value any) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), c)
}

// AIToolConfig AI工具配置模型
type AIToolConfig struct {
	ID               int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ToolName         string    `json:"tool_name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Category         string    `json:"category" gorm:"type:varchar(50);not null"`
	IsEnabled        bool      `json:"is_enabled" gorm:"default:true"`
	Config           AIConfig  `json:"config" gorm:"type:jsonb;serializer:json"`
	DefaultAccountID *string   `json:"default_account_id" gorm:"type:varchar(64)"`
	DefaultCardID    *string   `json:"default_card_id" gorm:"type:varchar(64)"`
	DisplayOrder     int       `json:"display_order" gorm:"default:0"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName 指定表名
func (AIToolConfig) TableName() string {
	return "ai_tool_configs"
}

// AIToolAccountBinding AI工具-账号绑定模型
type AIToolAccountBinding struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ToolName    string    `json:"tool_name" gorm:"type:varchar(100);not null;uniqueIndex:idx_tool_account"`
	AccountType string    `json:"account_type" gorm:"type:varchar(50);not null;uniqueIndex:idx_tool_account"`
	AccountID   string    `json:"account_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_tool_account"`
	IsPrimary   bool      `json:"is_primary" gorm:"default:false"`
	Config      AIConfig  `json:"config" gorm:"type:jsonb;serializer:json"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (AIToolAccountBinding) TableName() string {
	return "ai_tool_account_bindings"
}

// AIToolWithBinding 带绑定信息的工具
type AIToolWithBinding struct {
	AIToolConfig
	BoundAccounts []AIToolAccountBinding `json:"bound_accounts,omitempty"`
	Stats         *AIToolStats           `json:"stats,omitempty"`
}

// AIToolStats 工具统计
type AIToolStats struct {
	TotalCalls  int64   `json:"total_calls"`
	SuccessRate float64 `json:"success_rate"`
	AvgDuration int64   `json:"avg_duration_ms"`
	TodayCalls  int64   `json:"today_calls"`
}

// AIToolListResponse 工具列表响应
type AIToolListResponse struct {
	List  []AIToolWithBinding `json:"list"`
	Total int                 `json:"total"`
}

