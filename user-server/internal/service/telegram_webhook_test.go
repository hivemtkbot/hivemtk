package service

import (
	"os"
	"testing"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
)

// TestResolveTelegramWebhookURL 覆盖 ResolveTelegramWebhookURL 的所有分支
func TestResolveTelegramWebhookURL(t *testing.T) {
	oldEnv := os.Getenv("PUBLIC_BASE_URL")
	defer func() {
		if oldEnv == "" {
			_ = os.Unsetenv("PUBLIC_BASE_URL")
		} else {
			_ = os.Setenv("PUBLIC_BASE_URL", oldEnv)
		}
	}()
	oldCfg := config.GetAppConfig()
	defer config.SetAppConfig(&oldCfg)

	t.Run("nil account", func(t *testing.T) {
		url, has := ResolveTelegramWebhookURL(nil)
		if url != "" || has {
			t.Errorf("nil account: got url=%q has=%v, want both empty/false", url, has)
		}
	})

	t.Run("explicit webhook_url wins regardless of public_base", func(t *testing.T) {
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		config.SetAppConfig(&config.AppConfig{External: config.ExternalConfig{PublicBaseURL: "https://env.example.com"}})
		acc := &model.TelegramAccount{ID: 42, WebhookURL: "https://custom.example.com/hook"}
		url, has := ResolveTelegramWebhookURL(acc)
		if url != "https://custom.example.com/hook" {
			t.Errorf("got %q, want explicit URL to win", url)
		}
		if !has {
			t.Errorf("has should be true when explicit URL provided")
		}
	})

	t.Run("falls back to public_base_url env when explicit empty", func(t *testing.T) {
		_ = os.Setenv("PUBLIC_BASE_URL", "https://env.example.com")
		config.SetAppConfig(&config.AppConfig{})
		acc := &model.TelegramAccount{ID: 7, WebhookURL: ""}
		url, has := ResolveTelegramWebhookURL(acc)
		if url != "https://env.example.com/api/webhook/telegram/7" {
			t.Errorf("got %q, want env-based URL", url)
		}
		if !has {
			t.Errorf("has should be true when public base URL hits")
		}
	})

	t.Run("falls back to config.external.public_base_url when env empty", func(t *testing.T) {
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		config.SetAppConfig(&config.AppConfig{External: config.ExternalConfig{PublicBaseURL: "https://cfg.example.com/"}})
		acc := &model.TelegramAccount{ID: 9, WebhookURL: ""}
		url, has := ResolveTelegramWebhookURL(acc)
		if url != "https://cfg.example.com/api/webhook/telegram/9" {
			t.Errorf("got %q, want config-based URL with trailing slash stripped", url)
		}
		if !has {
			t.Errorf("has should be true when config hits")
		}
	})

	t.Run("returns empty when no explicit and no public base", func(t *testing.T) {
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		config.SetAppConfig(&config.AppConfig{})
		acc := &model.TelegramAccount{ID: 11, WebhookURL: ""}
		url, has := ResolveTelegramWebhookURL(acc)
		if url != "" {
			t.Errorf("got %q, want empty when no source", url)
		}
		if has {
			t.Errorf("has should be false when no source")
		}
	})
}

// TestIsTelegramPollingEnabled 覆盖 polling 自动启用的判定逻辑
func TestIsTelegramPollingEnabled(t *testing.T) {
	oldEnv := os.Getenv("TELEGRAM_POLLING_ENABLED")
	oldPB := os.Getenv("PUBLIC_BASE_URL")
	defer func() {
		_ = os.Setenv("TELEGRAM_POLLING_ENABLED", oldEnv)
		_ = os.Setenv("PUBLIC_BASE_URL", oldPB)
	}()
	oldCfg := config.GetAppConfig()
	defer config.SetAppConfig(&oldCfg)

	t.Run("env=1 forces on even with public_base set", func(t *testing.T) {
		_ = os.Setenv("TELEGRAM_POLLING_ENABLED", "1")
		_ = os.Setenv("PUBLIC_BASE_URL", "https://example.com")
		config.SetAppConfig(&config.AppConfig{})
		if !IsTelegramPollingEnabled() {
			t.Error("env=1 should force polling on")
		}
	})

	t.Run("env=0 forces off even without public_base", func(t *testing.T) {
		_ = os.Setenv("TELEGRAM_POLLING_ENABLED", "0")
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		config.SetAppConfig(&config.AppConfig{})
		if IsTelegramPollingEnabled() {
			t.Error("env=0 should force polling off")
		}
	})

	t.Run("env unset + public_base set → polling off (webhook path)", func(t *testing.T) {
		_ = os.Unsetenv("TELEGRAM_POLLING_ENABLED")
		_ = os.Setenv("PUBLIC_BASE_URL", "https://example.com")
		config.SetAppConfig(&config.AppConfig{})
		if IsTelegramPollingEnabled() {
			t.Error("with public base URL, polling should be disabled to use webhook")
		}
	})

	t.Run("env unset + no public_base → polling on (fallback)", func(t *testing.T) {
		_ = os.Unsetenv("TELEGRAM_POLLING_ENABLED")
		_ = os.Unsetenv("PUBLIC_BASE_URL")
		config.SetAppConfig(&config.AppConfig{})
		if !IsTelegramPollingEnabled() {
			t.Error("without public base URL, polling should auto-enable as fallback")
		}
	})

	t.Run("env=true/on/yes parsed correctly", func(t *testing.T) {
		for _, v := range []string{"true", "TRUE", "yes", "YES", "on", "ON"} {
			_ = os.Setenv("TELEGRAM_POLLING_ENABLED", v)
			if !IsTelegramPollingEnabled() {
				t.Errorf("env=%q should enable polling", v)
			}
		}
	})
}

// TestIsTelegramConflictError 覆盖 409 Conflict 错误识别
func TestIsTelegramConflictError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errString("random error"), false},
		{errString("409"), true},
		{errString("Conflict: terminated by other getUpdates request"), true},
		{errString("HTTP 409 conflict"), true},
	}
	for _, c := range cases {
		got := isTelegramConflictError(c.err)
		if got != c.want {
			t.Errorf("isTelegramConflictError(%q) = %v, want %v", errMsg(c.err), got, c.want)
		}
	}
}

// TestValidateTelegramWebhookURL 覆盖 S3-5 修复：URL 格式校验
//
// 校验规则：
//  1. 空 → 拒
//  2. scheme != https → 拒
//  3. host 缺失 → 拒
//  4. path 前缀不是 /api/webhook/telegram/ → 拒
//  5. 其余合法 https URL → 通过
func TestValidateTelegramWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "missing scheme", in: "example.com/api/webhook/telegram/1", wantErr: true},
		{name: "http scheme rejected", in: "http://chat.example.com/api/webhook/telegram/1", wantErr: true},
		{name: "HTTP uppercase rejected", in: "HTTP://chat.example.com/api/webhook/telegram/1", wantErr: true},
		{name: "missing host", in: "https:///api/webhook/telegram/1", wantErr: true},
		{name: "wrong path prefix", in: "https://chat.example.com/webhook/tg/1", wantErr: true},
		{name: "wrong path api only", in: "https://chat.example.com/api/hook/1", wantErr: true},
		{name: "wrong prefix telegram only", in: "https://chat.example.com/telegram/1", wantErr: true},
		{name: "valid https with path", in: "https://chat.example.com/api/webhook/telegram/1", wantErr: false},
		{name: "valid https with port", in: "https://chat.example.com:8443/api/webhook/telegram/42", wantErr: false},
		{name: "valid https subdomain", in: "https://bot.example.com/api/webhook/telegram/7", wantErr: false},
		{name: "valid https with trailing slash and id", in: "https://chat.example.com/api/webhook/telegram/100", wantErr: false},
		{name: "malformed url parse error", in: "ht!tp://[invalid", wantErr: true},
		{name: "valid https with extra path segment allowed", in: "https://chat.example.com/api/webhook/telegram/3/extra", wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := ValidateTelegramWebhookURL(tc.in)
			if tc.wantErr && gotErr == nil {
				t.Errorf("ValidateTelegramWebhookURL(%q) = nil, want error", tc.in)
			}
			if !tc.wantErr && gotErr != nil {
				t.Errorf("ValidateTelegramWebhookURL(%q) = %v, want nil", tc.in, gotErr)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
func errMsg(e error) string {
	if e == nil {
		return "<nil>"
	}
	return e.Error()
}
