package service

import (
	"context"
	"encoding/json"

	"marketing/internal/repository"
)

// ============================================================================
// Agent Loop 运行期调参（数据库驱动，非环境变量）
//
// 包括单轮对话最多工具迭代次数（max_loop_iterations）、未配置白名单时注入工具数上限
// （max_tools）。这些属于“智能体运行期配置”，与工具依赖的外部集成凭证同样统一存数据库
// system_config_kv 表（key = agent.settings），而非环境变量，便于后台可视化调参与热更新。
//
// 数据库 system_config_kv 是唯一真相源；读取失败/缺配置时回退到代码内默认值
// （由 package 级变量 agentLoopMaxIterations / agentLoopMaxTools 提供，可被
// SetAgentLoopMaxIterations / SetAgentLoopMaxTools 注入覆盖，主要用于测试/内嵌场景）。
// ============================================================================

const (
	// kvAgentSettings Agent Loop 运行期调参在 system_config_kv 表中的 key。
	kvAgentSettings = "agent.settings"
)

// AgentSettingsConfig Agent Loop 运行期调参
type AgentSettingsConfig struct {
	MaxTools          int `json:"max_tools"`           // 默认 18，未配置白名单时注入工具数上限
	MaxLoopIterations int `json:"max_loop_iterations"` // 默认 5，单轮对话最多工具迭代次数（必须 >= 2）
}

// DefaultAgentSettingsConfig 返回当前代码内默认调参（尊重 SetAgentLoop* 注入值）。
func DefaultAgentSettingsConfig() *AgentSettingsConfig {
	return &AgentSettingsConfig{
		MaxTools:          agentLoopMaxTools,
		MaxLoopIterations: agentLoopMaxIterations,
	}
}

// LoadAgentSettingsConfig 从数据库 system_config_kv 读取 Agent Loop 运行期调参。
// 缺少配置行或 JSON 解析失败时返回默认调参（尊重 SetAgentLoop* 注入），不报错。
func LoadAgentSettingsConfig(ctx context.Context) (*AgentSettingsConfig, error) {
	cfg := DefaultAgentSettingsConfig()
	repo := repository.NewSystemConfigKVRepository()
	raw, err := repo.Get(ctx, kvAgentSettings)
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

// SaveAgentSettingsConfig 把 Agent Loop 运行期调参写入数据库 system_config_kv。
func SaveAgentSettingsConfig(ctx context.Context, cfg *AgentSettingsConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	repo := repository.NewSystemConfigKVRepository()
	_, err = repo.Upsert(ctx, kvAgentSettings, string(raw))
	return err
}
