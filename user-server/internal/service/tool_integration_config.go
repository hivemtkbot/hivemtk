package service

import (
	"context"
	"encoding/json"

	"hivemtk-user/internal/repository"
)


const (
	kvToolIntegrationConfig = "agent.tool_integrations"
)

// LogisticsIntegration 实时快递轨迹接口配置（工具依赖）。
type LogisticsIntegration struct {
	Enabled  bool   `json:"enabled"`  
	Provider string `json:"provider"` 
	BaseURL  string `json:"base_url"` 
	Key      string `json:"key"`      
	Secret   string `json:"secret"`   
}

// AfterSaleIntegration 售后回写电商平台接口配置（工具依赖）。
type AfterSaleIntegration struct {
	Enabled bool   `json:"enabled"`  
	BaseURL string `json:"base_url"` 
	Key     string `json:"key"`      
	Secret  string `json:"secret"`   
}

// ToolIntegrationConfig 工具依赖的外部集成配置聚合体，整体作为 JSON 存入 system_config_kv。
type ToolIntegrationConfig struct {
	Logistics LogisticsIntegration `json:"logistics"`
	AfterSale AfterSaleIntegration `json:"after_sale"`
}

// DefaultToolIntegrationConfig 返回全部未启用的默认配置（即未配置任何外部集成）。
func DefaultToolIntegrationConfig() *ToolIntegrationConfig {
	return &ToolIntegrationConfig{
		Logistics: LogisticsIntegration{Enabled: false},
		AfterSale: AfterSaleIntegration{Enabled: false},
	}
}

// LoadToolIntegrationConfig 从数据库 system_config_kv 读取工具集成配置（数据库为唯一真相源）。
// 缺少配置行或 JSON 解析失败时返回未启用的默认配置，不报错，保证工具降级可用。
func LoadToolIntegrationConfig(ctx context.Context) (*ToolIntegrationConfig, error) {
	cfg := DefaultToolIntegrationConfig()
	repo := repository.NewSystemConfigKVRepository()
	raw, err := repo.Get(ctx, kvToolIntegrationConfig)
	if err != nil {
		return cfg, err
	}
	if raw == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveToolIntegrationConfig 把工具集成配置写入数据库 system_config_kv。
func SaveToolIntegrationConfig(ctx context.Context, cfg *ToolIntegrationConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	repo := repository.NewSystemConfigKVRepository()
	_, err = repo.Upsert(ctx, kvToolIntegrationConfig, string(raw))
	return err
}

