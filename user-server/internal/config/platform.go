package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type PlatformConfig struct {
	APIURL              string `yaml:"api_url"`
	Secret              string `yaml:"secret"`
	LogReportInterval   int    `yaml:"log_report_interval"`
	LicenseSyncInterval int    `yaml:"license_sync_interval"`
	AdminUsername       string `yaml:"admin_username"`
	AdminPassword       string `yaml:"admin_password"`
}

var PlatformCfg *PlatformConfig

// LoadPlatform 读取 platform.yaml 并对 ${VAR} 形式的环境变量做展开
// （合规基线 §7.2：敏感字段不落库明文，由部署环境注入）。
// 注意：Go 的 os.ExpandEnv 不支持 ${VAR:-default} 语法，未设置的环境变量将展开为空串，
// 由调用方（platform/client.go）做空值校验并返回明确错误。
func LoadPlatform(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	expanded := os.ExpandEnv(string(data))
	PlatformCfg = &PlatformConfig{}
	if err := yaml.Unmarshal([]byte(expanded), PlatformCfg); err != nil {
		return fmt.Errorf("解析 platform.yaml 失败: %w", err)
	}
	// 强制校验：管理员密码必须通过 ADMIN_PASSWORD 环境变量提供，禁止任何硬编码默认值
	if PlatformCfg.AdminPassword == "" {
		return fmt.Errorf("平台管理员密码未配置：请通过环境变量 ADMIN_PASSWORD 注入（参见 .env-example）")
	}
	if PlatformCfg.Secret == "" {
		return fmt.Errorf("商户 API 共享密钥未配置：请通过环境变量 MERCHANT_API_SECRET 注入")
	}
	// 环境变量 PLATFORM_URL 优先级最高，便于本地联调本地 platform-server
	// 默认 platform.yaml 中的 api_url 已经是线上域名 hivepaltformapi.xapptool.cn
	if v := os.Getenv("PLATFORM_URL"); v != "" {
		PlatformCfg.APIURL = v
	}
	return nil
}
