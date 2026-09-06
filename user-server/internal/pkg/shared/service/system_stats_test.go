package service

import (
	"testing"
)

func TestNewSystemStatsService(t *testing.T) {
	svc := NewSystemStatsService()
	if svc == nil {
		t.Error("Expected non-nil SystemStatsService")
	}
}

func TestSystemStatsService_GetSystemInfo(t *testing.T) {
	svc := NewSystemStatsService()
	info, err := svc.GetSystemInfo()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Error("Expected non-nil SystemInfo")
	}
	if info.GoVersion == "" {
		t.Error("Expected non-empty GoVersion")
	}
	if info.Hostname == "" {
		t.Error("Expected non-empty Hostname")
	}
}
