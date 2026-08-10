package config

import (
	"os"
	"testing"
)

func TestNormalizePublicBaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "trim spaces", in: "  ", want: ""},
		{name: "trim trailing slash", in: "https://example.com/", want: "https://example.com"},
		{name: "trim multiple trailing slashes", in: "https://example.com////", want: "https://example.com"},
		{name: "no scheme + no port → https", in: "example.com", want: "https://example.com"},
		{name: "no scheme + with port → https", in: "example.com:8443", want: "https://example.com:8443"},
		{name: "http with port → upgraded to https", in: "http://example.com:8443", want: "https://example.com:8443"},
		{name: "http without port → kept as http (back-compat for dev)", in: "http://example.com", want: "http://example.com"},
		{name: "https with port kept", in: "https://example.com:8443", want: "https://example.com:8443"},
		{name: "https with subdomain kept", in: "https://chat.example.com", want: "https://chat.example.com"},
		{name: "https with path kept", in: "https://example.com/api", want: "https://example.com/api"},
		{name: "uppercase scheme lowercased only when http+port", in: "HTTP://Example.com:8443", want: "https://Example.com:8443"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePublicBaseURL(tc.in)
			if got != tc.want {
				t.Errorf("NormalizePublicBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestGetPublicBaseURL_EnvOverridesConfig(t *testing.T) {
	// 准备：临时设置环境变量 + 内存里塞一份 AppConfig
	oldEnv, hadEnv := os.LookupEnv("PUBLIC_BASE_URL")
	defer func() {
		if hadEnv {
			_ = os.Setenv("PUBLIC_BASE_URL", oldEnv)
		} else {
			_ = os.Unsetenv("PUBLIC_BASE_URL")
		}
	}()

	// 测试 1：仅配置文件生效
	t.Run("config only", func(t *testing.T) {
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		setAppConfigForTest(t, &AppConfig{External: ExternalConfig{PublicBaseURL: "https://config.example.com"}})
		if got := GetPublicBaseURL(); got != "https://config.example.com" {
			t.Errorf("got %q, want config.example.com", got)
		}
	})

	// 测试 2：环境变量覆盖配置
	t.Run("env overrides config", func(t *testing.T) {
		_ = os.Setenv("PUBLIC_BASE_URL", "https://env.example.com:8443")
		setAppConfigForTest(t, &AppConfig{External: ExternalConfig{PublicBaseURL: "https://config.example.com"}})
		if got := GetPublicBaseURL(); got != "https://env.example.com:8443" {
			t.Errorf("got %q, want env.example.com:8443 (env should override config)", got)
		}
	})

	// 测试 3：环境变量为空 + 配置文件为空 → 返回空串
	t.Run("both empty", func(t *testing.T) {
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		setAppConfigForTest(t, &AppConfig{})
		if got := GetPublicBaseURL(); got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	// 测试 4：环境变量配置了 http+端口 → 升级为 https
	t.Run("env http+port upgraded to https", func(t *testing.T) {
		_ = os.Setenv("PUBLIC_BASE_URL", "http://example.com:8443")
		setAppConfigForTest(t, &AppConfig{})
		if got := GetPublicBaseURL(); got != "https://example.com:8443" {
			t.Errorf("got %q, want https (http+port must be upgraded for Telegram)", got)
		}
	})
}

// setAppConfigForTest 替换 AppConfig（测试结束自动还原）
func setAppConfigForTest(t *testing.T, cfg *AppConfig) {
	t.Helper()
	old := GetAppConfig()
	SetAppConfig(cfg)
	t.Cleanup(func() { SetAppConfig(&old) })
}
