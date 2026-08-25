package model

import (
	"database/sql/driver"
	"encoding/json"
)

// LeadMiningConfig 线索发掘全局配置（单例，ID 固定为 1）
type LeadMiningConfig struct {
	ID             int64       `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	Enabled        bool        `gorm:"column:enabled;default:false" json:"enabled"`
	Keywords       JSONStrings `gorm:"column:keywords;type:text" json:"keywords"`       
	Tags           JSONStrings `gorm:"column:tags;type:text" json:"tags"`               
	Requirement    string      `gorm:"column:requirement;type:text" json:"requirement"` 
	Channels       JSONStrings `gorm:"column:channels;type:text" json:"channels"`       
	MinIntentScore int         `gorm:"column:min_intent_score;default:50" json:"min_intent_score"`
	Model          string      `gorm:"column:model;type:varchar(64)" json:"model"` 
	CreatedAt      int64       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      int64       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (m *LeadMiningConfig) TableName() string { return "lead_mining_configs" }

// JSONStrings 可存 JSON 数组字符串的字段类型
type JSONStrings []string

func (j JSONStrings) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (j *JSONStrings) Scan(v any) error {
	if v == nil {
		*j = JSONStrings{}
		return nil
	}
	var data []byte
	switch t := v.(type) {
	case string:
		data = []byte(t)
	case []byte:
		data = t
	default:
		*j = JSONStrings{}
		return nil
	}
	if len(data) == 0 {
		*j = JSONStrings{}
		return nil
	}
	return json.Unmarshal(data, j)
}

var _ driver.Valuer = (JSONStrings)(nil)

