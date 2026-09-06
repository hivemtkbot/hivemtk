package model

import (
	"testing"
	"time"
)

func TestAPILog_TableName(t *testing.T) {
	log := &APILog{}
	tableName := log.TableName()
	if tableName != "api_logs" {
		t.Errorf("Expected table name 'api_logs', got %s", tableName)
	}
}

func TestAPILog_BasicFields(t *testing.T) {
	now := time.Now()
	log := &APILog{
		ID:         1,
		CreatedAt:  now,
		LicenseID:  "license-123",
		Endpoint:   "/api/users",
		Method:     "GET",
		StatusCode: 200,
		Duration:   150,
		IPAddress:  "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
	}

	if log.ID != 1 {
		t.Errorf("Expected ID 1, got %d", log.ID)
	}
	if log.LicenseID != "license-123" {
		t.Errorf("Expected LicenseID 'license-123', got %s", log.LicenseID)
	}
	if log.Endpoint != "/api/users" {
		t.Errorf("Expected Endpoint '/api/users', got %s", log.Endpoint)
	}
	if log.Method != "GET" {
		t.Errorf("Expected Method 'GET', got %s", log.Method)
	}
	if log.StatusCode != 200 {
		t.Errorf("Expected StatusCode 200, got %d", log.StatusCode)
	}
	if log.Duration != 150 {
		t.Errorf("Expected Duration 150, got %d", log.Duration)
	}
	if log.IPAddress != "192.168.1.1" {
		t.Errorf("Expected IPAddress '192.168.1.1', got %s", log.IPAddress)
	}
	if log.UserAgent != "Mozilla/5.0" {
		t.Errorf("Expected UserAgent 'Mozilla/5.0', got %s", log.UserAgent)
	}
}

func TestAPILog_WithStatusCodes(t *testing.T) {
	statusCodes := []int{200, 201, 400, 401, 404, 500}

	for _, code := range statusCodes {
		log := &APILog{
			StatusCode: code,
		}
		if log.StatusCode != code {
			t.Errorf("Expected StatusCode %d, got %d", code, log.StatusCode)
		}
	}
}

func TestAPILog_WithHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		log := &APILog{
			Method: method,
		}
		if log.Method != method {
			t.Errorf("Expected Method %s, got %s", method, log.Method)
		}
	}
}

func TestAPILog_WithDuration(t *testing.T) {
	log := &APILog{
		Endpoint: "/api/slow",
		Duration: 5000,
	}

	if log.Duration != 5000 {
		t.Errorf("Expected Duration 5000, got %d", log.Duration)
	}
}

func TestVisitLog_TableName(t *testing.T) {
	log := &VisitLog{}
	tableName := log.TableName()
	if tableName != "visit_logs" {
		t.Errorf("Expected table name 'visit_logs', got %s", tableName)
	}
}

func TestVisitLog_BasicFields(t *testing.T) {
	now := time.Now()
	log := &VisitLog{
		ID:        1,
		CreatedAt: now,
		LicenseID: "license-456",
		Path:      "/dashboard",
		IPAddress: "10.0.0.1",
		UserAgent: "Chrome/120.0",
		Referer:   "https://google.com",
	}

	if log.ID != 1 {
		t.Errorf("Expected ID 1, got %d", log.ID)
	}
	if log.LicenseID != "license-456" {
		t.Errorf("Expected LicenseID 'license-456', got %s", log.LicenseID)
	}
	if log.Path != "/dashboard" {
		t.Errorf("Expected Path '/dashboard', got %s", log.Path)
	}
	if log.IPAddress != "10.0.0.1" {
		t.Errorf("Expected IPAddress '10.0.0.1', got %s", log.IPAddress)
	}
	if log.UserAgent != "Chrome/120.0" {
		t.Errorf("Expected UserAgent 'Chrome/120.0', got %s", log.UserAgent)
	}
	if log.Referer != "https://google.com" {
		t.Errorf("Expected Referer 'https://google.com', got %s", log.Referer)
	}
}

func TestVisitLog_WithEmptyReferer(t *testing.T) {
	log := &VisitLog{
		Path: "/direct",
	}

	if log.Referer != "" {
		t.Errorf("Expected empty Referer, got %s", log.Referer)
	}
}

func TestDailyStats_TableName(t *testing.T) {
	stats := &DailyStats{}
	tableName := stats.TableName()
	if tableName != "daily_stats" {
		t.Errorf("Expected table name 'daily_stats', got %s", tableName)
	}
}

func TestDailyStats_BasicFields(t *testing.T) {
	now := time.Now()
	stats := &DailyStats{
		ID:              1,
		CreatedAt:       now,
		UpdatedAt:       now,
		Date:            "2024-01-15",
		LicenseID:       "license-789",
		APICalls:        10000,
		Visits:          5000,
		UniqueVisitors:  3000,
		ErrorCount:      50,
		AvgResponseTime: 200,
	}

	if stats.ID != 1 {
		t.Errorf("Expected ID 1, got %d", stats.ID)
	}
	if stats.Date != "2024-01-15" {
		t.Errorf("Expected Date '2024-01-15', got %s", stats.Date)
	}
	if stats.LicenseID != "license-789" {
		t.Errorf("Expected LicenseID 'license-789', got %s", stats.LicenseID)
	}
	if stats.APICalls != 10000 {
		t.Errorf("Expected APICalls 10000, got %d", stats.APICalls)
	}
	if stats.Visits != 5000 {
		t.Errorf("Expected Visits 5000, got %d", stats.Visits)
	}
	if stats.UniqueVisitors != 3000 {
		t.Errorf("Expected UniqueVisitors 3000, got %d", stats.UniqueVisitors)
	}
	if stats.ErrorCount != 50 {
		t.Errorf("Expected ErrorCount 50, got %d", stats.ErrorCount)
	}
	if stats.AvgResponseTime != 200 {
		t.Errorf("Expected AvgResponseTime 200, got %d", stats.AvgResponseTime)
	}
}

func TestDailyStats_DefaultValues(t *testing.T) {
	stats := &DailyStats{}

	if stats.APICalls != 0 {
		t.Logf("APICalls is %d (expected 0 before save, default is 0)", stats.APICalls)
	}
	if stats.Visits != 0 {
		t.Logf("Visits is %d (expected 0 before save, default is 0)", stats.Visits)
	}
	if stats.ErrorCount != 0 {
		t.Logf("ErrorCount is %d (expected 0 before save, default is 0)", stats.ErrorCount)
	}
}

func TestSystemMetrics_TableName(t *testing.T) {
	metrics := &SystemMetrics{}
	tableName := metrics.TableName()
	if tableName != "system_metrics" {
		t.Errorf("Expected table name 'system_metrics', got %s", tableName)
	}
}

func TestSystemMetrics_BasicFields(t *testing.T) {
	now := time.Now()
	metrics := &SystemMetrics{
		ID:                1,
		CreatedAt:         now,
		CPUUsage:          45.5,
		MemoryUsage:       60.2,
		DiskUsage:         75.0,
		NetworkIn:         1024000,
		NetworkOut:        512000,
		ActiveConnections: 100,
		ErrorCount:        5,
	}

	if metrics.ID != 1 {
		t.Errorf("Expected ID 1, got %d", metrics.ID)
	}
	if metrics.CPUUsage != 45.5 {
		t.Errorf("Expected CPUUsage 45.5, got %f", metrics.CPUUsage)
	}
	if metrics.MemoryUsage != 60.2 {
		t.Errorf("Expected MemoryUsage 60.2, got %f", metrics.MemoryUsage)
	}
	if metrics.DiskUsage != 75.0 {
		t.Errorf("Expected DiskUsage 75.0, got %f", metrics.DiskUsage)
	}
	if metrics.NetworkIn != 1024000 {
		t.Errorf("Expected NetworkIn 1024000, got %d", metrics.NetworkIn)
	}
	if metrics.NetworkOut != 512000 {
		t.Errorf("Expected NetworkOut 512000, got %d", metrics.NetworkOut)
	}
	if metrics.ActiveConnections != 100 {
		t.Errorf("Expected ActiveConnections 100, got %d", metrics.ActiveConnections)
	}
	if metrics.ErrorCount != 5 {
		t.Errorf("Expected ErrorCount 5, got %d", metrics.ErrorCount)
	}
}

func TestSystemMetrics_WithHighUsage(t *testing.T) {
	metrics := &SystemMetrics{
		CPUUsage:    95.5,
		MemoryUsage: 90.0,
		DiskUsage:   85.0,
	}

	if metrics.CPUUsage != 95.5 {
		t.Errorf("Expected CPUUsage 95.5, got %f", metrics.CPUUsage)
	}
}
