package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PlatformConfig 平台上报配置（开源版精简版）
//
// 背景：hivemtk 全面开源后不再有 License / 授权同步，
// 这里只保留"商户上报"用到的配置（APIURL / Secret / Admin*）。
// 兼容老字段（LogReportInterval / LicenseSyncInterval）仅作占位，
// 读不到也允许（给前端 / 监控保持 schema 兼容）。
type PlatformConfig struct {
	APIURL string `yaml:"api_url" json:"api_url"`
	Secret string `yaml:"secret" json:"secret"`
	AdminUsername string `yaml:"admin_username" json:"admin_username"`
	AdminPassword string `yaml:"admin_password" json:"admin_password"`
	LogReportInterval int `yaml:"log_report_interval" json:"log_report_interval"`
	LicenseSyncInterval int `yaml:"license_sync_interval" json:"license_sync_interval"`
}

// PlatformCfg 全局平台配置（运行时唯一实例）
//
// 开源版：仅承载"上报地址 + 签名密钥 + 平台管理员账号"，
// 不再做 License 授权同步 / 续期 / 校验。
var PlatformCfg *PlatformConfig

// LoadPlatform 从 YAML 文件加载平台配置
//
// 失败策略：
//   - 必填字段缺失（api_url / secret / admin_password）→ 返回错误
//   - 可选字段缺失 → 使用 0 值
//
// 路径：
//   - 默认 config/platform.yaml
//   - 可通过环境变量 PLATFORM_CONFIG_PATH 覆盖
func LoadPlatform(path string) error {
	if p := os.Getenv("PLATFORM_CONFIG_PATH"); p != "" {
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取平台配置失败 (%s): %w", path, err)
	}
	expanded := os.ExpandEnv(string(data))
	var cfg PlatformConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return fmt.Errorf("解析平台配置失败: %w", err)
	}
	if v := os.Getenv("PLATFORM_API_HOST"); v != "" {
		cfg.APIURL = v
	} else if v := os.Getenv("PLATFORM_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if cfg.APIURL == "" {
		return fmt.Errorf("平台配置缺少必填字段 api_url（请通过环境变量 PLATFORM_API_HOST/PLATFORM_API_URL 或 config/platform.yaml 的 api_url 指定，容器内禁止留空）")
	}
	if cfg.Secret == "" {
		return fmt.Errorf("平台配置缺少必填字段 secret（HMAC 签名密钥）")
	}
	if cfg.AdminPassword == "" {
		return fmt.Errorf("平台配置缺少必填字段 admin_password（用于 /platform/* JWT 登录）")
	}
	PlatformCfg = &cfg
	return nil
}

