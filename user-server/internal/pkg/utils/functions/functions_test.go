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
	NoTag   string          
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

	if result["name"] != "John" {
		t.Errorf("StructToMap() name = %v, want John", result["name"])
	}
	if !result["decimal"].(decimal.Decimal).Equal(decimal.NewFromFloat(99.99)) {
		t.Errorf("StructToMap() decimal = %v, want 99.99", result["decimal"])
	}
	if result["uuid"] != nil {
		t.Errorf("StructToMap() uuid = %v, want nil", result["uuid"])
	}
	if _, exists := result["NoTag"]; exists {
		t.Error("StructToMap() should skip fields without json tag")
	}
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

	opts := StructToMapData{
		Mode: StructToMapExcludeMode,
		Keys: []string{"name", "age"},
	}

	result := StructToMap(testStruct, opts)

	if _, exists := result["name"]; exists {
		t.Error("StructToMap() exclude mode should exclude 'name'")
	}
	if _, exists := result["age"]; exists {
		t.Error("StructToMap() exclude mode should exclude 'age'")
	}
}

func TestStructToMap_IncludeMode(t *testing.T) {
	testStruct := TestStruct{
		Name:    "Bob",
		Age:     35,
		Email:   "bob@example.com",
		Decimal: decimal.NewFromFloat(75.50),
		UUID:    uuid.Nil,
	}

	opts := StructToMapData{
		Mode: StructToMapIncludeMode,
		Keys: []string{"name", "decimal"},
	}

	result := StructToMap(testStruct, opts)

	if result["name"] != "Bob" {
		t.Errorf("StructToMap() name = %v, want Bob", result["name"])
	}
	if !result["decimal"].(decimal.Decimal).Equal(decimal.NewFromFloat(75.50)) {
		t.Errorf("StructToMap() decimal = %v, want 75.50", result["decimal"])
	}
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

	opts := StructToMapData{
		IgnoreNilFlag: true,
	}

	result := StructToMap(testStruct, opts)

	if _, exists := result["nil_ptr"]; exists {
		t.Error("StructToMap() with IgnoreNilFlag should skip nil pointers")
	}
}

func TestStructToMap_OmitEmpty(t *testing.T) {
	testStruct := TestStructWithOmitEmpty{
		Name:    "Charlie",
		Email:   "", 
		Decimal: decimal.NewFromFloat(80.00),
		UUID:    uuid.Nil,
	}

	result := StructToMap(testStruct)

	if _, exists := result["email"]; exists {
		t.Error("StructToMap() should skip omitempty fields with empty values")
	}
}

func TestStructToMap_OmitEmptyWithValue(t *testing.T) {
	testStruct := TestStructWithOmitEmpty{
		Name:    "David",
		Email:   "david@example.com", 
		Decimal: decimal.NewFromFloat(90.00),
		UUID:    uuid.Nil,
	}

	opts := StructToMapData{
		Mode: StructToMapIncludeMode,
		Keys: []string{"email"},
	}

	result := StructToMap(testStruct, opts)

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

	result := StructToMap(testStruct)

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

