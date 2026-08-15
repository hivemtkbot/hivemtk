package model

import "time"

// SystemConfigKV 单条 KV 配置
type SystemConfigKV struct {
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 表名
func (SystemConfigKV) TableName() string {
	return "system_config_kv"
}

