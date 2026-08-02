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
	// APIURL 平台 API 地址（如 https://platform.example.com）
	APIURL string `yaml:"api_url" json:"api_url"`
	// Secret 商户签名密钥（HMAC-SHA256）
	Secret string `yaml:"secret" json:"secret"`
	// AdminUsername 平台管理员用户名（用于 JWT 登录代理 /platform/* 路由）
	AdminUsername string `yaml:"admin_username" json:"admin_username"`
	// AdminPassword 平台管理员密码（用于 JWT 登录代理 /platform/* 路由）
	AdminPassword string `yaml:"admin_password" json:"admin_password"`
	// LogReportInterval API 日志上报周期（秒），仅占位
	LogReportInterval int `yaml:"log_report_interval" json:"log_report_interval"`
	// LicenseSyncInterval 授权同步周期（秒），仅占位，开源版不再使用
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
	// 支持 ${VAR} 形式的环境变量展开
	expanded := os.ExpandEnv(string(data))
	var cfg PlatformConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return fmt.Errorf("解析平台配置失败: %w", err)
	}
	// 平台 API 地址解析优先级（单一来源，禁止任何硬编码具体域名作为默认）：
	//   1. PLATFORM_API_HOST   （容器/宿主必填，如 http://host.docker.internal:8205，
	//      容器内 user-server 必须经此访问平台，127.0.0.1/localhost 在容器内不可达）
	//   2. PLATFORM_API_URL    （兼容别名）
	//   3. config/platform.yaml 的 api_url 占位（由环境变量展开，可为空）
	// 三者皆空 → 返回错误（不再回退到硬编码域名，避免容器内跑到 127.0.0.1 失败）。
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
