package config

import (
	"os"
	"reflect"
	"testing"
)

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

	if cfg.Login.ShowDefaultCredentials {
		t.Error("Expected ShowDefaultCredentials to be false by default (no default password to show)")
	}
	if cfg.Login.DefaultCredentialsHint != "" {
		t.Errorf("Expected empty DefaultCredentialsHint, got %s", cfg.Login.DefaultCredentialsHint)
	}
}

func TestGetAdminConfig_NoPasswordField(t *testing.T) {
	cfg := GetAdminConfig()
	_ = cfg.DefaultAdmin

	if hasPasswordField("DefaultAdminConfig") {
		t.Error("DefaultAdminConfig 不应再有 Password 字段")
	}
}

func TestGetAdminConfig_EnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

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
		"PLATFORM_ADMIN_PASSWORD",
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
