// 注意：默认密码已废弃。初始化超管必须由调用方传 password。
//
// 阶段 3 改造（系统用户统一 plan v3.1 §3.2）：
//   - 不再硬编码默认密码 Admin@123456（防止供应链 / 误部署直接落入生产）
//   - 创建初始超管统一走 POST /api/system/init-admin，由调用方在请求体中传 password
//   - config 仅保留 Username / Email / RealName 等非敏感默认值，Password 留空
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// AdminConfig 管理员配置
type AdminConfig struct {
	DefaultAdmin DefaultAdminConfig `json:"default_admin" yaml:"default_admin"`
	AutoLogin    AutoLoginConfig    `json:"auto_login" yaml:"auto_login"`
	Login        LoginConfig        `json:"login" yaml:"login"`
}

// DefaultAdminConfig 默认管理员配置
type DefaultAdminConfig struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	Email    string `json:"email" yaml:"email"`
	RealName string `json:"real_name" yaml:"real_name"`
}

// AutoLoginConfig 自动登录配置
type AutoLoginConfig struct {
	Enabled         bool `json:"enabled" yaml:"enabled"`
	UseDefaultAdmin bool `json:"use_default_admin" yaml:"use_default_admin"`
}

// LoginConfig 登录配置
type LoginConfig struct {
	ShowDefaultCredentials bool   `json:"show_default_credentials" yaml:"show_default_credentials"`
	DefaultCredentialsHint string `json:"default_credentials_hint" yaml:"default_credentials_hint"`
}

// 默认管理员配置
//
// Password 字段已废弃：阶段 3 起，初始化超管必须由调用方在请求体中传入。
// 此处保留空字符串（不预置 Admin@123456），防止任何代码路径误用默认值。
var defaultAdminConfig = AdminConfig{
	DefaultAdmin: DefaultAdminConfig{
		Username: "admin",
		Password: "",
		Email:    "admin@example.com",
		RealName: "系统管理员",
	},
	AutoLogin: AutoLoginConfig{
		Enabled:         true,
		UseDefaultAdmin: true,
	},
	Login: LoginConfig{
		ShowDefaultCredentials: false,
		DefaultCredentialsHint: "",
	},
}

// GetAdminConfig 获取管理员配置
func GetAdminConfig() *AdminConfig {
	config := defaultAdminConfig

	// 优先从配置文件读取（可选）：默认 ./.env/admin.json；也可通过 ADMIN_CONFIG_FILE 指定路径
	cfgPath := os.Getenv("ADMIN_CONFIG_FILE")
	if cfgPath == "" {
		cfgPath = filepath.Join(".env", "admin.json")
	}
	if b, err := os.ReadFile(cfgPath); err == nil && len(b) > 0 {
		var fileCfg AdminConfig
		if jsonErr := json.Unmarshal(b, &fileCfg); jsonErr == nil {
			// 合并文件配置到默认配置
			if fileCfg.DefaultAdmin.Username != "" {
				config.DefaultAdmin.Username = fileCfg.DefaultAdmin.Username
			}
			if fileCfg.DefaultAdmin.Password != "" {
				config.DefaultAdmin.Password = fileCfg.DefaultAdmin.Password
			}
			if fileCfg.DefaultAdmin.Email != "" {
				config.DefaultAdmin.Email = fileCfg.DefaultAdmin.Email
			}
			if fileCfg.DefaultAdmin.RealName != "" {
				config.DefaultAdmin.RealName = fileCfg.DefaultAdmin.RealName
			}

			config.AutoLogin.Enabled = fileCfg.AutoLogin.Enabled
			config.AutoLogin.UseDefaultAdmin = fileCfg.AutoLogin.UseDefaultAdmin

			if fileCfg.Login.DefaultCredentialsHint != "" {
				config.Login.DefaultCredentialsHint = fileCfg.Login.DefaultCredentialsHint
			}
			config.Login.ShowDefaultCredentials = fileCfg.Login.ShowDefaultCredentials
		}
	}

	// 从环境变量读取配置（如果存在）
	if username := os.Getenv("ADMIN_USERNAME"); username != "" {
		config.DefaultAdmin.Username = username
	}
	if password := os.Getenv("ADMIN_PASSWORD"); password != "" {
		config.DefaultAdmin.Password = password
	}
	if email := os.Getenv("ADMIN_EMAIL"); email != "" {
		config.DefaultAdmin.Email = email
	}
	if realName := os.Getenv("ADMIN_REAL_NAME"); realName != "" {
		config.DefaultAdmin.RealName = realName
	}

	// 自动登录配置
	if autoLogin := os.Getenv("AUTO_LOGIN_ENABLED"); autoLogin != "" {
		if enabled, err := strconv.ParseBool(autoLogin); err == nil {
			config.AutoLogin.Enabled = enabled
		}
	}
	if useDefault := os.Getenv("USE_DEFAULT_ADMIN"); useDefault != "" {
		if use, err := strconv.ParseBool(useDefault); err == nil {
			config.AutoLogin.UseDefaultAdmin = use
		}
	}

	// 登录配置
	if showHint := os.Getenv("SHOW_DEFAULT_CREDENTIALS"); showHint != "" {
		if show, err := strconv.ParseBool(showHint); err == nil {
			config.Login.ShowDefaultCredentials = show
		}
	}
	if hint := os.Getenv("DEFAULT_CREDENTIALS_HINT"); hint != "" {
		config.Login.DefaultCredentialsHint = hint
	}

	return &config
}

// GetDefaultAdminCredentials 获取默认管理员凭据
func GetDefaultAdminCredentials() (string, string) {
	config := GetAdminConfig()
	return config.DefaultAdmin.Username, config.DefaultAdmin.Password
}
