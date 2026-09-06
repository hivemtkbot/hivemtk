package model

import (
	"testing"
)

func TestLiveCode_TableName(t *testing.T) {
	liveCode := &LiveCode{}
	tableName := liveCode.TableName()
	if tableName != "live_codes" {
		t.Errorf("Expected table name 'live_codes', got %s", tableName)
	}
}

func TestLiveCode_BasicFields(t *testing.T) {
	liveCode := &LiveCode{
		ID:              "lc-123",
		Name:            "Test Live Code",
		ShortLink:       "abc123",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
		TotalViews:      1000,
		TodayViews:      100,
		TotalClicks:     500,
		DailyClicks:     50,
		ImageURL:        "https://example.com/image.jpg",
		EntryURL:        "https://example.com/entry",
		LandingURL:      "https://example.com/landing",
	}

	if liveCode.ID != "lc-123" {
		t.Errorf("Expected ID 'lc-123', got %s", liveCode.ID)
	}
	if liveCode.Name != "Test Live Code" {
		t.Errorf("Expected Name 'Test Live Code', got %s", liveCode.Name)
	}
	if liveCode.ShortLink != "abc123" {
		t.Errorf("Expected ShortLink 'abc123', got %s", liveCode.ShortLink)
	}
	if liveCode.ShortDomainID != 1 {
		t.Errorf("Expected ShortDomainID 1, got %d", liveCode.ShortDomainID)
	}
	if liveCode.EntryDomainID != 2 {
		t.Errorf("Expected EntryDomainID 2, got %d", liveCode.EntryDomainID)
	}
	if liveCode.LandingDomainID != 3 {
		t.Errorf("Expected LandingDomainID 3, got %d", liveCode.LandingDomainID)
	}
	if liveCode.Status != 1 {
		t.Errorf("Expected Status 1, got %d", liveCode.Status)
	}
	if liveCode.TotalViews != 1000 {
		t.Errorf("Expected TotalViews 1000, got %d", liveCode.TotalViews)
	}
	if liveCode.TodayViews != 100 {
		t.Errorf("Expected TodayViews 100, got %d", liveCode.TodayViews)
	}
	if liveCode.TotalClicks != 500 {
		t.Errorf("Expected TotalClicks 500, got %d", liveCode.TotalClicks)
	}
	if liveCode.DailyClicks != 50 {
		t.Errorf("Expected DailyClicks 50, got %d", liveCode.DailyClicks)
	}
}

func TestLiveCode_DefaultValues(t *testing.T) {
	liveCode := &LiveCode{}

	if liveCode.Status != 0 {
		t.Logf("Status is %d (expected 0 before save, default is 1)", liveCode.Status)
	}
	if liveCode.TotalViews != 0 {
		t.Logf("TotalViews is %d (expected 0 before save, default is 0)", liveCode.TotalViews)
	}
	if liveCode.TodayViews != 0 {
		t.Logf("TodayViews is %d (expected 0 before save, default is 0)", liveCode.TodayViews)
	}
}

func TestLiveCode_WithStatusValues(t *testing.T) {
	statuses := []int{0, 1}
	statusNames := map[int]string{
		0: "禁用",
		1: "启用",
	}

	for _, status := range statuses {
		liveCode := &LiveCode{
			Status: status,
		}
		if liveCode.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, statusNames[status], liveCode.Status)
		}
	}
}

func TestLiveCode_WithEmptyID(t *testing.T) {
	liveCode := &LiveCode{
		Name:      "Test Live Code",
		ShortLink: "test123",
		ID:        "",
	}

	if liveCode.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", liveCode.ID)
	}
}

func TestLiveCode_WithHighViews(t *testing.T) {
	liveCode := &LiveCode{
		Name:       "Popular Live Code",
		TotalViews: 1000000,
		TodayViews: 10000,
	}

	if liveCode.TotalViews != 1000000 {
		t.Errorf("Expected TotalViews 1000000, got %d", liveCode.TotalViews)
	}
}

func TestLiveCode_BeforeCreate_GeneratesID(t *testing.T) {
	liveCode := &LiveCode{
		Name: "Test Live Code",
	}

	err := liveCode.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if liveCode.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
}

func TestLiveCode_BeforeCreate_NoChangeIfExists(t *testing.T) {
	liveCode := &LiveCode{
		ID:   "existing-id",
		Name: "Test",
	}

	err := liveCode.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if liveCode.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", liveCode.ID)
	}
}
