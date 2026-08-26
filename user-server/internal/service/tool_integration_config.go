package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)


const (
	kvToolIntegrationConfig = "agent.tool_integrations"

	// masterKeyEnv 环境变量名，提供时对 provider key/secret 做 AES-256-GCM 加密存储。
	masterKeyEnv = "MASTER_KEY"

	// encFieldPrefix 加密字段值前缀：enc:v1:{base64(nonce|ciphertext)}
	encFieldPrefix = "enc:v1:"

	// gcmNonceSize AES-GCM 标准 nonce 长度
	gcmNonceSize = 12
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

// deriveAESKeyFromMaster 从 MASTER_KEY 派生 32 字节 AES-256 密钥（sha256(masterKey)）。
// 未设置 MASTER_KEY 时返回 (nil, false)，调用方降级为明文读写（向后兼容，绝不报错中断服务）。
func deriveAESKeyFromMaster() ([]byte, bool) {
	mk := os.Getenv(masterKeyEnv)
	if mk == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(mk))
	return sum[:], true
}

// encryptField 使用 AES-256-GCM 加密字段值，输出格式 `enc:v1:{base64(nonce|ciphertext)}`。
// MASTER_KEY 未设置时原样返回明文（降级模式）。
func encryptField(plaintext string) (string, error) {
	key, ok := deriveAESKeyFromMaster()
	if !ok {
		return plaintext, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("tool_integration: init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("tool_integration: init gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("tool_integration: gen nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := make([]byte, 0, len(nonce)+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	return encFieldPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// decryptField 解密 `enc:v1:` 前缀的加密值；非该前缀的明文原样返回。
// 密文被篡改或损坏时返回错误。
func decryptField(value string) (string, error) {
	if !strings.HasPrefix(value, encFieldPrefix) {
		return value, nil
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encFieldPrefix))
	if err != nil {
		return "", fmt.Errorf("tool_integration: decode encrypted field: %w", err)
	}
	key, ok := deriveAESKeyFromMaster()
	if !ok {
		return "", errors.New("tool_integration: encrypted field found but MASTER_KEY not set")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("tool_integration: init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("tool_integration: init gcm: %w", err)
	}
	if len(payload) < gcm.NonceSize()+1 {
		return "", errors.New("tool_integration: encrypted payload too short")
	}
	nonce, ciphertext := payload[:gcmNonceSize], payload[gcmNonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("tool_integration: decrypt field failed (tampered or wrong key): %w", err)
	}
	return string(plain), nil
}

// isEncryptedField 判断值是否为加密格式
func isEncryptedField(v string) bool {
	return strings.HasPrefix(v, encFieldPrefix)
}

// decryptConfigFields 就地解密 cfg 中所有 provider key/secret 字段（enc: 前缀 → 明文）。
func decryptConfigFields(cfg *ToolIntegrationConfig) error {
	for _, p := range []*string{&cfg.Logistics.Key, &cfg.Logistics.Secret, &cfg.AfterSale.Key, &cfg.AfterSale.Secret} {
		if !isEncryptedField(*p) {
			continue
		}
		plain, err := decryptField(*p)
		if err != nil {
			return err
		}
		*p = plain
	}
	return nil
}

// hasPlaintextSecretFields 判断是否存在非空的明文 key/secret 字段（需透明升级）。
func hasPlaintextSecretFields(cfg *ToolIntegrationConfig) bool {
	for _, v := range []string{cfg.Logistics.Key, cfg.Logistics.Secret, cfg.AfterSale.Key, cfg.AfterSale.Secret} {
		if v != "" && !isEncryptedField(v) {
			return true
		}
	}
	return false
}

// LoadToolIntegrationConfig 从数据库 system_config_kv 读取工具集成配置（数据库为唯一真相源）。
// 缺少配置行或 JSON 解析失败时返回未启用的默认配置，不报错，保证工具降级可用。
//
// 密钥三态兼容：
//   - enc:v1: 前缀 → 解密返回；
//   - 明文 → 照常返回；若 MASTER_KEY 可用则透明升级重写为密文存储（WARN 日志）；
//   - MASTER_KEY 未设置 → 全部按明文处理并输出启动 WARN（向后兼容）。
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
	if _, hasKey := deriveAESKeyFromMaster(); !hasKey {
		logger.Warnf("[tool_integration] MASTER_KEY 未设置，provider key/secret 将以明文读写（建议配置以启用加密存储）")
	}
	if err := decryptConfigFields(cfg); err != nil {
		logger.Warnf("[tool_integration] 解密集成配置失败（密钥不匹配或密文损坏），相关凭据不可用: %v", err)
		return cfg, err
	}
	// 透明升级：检测到明文凭据且主密钥可用 → 重写为密文存储
	if _, hasKey := deriveAESKeyFromMaster(); hasKey && hasPlaintextSecretFields(cfg) {
		logger.Warnf("[tool_integration] 检测到明文 provider key/secret，正在透明升级为 AES-256-GCM 加密存储")
		if uerr := saveEncryptedToolIntegrationConfig(ctx, repo, cfg); uerr != nil {
			// 升级失败不影响本次读取，仅告警，下次 Save/Load 再尝试
			logger.Warnf("[tool_integration] 明文凭据透明升级失败（将在下次写入时重试）: %v", uerr)
		} else {
			logger.Debugf("[tool_integration] 明文凭据已透明升级为加密存储")
		}
	}
	return cfg, nil
}

// SaveToolIntegrationConfig 把工具集成配置写入数据库 system_config_kv。
// MASTER_KEY 存在时对 key/secret 字段加密存储；未设置时降级为明文（向后兼容）。
func SaveToolIntegrationConfig(ctx context.Context, cfg *ToolIntegrationConfig) error {
	repo := repository.NewSystemConfigKVRepository()
	return saveEncryptedToolIntegrationConfig(ctx, repo, cfg)
}

// saveEncryptedToolIntegrationConfig 序列化并落库，写库的是加密后的副本，不改入参 cfg。
func saveEncryptedToolIntegrationConfig(ctx context.Context, repo repository.SystemConfigKVRepository, cfg *ToolIntegrationConfig) error {
	store := *cfg
	if _, hasKey := deriveAESKeyFromMaster(); hasKey {
		enc, err := encryptConfigFields(&store)
		if err != nil {
			return err
		}
		store = *enc
	}
	raw, err := json.Marshal(&store)
	if err != nil {
		return err
	}
	_, err = repo.Upsert(ctx, kvToolIntegrationConfig, string(raw))
	return err
}

// encryptConfigFields 返回 cfg 的副本，其中非空 key/secret 字段已加密。
func encryptConfigFields(cfg *ToolIntegrationConfig) (*ToolIntegrationConfig, error) {
	out := *cfg
	for _, p := range []*string{&out.Logistics.Key, &out.Logistics.Secret, &out.AfterSale.Key, &out.AfterSale.Secret} {
		if *p == "" || isEncryptedField(*p) {
			continue
		}
		e, err := encryptField(*p)
		if err != nil {
			return nil, err
		}
		*p = e
	}
	return &out, nil
}
