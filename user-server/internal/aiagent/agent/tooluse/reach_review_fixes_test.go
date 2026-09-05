package tooluse

import (
	"testing"
)

// TestGetArgMap_PreservesTypes 验证 getArgMap 保留原始类型（数字/布尔/字符串/对象）
func TestGetArgMap_PreservesTypes(t *testing.T) {
	args := map[string]any{
		"payload": map[string]any{
			"order_id": float64(12345),
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
