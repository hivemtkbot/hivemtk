package utils

import "testing"

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		input    bool
		expected bool
	}{
		{true, true},
		{false, false},
	}

	for _, tt := range tests {
		result := BoolPtr(tt.input)
		if result == nil {
			t.Errorf("BoolPtr(%v) returned nil", tt.input)
		}
		if *result != tt.expected {
			t.Errorf("BoolPtr(%v) = %v, expected %v", tt.input, *result, tt.expected)
		}
	}
}

func TestGetBoolValue(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name       string
		input      *bool
		defaultVal bool
		expected   bool
	}{
		{"nil pointer with true default", nil, true, true},
		{"nil pointer with false default", nil, false, false},
		{"true value", &trueVal, false, true},
		{"false value", &falseVal, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBoolValue(tt.input, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("GetBoolValue(%v, %v) = %v, expected %v", tt.input, tt.defaultVal, result, tt.expected)
			}
		})
	}
}
