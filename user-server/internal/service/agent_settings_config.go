package service

import (
	"context"
	"encoding/json"

	"hivemtk-user/internal/repository"
)

const (
	kvAgentSettings = "agent.settings"
)

// AgentSettingsConfig Agent Loop 运行期调参
type AgentSettingsConfig struct {
	MaxTools          int `json:"max_tools"`
	MaxLoopIterations int `json:"max_loop_iterations"`
	// DisabledTools 租户级工具启停（TL-3）：列出的工具名在装配处被剔除，
	// 对所有场景不可见；为空/未配置时全量启用（向后兼容）
	DisabledTools []string `json:"disabled_tools,omitempty"`
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
