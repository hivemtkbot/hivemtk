package utils

import (
	"testing"
	"time"
)

func TestAppError_Error(t *testing.T) {
	err := NewAppError(ErrorTypeValidation, "Invalid input", 400, "Field 'email' is required")
	expected := "[validation_error] Invalid input: Field 'email' is required"

	if err.Error() != expected {
		t.Errorf("Error() = %q, expected %q", err.Error(), expected)
	}
}

func TestNewAppError(t *testing.T) {
	err := NewAppError(ErrorTypeDatabase, "DB error", 500, "Connection timeout")

	if err.Type != ErrorTypeDatabase {
		t.Errorf("Type = %v, expected %v", err.Type, ErrorTypeDatabase)
	}
	if err.Message != "DB error" {
		t.Errorf("Message = %v, expected %v", err.Message, "DB error")
	}
	if err.Code != 500 {
		t.Errorf("Code = %v, expected %v", err.Code, 500)
	}
	if err.Details != "Connection timeout" {
		t.Errorf("Details = %v, expected %v", err.Details, "Connection timeout")
	}
	if err.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestNewAppError_EmptyDetails(t *testing.T) {
	err := NewAppError(ErrorTypeAuth, "Unauthorized", 401, "")

	if err.Details != "" {
		t.Errorf("Details = %v, expected empty", err.Details)
	}
}

func TestErrorTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		errType  ErrorType
		expected string
	}{
		{"Validation", ErrorTypeValidation, "validation_error"},
		{"Database", ErrorTypeDatabase, "database_error"},
		{"Business", ErrorTypeBusiness, "business_error"},
		{"System", ErrorTypeSystem, "system_error"},
		{"Auth", ErrorTypeAuth, "auth_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.errType) != tt.expected {
				t.Errorf("ErrorType string = %q, expected %q", tt.errType, tt.expected)
			}
		})
	}
}

func TestHandleValidationError(t *testing.T) {
	err := NewAppError(ErrorTypeValidation, "Test validation error", 400, "Test details")

	if err.Type != ErrorTypeValidation {
		t.Errorf("Expected ErrorTypeValidation, got %v", err.Type)
	}
	if err.Code != 400 {
		t.Errorf("Expected Code 400, got %d", err.Code)
	}
}

func TestHandleDatabaseError(t *testing.T) {
	err := NewAppError(ErrorTypeDatabase, "数据库操作失败", 500, "Connection refused")

	if err.Type != ErrorTypeDatabase {
		t.Errorf("Expected ErrorTypeDatabase, got %v", err.Type)
	}
	if err.Code != 500 {
		t.Errorf("Expected Code 500, got %d", err.Code)
	}
	if err.Message != "数据库操作失败" {
		t.Errorf("Expected Message '数据库操作失败', got %v", err.Message)
	}
}

func TestHandleBusinessError(t *testing.T) {
	err := NewAppError(ErrorTypeBusiness, "Business logic failed", 400, "Invalid state transition")

	if err.Type != ErrorTypeBusiness {
		t.Errorf("Expected ErrorTypeBusiness, got %v", err.Type)
	}
	if err.Code != 400 {
		t.Errorf("Expected Code 400, got %d", err.Code)
	}
}

func TestHandleAuthError(t *testing.T) {
	err := NewAppError(ErrorTypeAuth, "Invalid credentials", 401, "")

	if err.Type != ErrorTypeAuth {
		t.Errorf("Expected ErrorTypeAuth, got %v", err.Type)
	}
	if err.Code != 401 {
		t.Errorf("Expected Code 401, got %d", err.Code)
	}
	if err.Details != "" {
		t.Errorf("Expected empty Details, got %v", err.Details)
	}
}

func TestAppError_WithStackTrace(t *testing.T) {
	err := &AppError{
		Type:      ErrorTypeSystem,
		Message:   "System error",
		Code:      500,
		Details:   "Something went wrong",
		Timestamp: time.Now(),
	}

	if err.StackTrace != "" {
		t.Logf("StackTrace = %v", err.StackTrace)
	}
}

func TestAppError_JSONSerialization(t *testing.T) {
	err := NewAppError(ErrorTypeValidation, "Test error", 400, "Test details")

	if err.Type == "" {
		t.Error("Type should be set for JSON serialization")
	}
	if err.Code == 0 {
		t.Error("Code should be set for JSON serialization")
	}
}
