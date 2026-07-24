package config

import (
	"os"
	"reflect"
	"testing"
)

// 2026-07-24 重构后：DefaultAdmin 已无 Password 字段；ShowDefaultCredentials 默认 false；
// 任何「默认密码」相关用例均已删除（admin 密码只能从 InitAdmin 写入 DB）。

func TestGetAdminConfig_Default(t *testing.T) {
	clearEnv()
	defer clearEnv()

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if cfg.DefaultAdmin.Username != "admin" {
		t.Errorf("Expected default username 'admin', got %s", cfg.DefaultAdmin.Username)
	}
	if cfg.DefaultAdmin.Email != "admin@example.com" {
		t.Errorf("Expected default email 'admin@example.com', got %s", cfg.DefaultAdmin.Email)
	}
	if cfg.DefaultAdmin.RealName != "系统管理员" {
		t.Errorf("Expected default real name '系统管理员', got %s", cfg.DefaultAdmin.RealName)
	}
}

func TestGetAdminConfig_AutoLogin(t *testing.T) {
	clearEnv()
	defer clearEnv()

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	// 重构后：自动登录默认关闭（不再诱导用户使用默认账号）
	if cfg.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be false by default (security hardening)")
	}
	if cfg.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be false by default (security hardening)")
	}
}

func TestGetAdminConfig_Login(t *testing.T) {
	clearEnv()
	defer clearEnv()

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	// 重构后：登录页不再展示默认凭据提示
	if cfg.Login.ShowDefaultCredentials {
		t.Error("Expected ShowDefaultCredentials to be false by default (no default password to show)")
	}
	if cfg.Login.DefaultCredentialsHint != "" {
		t.Errorf("Expected empty DefaultCredentialsHint, got %s", cfg.Login.DefaultCredentialsHint)
	}
}

func TestGetAdminConfig_NoPasswordField(t *testing.T) {
	// 强约束：DefaultAdminConfig 不再包含 Password 字段
	// 此用例为编译期/反射期双重保险，确保未来不会再有人加回 Password
	cfg := GetAdminConfig()
	_ = cfg.DefaultAdmin // 编译通过即说明 struct 不含 Password

	// 反射断言：字段集中不含 Password
	if hasPasswordField("DefaultAdminConfig") {
		t.Error("DefaultAdminConfig 不应再有 Password 字段")
	}
}

func TestGetAdminConfig_EnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	// 仅覆盖非敏感的展示字段
	os.Setenv("ADMIN_USERNAME", "custom_admin")
	os.Setenv("ADMIN_EMAIL", "custom@example.com")
	os.Setenv("ADMIN_REAL_NAME", "Custom Admin")

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if cfg.DefaultAdmin.Username != "custom_admin" {
		t.Errorf("Expected username 'custom_admin', got %s", cfg.DefaultAdmin.Username)
	}
	if cfg.DefaultAdmin.Email != "custom@example.com" {
		t.Errorf("Expected email 'custom@example.com', got %s", cfg.DefaultAdmin.Email)
	}
	if cfg.DefaultAdmin.RealName != "Custom Admin" {
		t.Errorf("Expected real name 'Custom Admin', got %s", cfg.DefaultAdmin.RealName)
	}
}

func TestGetAdminConfig_AutoLoginEnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	os.Setenv("AUTO_LOGIN_ENABLED", "true")
	os.Setenv("USE_DEFAULT_ADMIN", "true")

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if !cfg.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be true when env override set")
	}
	if !cfg.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be true when env override set")
	}
}

func TestGetAdminConfig_LoginEnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	os.Setenv("SHOW_DEFAULT_CREDENTIALS", "true")
	os.Setenv("DEFAULT_CREDENTIALS_HINT", "Custom hint")

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if !cfg.Login.ShowDefaultCredentials {
		t.Error("Expected ShowDefaultCredentials to be true when env override set")
	}
	if cfg.Login.DefaultCredentialsHint != "Custom hint" {
		t.Errorf("Expected hint 'Custom hint', got %s", cfg.Login.DefaultCredentialsHint)
	}
}

func TestGetAdminConfig_InvalidBoolEnv(t *testing.T) {
	clearEnv()
	defer clearEnv()

	os.Setenv("AUTO_LOGIN_ENABLED", "invalid")
	os.Setenv("USE_DEFAULT_ADMIN", "invalid")
	os.Setenv("SHOW_DEFAULT_CREDENTIALS", "invalid")

	cfg := GetAdminConfig()
	if cfg == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	// 解析失败时使用默认值（false）
	if cfg.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be false (default) on parse error")
	}
	if cfg.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be false (default) on parse error")
	}
	if cfg.Login.ShowDefaultCredentials {
		t.Error("Expected ShowDefaultCredentials to be false (default) on parse error")
	}
}

// hasPasswordField 反射检查结构体是否含 Password 字段（防止回归）
func hasPasswordField(typeName string) bool {
	t := reflectTypeByName(typeName)
	if t == nil {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Name == "Password" {
			return true
		}
	}
	return false
}

// reflectTypeByName 按类型名查找 reflect.Type（同包内）
func reflectTypeByName(name string) reflect.Type {
	switch name {
	case "DefaultAdminConfig":
		return reflect.TypeOf(DefaultAdminConfig{})
	case "AdminConfig":
		return reflect.TypeOf(AdminConfig{})
	default:
		return nil
	}
}

func clearEnv() {
	envVars := []string{
		"ADMIN_USERNAME",
		"PLATFORM_ADMIN_PASSWORD", // 保留为清理项（即便不再读取，也不允许残留）
		"ADMIN_EMAIL",
		"ADMIN_REAL_NAME",
		"AUTO_LOGIN_ENABLED",
		"USE_DEFAULT_ADMIN",
		"SHOW_DEFAULT_CREDENTIALS",
		"DEFAULT_CREDENTIALS_HINT",
		"ADMIN_CONFIG_FILE",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}
