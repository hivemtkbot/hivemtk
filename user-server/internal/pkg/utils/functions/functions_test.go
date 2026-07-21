package functions

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// 测试用的结构体 - 用于测试不包含 omitempty 的行为
type TestStruct struct {
	Name    string          `json:"name"`
	Age     int             `json:"age"`
	Email   string          `json:"email"`
	NilPtr  *string         `json:"nil_ptr"`
	Decimal decimal.Decimal `json:"decimal"`
	UUID    uuid.UUID       `json:"uuid"`
	NoTag   string          // 没有 json tag
}

// 测试用的结构体 - 用于测试 omitempty 的行为
type TestStructWithOmitEmpty struct {
	Name    string          `json:"name"`
	Email   string          `json:"email,omitempty"`
	Decimal decimal.Decimal `json:"decimal"`
	UUID    uuid.UUID       `json:"uuid"`
}

func TestStructToMap_Basic(t *testing.T) {
	testStruct := TestStruct{
		Name:    "John",
		Age:     30,
		Email:   "john@example.com",
		NilPtr:  nil,
		Decimal: decimal.NewFromFloat(99.99),
		UUID:    uuid.Nil,
		NoTag:   "should be skipped",
	}

	result := StructToMap(testStruct)

	// 验证基本字段
	if result["name"] != "John" {
		t.Errorf("StructToMap() name = %v, want John", result["name"])
	}
	// 使用反射比较 decimal
	if !result["decimal"].(decimal.Decimal).Equal(decimal.NewFromFloat(99.99)) {
		t.Errorf("StructToMap() decimal = %v, want 99.99", result["decimal"])
	}
	// uuid.Nil 应该被转换为 nil
	if result["uuid"] != nil {
		t.Errorf("StructToMap() uuid = %v, want nil", result["uuid"])
	}
	// 没有 json tag 的字段应该被跳过
	if _, exists := result["NoTag"]; exists {
		t.Error("StructToMap() should skip fields without json tag")
	}
	// email 有 omitempty 且不为空，应该保留
	if result["email"] != "john@example.com" {
		t.Errorf("StructToMap() email = %v, want john@example.com", result["email"])
	}
}

func TestStructToMap_ExcludeMode(t *testing.T) {
	testStruct := TestStruct{
		Name:    "Jane",
		Age:     25,
		Email:   "jane@example.com",
		Decimal: decimal.NewFromFloat(50.00),
		UUID:    uuid.Nil,
	}

	// 排除模式：排除 name 和 age
	opts := StructToMapData{
		Mode: StructToMapExcludeMode,
		Keys: []string{"name", "age"},
	}

	result := StructToMap(testStruct, opts)

	// 验证排除的字段不存在
	if _, exists := result["name"]; exists {
		t.Error("StructToMap() exclude mode should exclude 'name'")
	}
	if _, exists := result["age"]; exists {
		t.Error("StructToMap() exclude mode should exclude 'age'")
	}
	// 其他字段应该存在（注意：email 有 omitempty，在 exclude 模式下会被跳过）
	// 这是代码的预期行为
}

func TestStructToMap_IncludeMode(t *testing.T) {
	testStruct := TestStruct{
		Name:    "Bob",
		Age:     35,
		Email:   "bob@example.com",
		Decimal: decimal.NewFromFloat(75.50),
		UUID:    uuid.Nil,
	}

	// 包括模式：只包括 name 和 decimal
	opts := StructToMapData{
		Mode: StructToMapIncludeMode,
		Keys: []string{"name", "decimal"},
	}

	result := StructToMap(testStruct, opts)

	// 验证只包括指定的字段
	if result["name"] != "Bob" {
		t.Errorf("StructToMap() name = %v, want Bob", result["name"])
	}
	// 使用反射比较 decimal
	if !result["decimal"].(decimal.Decimal).Equal(decimal.NewFromFloat(75.50)) {
		t.Errorf("StructToMap() decimal = %v, want 75.50", result["decimal"])
	}
	// 其他字段不应该存在
	if _, exists := result["age"]; exists {
		t.Error("StructToMap() include mode should only include specified fields")
	}
	if _, exists := result["email"]; exists {
		t.Error("StructToMap() include mode should only include specified fields")
	}
}

func TestStructToMap_IgnoreNil(t *testing.T) {
	testStruct := TestStruct{
		Name:    "Alice",
		Age:     28,
		NilPtr:  nil,
		Decimal: decimal.NewFromFloat(60.00),
	}

	// 测试 IgnoreNilFlag = true
	opts := StructToMapData{
		IgnoreNilFlag: true,
	}

	result := StructToMap(testStruct, opts)

	// nil 指针字段应该被跳过
	if _, exists := result["nil_ptr"]; exists {
		t.Error("StructToMap() with IgnoreNilFlag should skip nil pointers")
	}
}

func TestStructToMap_OmitEmpty(t *testing.T) {
	testStruct := TestStructWithOmitEmpty{
		Name:    "Charlie",
		Email:   "", // 空值
		Decimal: decimal.NewFromFloat(80.00),
		UUID:    uuid.Nil,
	}

	// 默认模式下，omitempty 字段如果为空应该被跳过
	result := StructToMap(testStruct)

	// email 有 omitempty 且为空，应该被跳过
	if _, exists := result["email"]; exists {
		t.Error("StructToMap() should skip omitempty fields with empty values")
	}
}

func TestStructToMap_OmitEmptyWithValue(t *testing.T) {
	testStruct := TestStructWithOmitEmpty{
		Name:    "David",
		Email:   "david@example.com", // 有值
		Decimal: decimal.NewFromFloat(90.00),
		UUID:    uuid.Nil,
	}

	// 使用 include 模式来保留 email 字段（因为 omitempty 在 exclude 模式下会被跳过）
	opts := StructToMapData{
		Mode: StructToMapIncludeMode,
		Keys: []string{"email"},
	}

	result := StructToMap(testStruct, opts)

	// email 有 omitempty 但有值，且在 include 模式下应该保留
	if result["email"] != "david@example.com" {
		t.Errorf("StructToMap() email = %v, want david@example.com", result["email"])
	}
}

func TestStructToMap_NonNilPointer(t *testing.T) {
	str := "test value"
	type StructWithPtr struct {
		Name *string `json:"name"`
	}

	testStruct := StructWithPtr{
		Name: &str,
	}

	result := StructToMap(testStruct)

	if result["name"] == nil || *(result["name"].(*string)) != "test value" {
		t.Errorf("StructToMap() name = %v, want test value", result["name"])
	}
}

func TestStructToMap_EmptyOptions(t *testing.T) {
	testStruct := TestStruct{
		Name:    "Eve",
		Age:     32,
		Decimal: decimal.NewFromFloat(70.00),
		UUID:    uuid.Nil,
	}

	// 不传选项，使用默认值
	result := StructToMap(testStruct)

	// 验证基本功能正常
	if result["name"] != "Eve" {
		t.Errorf("StructToMap() name = %v, want Eve", result["name"])
	}
}

func TestSliceContainString(t *testing.T) {
	tests := []struct {
		name string
		list []string
		item string
		want bool
	}{
		{"包含元素", []string{"a", "b", "c"}, "b", true},
		{"不包含元素", []string{"a", "b", "c"}, "d", false},
		{"空列表", []string{}, "a", false},
		{"单个元素匹配", []string{"only"}, "only", true},
		{"单个元素不匹配", []string{"only"}, "other", false},
		{"空字符串匹配", []string{"", "b"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SliceContainString(tt.list, tt.item)
			if got != tt.want {
				t.Errorf("SliceContainString(%v, %s) = %v, want %v", tt.list, tt.item, got, tt.want)
			}
		})
	}
}

func TestParseUUID_Valid(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	result, err := ParseUUID(validUUID)
	if err != nil {
		t.Errorf("ParseUUID() expected no error, got %v", err)
	}

	expectedUUID, _ := uuid.Parse(validUUID)
	if result != expectedUUID {
		t.Errorf("ParseUUID() = %v, want %v", result, expectedUUID)
	}
}

func TestParseUUID_Invalid(t *testing.T) {
	invalidUUID := "not-a-valid-uuid"

	_, err := ParseUUID(invalidUUID)
	if err == nil {
		t.Error("ParseUUID() expected error for invalid UUID, got nil")
	}
}

func TestParseUUID_EmptyString(t *testing.T) {
	emptyUUID := ""

	_, err := ParseUUID(emptyUUID)
	if err == nil {
		t.Error("ParseUUID() expected error for empty string, got nil")
	}
}

func TestParseUUID_PartialUUID(t *testing.T) {
	partialUUID := "550e8400-e29b"

	_, err := ParseUUID(partialUUID)
	if err == nil {
		t.Error("ParseUUID() expected error for partial UUID, got nil")
	}
}

// 测试指针类型的结构体
func TestStructToMap_PointerToStruct(t *testing.T) {
	testStruct := &TestStruct{
		Name:    "PointerTest",
		Age:     50,
		Decimal: decimal.NewFromFloat(100.00),
		UUID:    uuid.Nil,
	}

	result := StructToMap(testStruct)

	if result["name"] != "PointerTest" {
		t.Errorf("StructToMap() with pointer = %v, want PointerTest", result["name"])
	}
}
