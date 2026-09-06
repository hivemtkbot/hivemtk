package service

import (
	"context"
	"encoding/json"
	"errors"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/secrets"
)

const (
	kvToolIntegrationConfig = "agent.tool_integrations"
)

type LogisticsIntegration struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Key      string `json:"key"`
	Secret   string `json:"secret"`
}

type AfterSaleIntegration struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	Key     string `json:"key"`
	Secret  string `json:"secret"`
}

// ToolIntegrationConfig 工具依赖的外部集成配置聚合体，整体作为 JSON 存入 system_config_kv。
// 凭证字段（Logistics.Key/Secret, AfterSale.Key/Secret）读写时通过 secrets 包透明加解密。
type ToolIntegrationConfig struct {
	Logistics LogisticsIntegration `json:"logistics"`
	AfterSale AfterSaleIntegration `json:"after_sale"`
}

func DefaultToolIntegrationConfig() *ToolIntegrationConfig {
	return &ToolIntegrationConfig{
		Logistics: LogisticsIntegration{Enabled: false},
		AfterSale: AfterSaleIntegration{Enabled: false},
	}
}

func (c *ToolIntegrationConfig) credentialFields() []*string {
	return []*string{
		&c.Logistics.Key,
		&c.Logistics.Secret,
		&c.AfterSale.Key,
		&c.AfterSale.Secret,
	}
}

func (c *ToolIntegrationConfig) hasPlaintextCredentials() bool {
	for _, p := range c.credentialFields() {
		if *p != "" && !secrets.IsCiphertextFormat(*p) {
			return true
		}
	}
	return false
}

// LoadToolIntegrationConfig 从数据库读取工具集成配置，凭证字段透明解密。
//
// 密钥三态兼容（TL-1 决策）：
//   - MASTER_KEY 可用：IsCiphertextFormat 判定为密文 → secrets.DecryptString 解密；
//     判定为明文 → 原样返回并 WARN（透明升级重写）；
//   - MASTER_KEY 缺失：secrets.Ready()=false，全部按明文处理并输出启动 WARN；
//   - 解密失败：回退明文 + ERROR 日志（向后兼容存量明文，不阻塞读取）。
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

	if !secrets.Ready() {
		logger.Warnf("[tool_integration] MASTER_KEY 未配置或密钥不足32字节，凭证字段以明文读取（建议配置 MASTER_KEY 启用加密存储）")
		return cfg, nil
	}

	for _, p := range cfg.credentialFields() {
		if *p == "" {
			continue
		}
		plain, derr := secrets.DecryptString(*p)
		if derr != nil {
			logger.Errorf("[tool_integration] 凭证解密失败（密钥不匹配或密文损坏），回退明文: %v", derr)
			continue
		}
		*p = plain
	}

	if cfg.hasPlaintextCredentials() {
		logger.Warnf("[tool_integration] 检测到明文凭据，透明升级为 AES-256-GCM 加密存储")
		if uerr := saveEncryptedToolIntegrationConfig(ctx, repo, cfg); uerr != nil {
			logger.Warnf("[tool_integration] 明文凭据透明升级失败（下次写入重试）: %v", uerr)
		} else {
			logger.Debugf("[tool_integration] 明文凭据已透明升级为加密存储")
		}
	}
	return cfg, nil
}

// SaveToolIntegrationConfig 写入工具集成配置。MASTER_KEY 可用时加密凭证字段，否则写明文 + WARN。
func SaveToolIntegrationConfig(ctx context.Context, cfg *ToolIntegrationConfig) error {
	repo := repository.NewSystemConfigKVRepository()
	return saveEncryptedToolIntegrationConfig(ctx, repo, cfg)
}

func saveEncryptedToolIntegrationConfig(ctx context.Context, repo repository.SystemConfigKVRepository, cfg *ToolIntegrationConfig) error {
	store := *cfg
	if secrets.Ready() {
		if err := encryptCredentialFields(&store); err != nil {
			return err
		}
	} else {
		logger.Warnf("[tool_integration] MASTER_KEY 未配置，凭证字段以明文落库（建议配置启用加密存储）")
	}
	raw, err := json.Marshal(&store)
	if err != nil {
		return err
	}
	_, err = repo.Upsert(ctx, kvToolIntegrationConfig, string(raw))
	return err
}

func encryptCredentialFields(cfg *ToolIntegrationConfig) error {
	for _, p := range cfg.credentialFields() {
		if *p == "" || secrets.IsCiphertextFormat(*p) {
			continue
		}
		e, err := secrets.EncryptString(*p)
		if err != nil {
			if errors.Is(err, secrets.ErrMasterKeyMissing) {
				return err
			}
			return err
		}
		*p = e
	}
	return nil
}
