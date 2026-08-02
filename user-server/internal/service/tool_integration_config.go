package service

import (
	"context"
	"encoding/json"

	"marketing/internal/repository"
)

// ============================================================================
// 工具依赖的外部集成配置（数据库驱动，非环境变量）
//
// 客服 Agent 的部分工具依赖外部系统（实时快递轨迹、售后回写电商），其凭证/基地址
// 属于“工具依赖的配置”，统一存数据库 system_config_kv 表（key = agent.tool_integrations），
// 而非环境变量。原因：
//   - 多租户/多电商场景下凭证随环境变化，数据库可按需写入、后台可视化编辑；
//   - 凭证属于敏感配置，入库存放比散落在 .env / 进程环境更易管控与审计；
//   - 读取发生在工具执行的请求路径上，保存后立即对新请求生效（无需重启）。
//
// 数据库 system_config_kv 是唯一真相源；读取失败/缺配置时降级为“未启用”，
// 保证工具始终可用，不会因配置缺失而中断对话。
// ============================================================================

const (
	// kvToolIntegrationConfig 工具集成配置在 system_config_kv 表中的 key。
	kvToolIntegrationConfig = "agent.tool_integrations"
)

// LogisticsIntegration 实时快递轨迹接口配置（工具依赖）。
type LogisticsIntegration struct {
	Enabled  bool   `json:"enabled"`   // 是否启用实时快递轨迹
	Provider string `json:"provider"`  // 聚合平台标识，如 kuaidi100 / 自建
	BaseURL  string `json:"base_url"`  // 实时快递接口基地址（如 https://kuaidi.example.com）
	Key      string `json:"key"`       // 接口 Key（Bearer Token，可选）
	Secret   string `json:"secret"`    // 接口密钥（X-Api-Secret 头，可选）
}

// AfterSaleIntegration 售后回写电商平台接口配置（工具依赖）。
type AfterSaleIntegration struct {
	Enabled bool   `json:"enabled"`  // 是否启用售后回写电商
	BaseURL string `json:"base_url"` // 电商售后接口基地址（如 https://mall.example.com）
	Key     string `json:"key"`      // 接口 Key（Bearer Token，可选）
	Secret  string `json:"secret"`   // 接口密钥（X-Api-Secret 头，可选）
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
