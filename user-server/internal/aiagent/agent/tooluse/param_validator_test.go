package tooluse

import (
	"testing"
)

// TestValidateParams_MissingRequired 验证必填字段缺失被拦截
func TestValidateParams_MissingRequired(t *testing.T) {
	schema := ToolParameters{
		Type: "object",
		Properties: map[string]ToolParam{
			"name": {Type: "string"},
		},
		Required: []string{"name"},
	}
	err := validateParams("test", schema, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !contains(err.Error(), "missing required param: name") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateParams_TypeMismatch_String 验证 string 类型不匹配
func TestValidateParams_TypeMismatch_String(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"x": {Type: "string"},
		},
	}
	err := validateParams("test", schema, map[string]any{"x": 123})
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
	if !contains(err.Error(), "type mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestValidateParams_TypeMismatch_Number 验证 number 类型不匹配
func TestValidateParams_TypeMismatch_Number(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"x": {Type: "number"},
		},
	}
	err := validateParams("test", schema, map[string]any{"x": "abc"})
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

// TestValidateParams_IntegerFromFloat 验证 LLM 常见的 "1.0" → integer 1 兼容
func TestValidateParams_IntegerFromFloat(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"x": {Type: "integer"},
		},
	}
	if err := validateParams("test", schema, map[string]any{"x": float64(5)}); err != nil {
		t.Errorf("5.0 should be valid integer: %v", err)
	}
	if err := validateParams("test", schema, map[string]any{"x": float64(5.5)}); err == nil {
		t.Error("5.5 should be rejected for integer type")
	}
}

func TestValidateParams_NestedObject(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"user": {
				Type: "object",
				Properties: map[string]ToolParam{
					"email": {Type: "string"},
					"age":   {Type: "integer"},
				},
				Required: []string{"email"},
			},
		},
	}

	err := validateParams("test", schema, map[string]any{
		"user": map[string]any{"age": 30},
	})
	if err == nil {
		t.Fatal("expected nested required error")
	}
	if !contains(err.Error(), "user missing required sub-param: email") {
		t.Errorf("unexpected error: %v", err)
	}

	err = validateParams("test", schema, map[string]any{
		"user": map[string]any{"email": "x@y.z", "age": "not-a-number"},
	})
	if err == nil {
		t.Fatal("expected nested type error")
	}

	if err := validateParams("test", schema, map[string]any{
		"user": map[string]any{"email": "x@y.z", "age": 30},
	}); err != nil {
		t.Errorf("valid nested should pass: %v", err)
	}
}

// TestValidateParams_ArrayItems 验证数组元素校验
func TestValidateParams_ArrayItems(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"tags": {
				Type:  "array",
				Items: &ToolParam{Type: "string"},
			},
		},
	}

	err := validateParams("test", schema, map[string]any{
		"tags": []any{"valid", 123, "another"},
	})
	if err == nil {
		t.Fatal("expected array element type error")
	}
	if !contains(err.Error(), "tags[1]") {
		t.Errorf("error should pinpoint element index, got: %v", err)
	}

	if err := validateParams("test", schema, map[string]any{
		"tags": []any{"a", "b", "c"},
	}); err != nil {
		t.Errorf("valid array should pass: %v", err)
	}
}

// TestValidateParams_StringLength 验证 minLength / maxLength
func TestValidateParams_StringLength(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"code": {Type: "string", MinLength: 3, MaxLength: 5},
		},
	}

	if err := validateParams("test", schema, map[string]any{"code": "ab"}); err == nil {
		t.Error("minLength=3 should reject 'ab'")
	}
	if err := validateParams("test", schema, map[string]any{"code": "abcdef"}); err == nil {
		t.Error("maxLength=5 should reject 'abcdef'")
	}
	if err := validateParams("test", schema, map[string]any{"code": "abc"}); err != nil {
		t.Errorf("valid length should pass: %v", err)
	}
}

// TestValidateParams_NumberRange 验证 minimum / maximum
func TestValidateParams_NumberRange(t *testing.T) {
	min := 0.0
	max := 100.0
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"score": {Type: "number", Minimum: &min, Maximum: &max},
		},
	}

	if err := validateParams("test", schema, map[string]any{"score": -1.0}); err == nil {
		t.Error("minimum=0 should reject -1")
	}
	if err := validateParams("test", schema, map[string]any{"score": 101.0}); err == nil {
		t.Error("maximum=100 should reject 101")
	}
	if err := validateParams("test", schema, map[string]any{"score": 50.0}); err != nil {
		t.Errorf("valid range should pass: %v", err)
	}
}

// TestValidateParams_Enum 验证枚举
func TestValidateParams_Enum(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"color": {Type: "string", Enum: []string{"red", "green", "blue"}},
		},
	}
	if err := validateParams("test", schema, map[string]any{"color": "yellow"}); err == nil {
		t.Error("enum mismatch should be rejected")
	}
	if err := validateParams("test", schema, map[string]any{"color": "red"}); err != nil {
		t.Errorf("valid enum should pass: %v", err)
	}
}

// TestValidateParams_DeepNesting 验证 3 层嵌套
func TestValidateParams_DeepNesting(t *testing.T) {
	schema := ToolParameters{
		Properties: map[string]ToolParam{
			"order": {
				Type: "object",
				Properties: map[string]ToolParam{
					"items": {
						Type: "array",
						Items: &ToolParam{
							Type: "object",
							Properties: map[string]ToolParam{
								"sku":   {Type: "string", MinLength: 1},
								"count": {Type: "integer", Minimum: ptrFloat(1)},
							},
							Required: []string{"sku"},
						},
					},
				},
			},
		},
	}

	err := validateParams("test", schema, map[string]any{
		"order": map[string]any{
			"items": []any{
				map[string]any{"sku": "A1", "count": 2},
				map[string]any{"count": 0},
			},
		},
	})
	if err == nil {
		t.Fatal("expected deep nesting error")
	}
	if !contains(err.Error(), "order.items[1]") {
		t.Errorf("error path should show full chain, got: %v", err)
	}

	err = validateParams("test", schema, map[string]any{
		"order": map[string]any{
			"items": []any{
				map[string]any{"sku": "A1", "count": 2},
				map[string]any{"sku": "A2", "count": 0},
			},
		},
	})
	if err == nil {
		t.Fatal("expected minimum violation")
	}
	if !contains(err.Error(), "items[1].count") {
		t.Errorf("error path should reach count, got: %v", err)
	}
}

func ptrFloat(f float64) *float64 { return &f }
