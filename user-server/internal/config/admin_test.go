package config

import (
	"os"
	"testing"
)

func TestGetAdminConfig_Default(t *testing.T) {
	// Clear environment variables to ensure default values
	clearEnv()
	defer clearEnv()

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if config.DefaultAdmin.Username != "admin" {
		t.Errorf("Expected default username 'admin', got %s", config.DefaultAdmin.Username)
	}
	if config.DefaultAdmin.Password != "Admin@123456" {
		t.Errorf("Expected default password 'Admin@123456', got %s", config.DefaultAdmin.Password)
	}
	if config.DefaultAdmin.Email != "admin@example.com" {
		t.Errorf("Expected default email 'admin@example.com', got %s", config.DefaultAdmin.Email)
	}
	if config.DefaultAdmin.RealName != "系统管理员" {
		t.Errorf("Expected default real name '系统管理员', got %s", config.DefaultAdmin.RealName)
	}
}

func TestGetAdminConfig_AutoLogin(t *testing.T) {
	clearEnv()
	defer clearEnv()

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if !config.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be true by default")
	}
	if !config.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be true by default")
	}
}

func TestGetAdminConfig_Login(t *testing.T) {
	clearEnv()
	defer clearEnv()

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if !config.Login.ShowDefaultCredentials {
		t.Error("Expected Login.ShowDefaultCredentials to be true by default")
	}
	if config.Login.DefaultCredentialsHint != "默认账户：admin / Admin@123456" {
		t.Errorf("Expected Login.DefaultCredentialsHint '默认账户：admin / Admin@123456', got %s", config.Login.DefaultCredentialsHint)
	}
}

func TestGetDefaultAdminCredentials(t *testing.T) {
	clearEnv()
	defer clearEnv()

	username, password := GetDefaultAdminCredentials()
	if username != "admin" {
		t.Errorf("Expected username 'admin', got %s", username)
	}
	if password != "Admin@123456" {
		t.Errorf("Expected password 'Admin@123456', got %s", password)
	}
}

func TestGetAdminConfig_EnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	// Set environment variables
	os.Setenv("ADMIN_USERNAME", "custom_admin")
	os.Setenv("ADMIN_PASSWORD", "custom_password")
	os.Setenv("ADMIN_EMAIL", "custom@example.com")
	os.Setenv("ADMIN_REAL_NAME", "Custom Admin")

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if config.DefaultAdmin.Username != "custom_admin" {
		t.Errorf("Expected username 'custom_admin', got %s", config.DefaultAdmin.Username)
	}
	if config.DefaultAdmin.Password != "custom_password" {
		t.Errorf("Expected password 'custom_password', got %s", config.DefaultAdmin.Password)
	}
	if config.DefaultAdmin.Email != "custom@example.com" {
		t.Errorf("Expected email 'custom@example.com', got %s", config.DefaultAdmin.Email)
	}
	if config.DefaultAdmin.RealName != "Custom Admin" {
		t.Errorf("Expected real name 'Custom Admin', got %s", config.DefaultAdmin.RealName)
	}
}

func TestGetAdminConfig_AutoLoginEnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	os.Setenv("AUTO_LOGIN_ENABLED", "false")
	os.Setenv("USE_DEFAULT_ADMIN", "false")

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if config.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be false")
	}
	if config.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be false")
	}
}

func TestGetAdminConfig_LoginEnvOverride(t *testing.T) {
	clearEnv()
	defer clearEnv()

	os.Setenv("SHOW_DEFAULT_CREDENTIALS", "false")
	os.Setenv("DEFAULT_CREDENTIALS_HINT", "Custom hint")

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	if config.Login.ShowDefaultCredentials {
		t.Error("Expected Login.ShowDefaultCredentials to be false")
	}
	if config.Login.DefaultCredentialsHint != "Custom hint" {
		t.Errorf("Expected Login.DefaultCredentialsHint 'Custom hint', got %s", config.Login.DefaultCredentialsHint)
	}
}

func TestGetAdminConfig_InvalidBoolEnv(t *testing.T) {
	clearEnv()
	defer clearEnv()

	// Set invalid bool values - should use defaults
	os.Setenv("AUTO_LOGIN_ENABLED", "invalid")
	os.Setenv("USE_DEFAULT_ADMIN", "invalid")
	os.Setenv("SHOW_DEFAULT_CREDENTIALS", "invalid")

	config := GetAdminConfig()
	if config == nil {
		t.Fatal("GetAdminConfig returned nil")
	}

	// Should use default values when parsing fails
	if !config.AutoLogin.Enabled {
		t.Error("Expected AutoLogin.Enabled to be true (default)")
	}
	if !config.AutoLogin.UseDefaultAdmin {
		t.Error("Expected AutoLogin.UseDefaultAdmin to be true (default)")
	}
	if !config.Login.ShowDefaultCredentials {
		t.Error("Expected Login.ShowDefaultCredentials to be true (default)")
	}
}

func clearEnv() {
	envVars := []string{
		"ADMIN_USERNAME",
		"ADMIN_PASSWORD",
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
