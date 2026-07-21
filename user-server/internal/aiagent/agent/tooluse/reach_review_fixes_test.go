package tooluse

import (
	"errors"
	"testing"
)

// TestParseAccountID 覆盖 parseAccountID 边界用例
//
// 2026-07-17 二次审核修复：原实现自写字符循环，改用 strconv 后必须保留相同行为
func TestParseAccountID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint
		wantErr error
	}{
		{"normal", "123", 123, nil},
		{"large", "999999", 999999, nil},
		{"zero", "0", 0, ErrInvalidAccountID},
		{"empty", "", 0, ErrInvalidAccountID},
		{"non-numeric", "abc", 0, ErrInvalidAccountID},
		{"negative", "-1", 0, ErrInvalidAccountID},
		{"mixed", "12a3", 0, ErrInvalidAccountID},
		{"space", " 123", 0, ErrInvalidAccountID},
		{"plus", "+123", 0, ErrInvalidAccountID},
		{"hex", "0x1A", 0, ErrInvalidAccountID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAccountID(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("parseAccountID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("parseAccountID(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseAccountID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseInt64 覆盖 parseInt64 边界用例（包括群组 chat_id 负数）
func TestParseInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr error
	}{
		{"normal-positive", "123", 123, nil},
		{"normal-negative", "-1001234567890", -1001234567890, nil},
		{"zero", "0", 0, ErrInvalidInt64},
		{"empty", "", 0, ErrInvalidInt64},
		{"non-numeric", "abc", 0, ErrInvalidInt64},
		{"plus-prefix", "+123", 123, nil},
		{"minus-only", "-", 0, ErrInvalidInt64},
		{"trailing-chars", "123x", 0, ErrInvalidInt64},
		{"max-int64", "9223372036854775807", 9223372036854775807, nil},
		{"min-int64", "-9223372036854775808", -9223372036854775808, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt64(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("parseInt64(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("parseInt64(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseInt64(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestIntegrationReachAdapter_SentinelErrors 验证 IntegrationReachAdapter 返回 sentinel error
//
// 2026-07-17 二次审核修复：原实现用 fmt.Errorf("...not implemented") 字符串错误
// 改为 wrap ErrChannelNotImplemented / ErrIntegrationServiceNotConfigured 让 errors.Is 生效
func TestIntegrationReachAdapter_SentinelErrors(t *testing.T) {
	// 无 IntegrationService 注入时，三个新方法应返回 ErrIntegrationServiceNotConfigured
	a := &IntegrationReachAdapter{} // 全部为 nil
	_, err := a.SendTelegram(nil, "1", "123", "x")
	if !errors.Is(err, ErrIntegrationServiceNotConfigured) {
		t.Errorf("SendTelegram 应返回 ErrIntegrationServiceNotConfigured, got %v", err)
	}
	_, err = a.SendWhatsApp(nil, "1", "+861", "x")
	if !errors.Is(err, ErrIntegrationServiceNotConfigured) {
		t.Errorf("SendWhatsApp 应返回 ErrIntegrationServiceNotConfigured, got %v", err)
	}
	_, err = a.SendFeishu(nil, "1", "ou_x", "x")
	if !errors.Is(err, ErrIntegrationServiceNotConfigured) {
		t.Errorf("SendFeishu 应返回 ErrIntegrationServiceNotConfigured, got %v", err)
	}

	// 8 个未实现方法应返回 ErrChannelNotImplemented
	_, err = a.SendSMS(nil, "1", "x", "", nil)
	if !errors.Is(err, ErrChannelNotImplemented) {
		t.Errorf("SendSMS 应返回 ErrChannelNotImplemented, got %v", err)
	}
	_, err = a.SendEmail(nil, "1", "s", "x", nil)
	if !errors.Is(err, ErrChannelNotImplemented) {
		t.Errorf("SendEmail 应返回 ErrChannelNotImplemented, got %v", err)
	}
	err = a.Recall(nil, "sms", "x")
	if !errors.Is(err, ErrChannelNotImplemented) {
		t.Errorf("Recall 应返回 ErrChannelNotImplemented, got %v", err)
	}
	_, err = a.AccountHealth(nil, "sms", "1")
	if !errors.Is(err, ErrChannelNotImplemented) {
		t.Errorf("AccountHealth 应返回 ErrChannelNotImplemented, got %v", err)
	}
	_, err = a.ListAccounts(nil, "sms")
	if !errors.Is(err, ErrChannelNotImplemented) {
		t.Errorf("ListAccounts 应返回 ErrChannelNotImplemented, got %v", err)
	}
}

// TestNewIntegrationReachAdapterFromDB_NilDB 验证 nil db 时不 panic 且返回空壳
//
// 2026-07-17 二次审核修复：原实现忽略 db 参数只返回空壳；现保留"nil db 返回空壳"行为，但要让
// 真实 db 路径真正实例化 IntegrationService（见 TestNewIntegrationReachAdapterFromDB_RealDB）
func TestNewIntegrationReachAdapterFromDB_NilDB(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("NewIntegrationReachAdapterFromDB(nil) panic: %v", r)
		}
	}()
	a := NewIntegrationReachAdapterFromDB(nil)
	if a == nil {
		t.Fatal("应返回非 nil adapter")
	}
	if a.tg != nil || a.wa != nil || a.feishu != nil {
		t.Error("nil db 时所有 IntegrationService 应为 nil")
	}
}

// TestGetArgMap_PreservesTypes 验证 getArgMap 保留原始类型（数字/布尔/字符串/对象）
//
// 2026-07-17 二次审核修复：替代 getArgStringMap 解决 BatchSendItem.Payload 类型丢失问题
func TestGetArgMap_PreservesTypes(t *testing.T) {
	args := map[string]any{
		"payload": map[string]any{
			"order_id": float64(12345), // JSON 数字反序列化为 float64
			"vip":      true,
			"name":     "Alice",
			"tags":     []any{"a", "b"},
			"nested":   map[string]any{"k": "v"},
		},
	}
	got := getArgMap(args, "payload")
	if got == nil {
		t.Fatal("getArgMap returned nil")
	}
	if v, ok := got["order_id"].(float64); !ok || v != 12345 {
		t.Errorf("order_id 类型应为 float64(12345), got %T(%v)", got["order_id"], got["order_id"])
	}
	if v, ok := got["vip"].(bool); !ok || !v {
		t.Errorf("vip 类型应为 bool(true), got %T(%v)", got["vip"], got["vip"])
	}
	if v, ok := got["name"].(string); !ok || v != "Alice" {
		t.Errorf("name 类型应为 string(Alice), got %T(%v)", got["name"], got["name"])
	}
	if _, ok := got["tags"].([]any); !ok {
		t.Errorf("tags 类型应为 []interface{}, got %T", got["tags"])
	}
	if _, ok := got["nested"].(map[string]any); !ok {
		t.Errorf("nested 类型应为 map[string]interface{}, got %T", got["nested"])
	}
}

func TestGetArgMap_NilAndMissing(t *testing.T) {
	if got := getArgMap(nil, "x"); got != nil {
		t.Errorf("nil args 应返回 nil, got %v", got)
	}
	if got := getArgMap(map[string]any{}, "missing"); got != nil {
		t.Errorf("missing key 应返回 nil, got %v", got)
	}
	if got := getArgMap(map[string]any{"x": "string"}, "x"); got != nil {
		t.Errorf("非 map 值应返回 nil, got %v", got)
	}
}
