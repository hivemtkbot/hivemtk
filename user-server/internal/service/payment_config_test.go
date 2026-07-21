package service

import (
	"testing"
)

func TestSafeParseJSON_Empty(t *testing.T) {
	got := safeParseJSON("")
	if got == nil {
		t.Error("expected non-nil map for empty string")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSafeParseJSON_Valid(t *testing.T) {
	got := safeParseJSON(`{"key":"value","num":42}`)
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got["key"])
	}
	if got["num"] != float64(42) {
		t.Errorf("expected num=42, got %v", got["num"])
	}
}

func TestSafeParseJSON_Invalid(t *testing.T) {
	got := safeParseJSON("not-json")
	if got == nil {
		t.Error("expected non-nil map for invalid json")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestDefaultPaymentConfig(t *testing.T) {
	cfg := defaultPaymentConfig()
	if cfg.DefaultMethod != "alipay" {
		t.Errorf("expected alipay, got %q", cfg.DefaultMethod)
	}
	if cfg.Timeout != 30 {
		t.Errorf("expected 30, got %d", cfg.Timeout)
	}
	if !cfg.AutoConfirm {
		t.Error("expected autoConfirm true")
	}
	if cfg.Alipay == nil {
		t.Error("expected alipay config")
	}
}
