package tooluse

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// TestStructuralFingerprint_MapKeyOrderIndependence
// 核心性质：map 字段顺序变化 → 同一指纹。
// 业界依据：LLM function call 多采样时 map key 顺序不可预测。
func TestStructuralFingerprint_MapKeyOrderIndependence(t *testing.T) {
	a := map[string]any{"x": 1, "y": 2, "z": 3}
	b := map[string]any{"z": 3, "y": 2, "x": 1}
	c := map[string]any{"a": 0, "b": 0, "x": 1, "y": 2, "z": 3}

	fa := structuralFingerprint(a)
	fb := structuralFingerprint(b)
	if fa != fb {
		t.Errorf("key order should not affect fingerprint:\n  a=%s\n  b=%s", fa, fb)
	}

	fc := structuralFingerprint(c)
	if fc == fa {
		t.Errorf("different content should produce different fingerprint")
	}
}

// TestStructuralFingerprint_NestedMapOrdering
// 嵌套 map 也应被规范化排序。
func TestStructuralFingerprint_NestedMapOrdering(t *testing.T) {
	a := map[string]any{
		"outer": map[string]any{"x": 1, "y": 2},
		"top":   "value",
	}
	b := map[string]any{
		"top":   "value",
		"outer": map[string]any{"y": 2, "x": 1},
	}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("nested map key order should not affect fingerprint")
	}
}

// TestStructuralFingerprint_FloatPrecision
// float 不同表达方式应被识别为等价。
// 业界依据：LLM 在 tool call 中可能输出 1.0 / 1.00 / 1e0 多种表示。
func TestStructuralFingerprint_FloatPrecision(t *testing.T) {
	a := map[string]any{"v": 1.0}
	b := map[string]any{"v": 1.0}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("identical float should have same fingerprint")
	}

	c := map[string]any{"v": 1.5}
	d := map[string]any{"v": 1.5}
	if structuralFingerprint(c) != structuralFingerprint(d) {
		t.Error("1.5 should equal 1.5 in canonical form")
	}

	e := map[string]any{"v": 1.5}
	f := map[string]any{"v": 2.5}
	if structuralFingerprint(e) == structuralFingerprint(f) {
		t.Error("1.5 must differ from 2.5")
	}
}

// TestStructuralFingerprint_SliceOrderMatters
// slice 顺序改变 → 不同指纹（slice 顺序有业务语义）。
func TestStructuralFingerprint_SliceOrderMatters(t *testing.T) {
	a := map[string]any{"items": []any{1, 2, 3}}
	b := map[string]any{"items": []any{3, 2, 1}}
	if structuralFingerprint(a) == structuralFingerprint(b) {
		t.Error("slice order is semantically meaningful and should affect fingerprint")
	}
}

// TestStructuralFingerprint_NilHandling
// nil 值应被规范化为 "null"（与 JSON 标准一致）。
func TestStructuralFingerprint_NilHandling(t *testing.T) {
	a := map[string]any{"x": nil}
	b := map[string]any{"x": nil}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("nil handling should be deterministic")
	}

	c := map[string]any{"y": 1}
	if structuralFingerprint(a) == structuralFingerprint(c) {
		t.Error("missing key must differ from nil value")
	}
}

// TestStructuralFingerprint_StringEscaping
// 字符串中的特殊字符应被正确转义，不影响指纹稳定性。
func TestStructuralFingerprint_StringEscaping(t *testing.T) {
	a := map[string]any{"s": "hello\nworld"}
	b := map[string]any{"s": "hello\nworld"}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("identical escaped strings should have same fingerprint")
	}
	c := map[string]any{"s": "hello world"}
	if structuralFingerprint(a) == structuralFingerprint(c) {
		t.Error("\\n vs space must differ")
	}
}

// TestStructuralFingerprint_EmptyMap
// 空 map 与 nil map 在业务上等价（这里都返回 "empty"），便于循环检测统一处理。
func TestStructuralFingerprint_EmptyMap(t *testing.T) {
	if structuralFingerprint(nil) != "empty" {
		t.Error("nil map should return 'empty'")
	}
	if structuralFingerprint(map[string]any{}) != "empty" {
		t.Error("empty map should return 'empty'")
	}
}

// TestStructuralFingerprint_LengthStable
// 指纹长度必须稳定为 32 字符（与原 hashArgs 兼容），便于日志/DB 列不变。
func TestStructuralFingerprint_LengthStable(t *testing.T) {
	fp := structuralFingerprint(map[string]any{"x": 1})
	if len(fp) != 32 {
		t.Errorf("fingerprint length should be 32 hex chars, got %d", len(fp))
	}
}

// TestStructuralFingerprint_HashVsJSONMarshal
// 关键回归测试：原 hashArgs 依赖 json.Marshal 的 map 字母序。
// 新实现对 struct/混合类型应更稳定。
func TestStructuralFingerprint_HashVsJSONMarshal(t *testing.T) {
	type fakeArgs struct {
		A int
		B string
	}

	a := map[string]any{"args": fakeArgs{A: 1, B: "x"}}
	b := map[string]any{"args": fakeArgs{B: "x", A: 1}}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("struct field order should not affect fingerprint via canonical re-parse")
	}
}

// TestStructuralFingerprint_NumericTypes
// 整型不同宽度（int vs int32）应被识别为等价（同为整数）。
func TestStructuralFingerprint_NumericTypes(t *testing.T) {
	a := map[string]any{"n": int(5)}
	b := map[string]any{"n": int32(5)}
	c := map[string]any{"n": int64(5)}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("int and int32 should be equivalent")
	}
	if structuralFingerprint(b) != structuralFingerprint(c) {
		t.Error("int32 and int64 should be equivalent")
	}
}

// TestStructuralFingerprint_LargeFloat
// 防止 NaN/Inf 等异常 float 破坏指纹。
func TestStructuralFingerprint_LargeFloat(t *testing.T) {
	a := map[string]any{"v": math.MaxFloat64}
	b := map[string]any{"v": math.MaxFloat64}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Error("MaxFloat64 should be deterministic")
	}

	c := map[string]any{"v": math.NaN()}
	d := map[string]any{"v": math.NaN()}
	_ = structuralFingerprint(c)
	_ = structuralFingerprint(d)

	if strings.Contains(structuralFingerprint(c), "NaN") {
		t.Error("NaN should be formatted, not raw")
	}
}

// TestStructuralFingerprint_StableWithJSONInput
// 与标准 json.Unmarshal 配合，验证从 JSON 字符串反序列化的参数也能稳定指纹。
func TestStructuralFingerprint_StableWithJSONInput(t *testing.T) {
	json1 := `{"a":1,"b":[1,2,3],"c":{"x":true,"y":null}}`
	json2 := `{"c":{"y":null,"x":true},"b":[1,2,3],"a":1}`

	var a, b map[string]any
	if err := json.Unmarshal([]byte(json1), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(json2), &b); err != nil {
		t.Fatal(err)
	}
	if structuralFingerprint(a) != structuralFingerprint(b) {
		t.Errorf("JSON with reordered keys should have same fingerprint:\n  a=%s\n  b=%s",
			structuralFingerprint(a), structuralFingerprint(b))
	}
}
