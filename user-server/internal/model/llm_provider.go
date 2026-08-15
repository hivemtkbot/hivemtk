package model

import "time"

// LLMProvider 持久化 LLM 模型/厂商定义。
//
// 此前 provider 定义只存在于内存（config 构建 + 后台「LLM路由」页面 AddProvider），
// 容器重启即丢失。改造后：后台页面 / 脚本 / config 引导注册的 provider 均落库到此表；
// user-server 启动时由 dispatcher.LoadProvidersFromDB 加载回内存路由表，
// 实现「可视化/脚本添加 → 落库 → 容器重启不丢」的闭环。
//
// Name 为全局唯一标识，与 dispatcher.ProviderConfig.Name 一一对应。
type LLMProvider struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"column:name;uniqueIndex;size:128" json:"name"` 
	DisplayName  string    `gorm:"column:display_name;size:255" json:"display_name"`
	BaseURL      string    `gorm:"column:base_url;size:512" json:"base_url"`
	Model        string    `gorm:"column:model;size:128" json:"model"`
	APIKey       string    `gorm:"column:api_key;type:text" json:"-"` 
	APIType      string    `gorm:"column:api_type;size:32;default:'openai'" json:"api_type"`
	Enabled      bool      `gorm:"column:enabled;default:false" json:"enabled"`
	QualityScore float64   `gorm:"column:quality_score;default:0.8" json:"quality_score"`
	MaxRPM       int       `gorm:"column:max_rpm;default:60" json:"max_rpm"`
	MaxTPM       int       `gorm:"column:max_tpm;default:0" json:"max_tpm"`
	CostPer1k    float64   `gorm:"column:cost_per_1k;default:0" json:"cost_per_1k"`
	AvgLatencyMs int       `gorm:"column:avg_latency_ms;default:0" json:"avg_latency_ms"`
	NoFC         bool      `gorm:"column:no_fc;default:false" json:"no_fc"`
	Vendor       string    `gorm:"column:vendor;size:64" json:"vendor"`
	Tags         string    `gorm:"column:tags;type:text" json:"-"` 
	SortOrder    int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (LLMProvider) TableName() string { return "llm_providers" }

