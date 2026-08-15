package model

import (
	"testing"
)

func TestShortLinkAccess_TableName(t *testing.T) {
	access := &ShortLinkAccess{}
	tableName := access.TableName()
	if tableName != "short_link_accesses" {
		t.Errorf("Expected table name 'short_link_accesses', got %s", tableName)
	}
}

func TestShortLinkAccess_BasicFields(t *testing.T) {
	access := &ShortLinkAccess{
		ID:          1,
		ShortLinkID: 100,
		IP:          "192.168.1.1",
		UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0",
		Referer:     "https://google.com",
		DeviceType:  "desktop",
		Browser:     "Chrome",
		OS:          "Windows",
		Location:    "Beijing, CN",
	}

	if access.ID != 1 {
		t.Errorf("Expected ID 1, got %d", access.ID)
	}
	if access.ShortLinkID != 100 {
		t.Errorf("Expected ShortLinkID 100, got %d", access.ShortLinkID)
	}
	if access.IP != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got %s", access.IP)
	}
	if access.UserAgent != "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0" {
		t.Errorf("Expected UserAgent, got %s", access.UserAgent)
	}
	if access.Referer != "https://google.com" {
		t.Errorf("Expected Referer 'https://google.com', got %s", access.Referer)
	}
	if access.DeviceType != "desktop" {
		t.Errorf("Expected DeviceType 'desktop', got %s", access.DeviceType)
	}
	if access.Browser != "Chrome" {
		t.Errorf("Expected Browser 'Chrome', got %s", access.Browser)
	}
	if access.OS != "Windows" {
		t.Errorf("Expected OS 'Windows', got %s", access.OS)
	}
	if access.Location != "Beijing, CN" {
		t.Errorf("Expected Location 'Beijing, CN', got %s", access.Location)
	}
}

func TestShortLinkAccess_WithDeviceTypes(t *testing.T) {
	deviceTypes := []string{"mobile", "desktop", "tablet"}

	for _, deviceType := range deviceTypes {
		access := &ShortLinkAccess{
			DeviceType: deviceType,
		}
		if access.DeviceType != deviceType {
			t.Errorf("Expected DeviceType %s, got %s", deviceType, access.DeviceType)
		}
	}
}

func TestShortLinkAccess_WithEmptyReferer(t *testing.T) {
	access := &ShortLinkAccess{
		IP: "10.0.0.1",
	}

	if access.Referer != "" {
		t.Errorf("Expected empty Referer, got %s", access.Referer)
	}
}

func TestShortLinkAccess_WithMobileDevice(t *testing.T) {
	access := &ShortLinkAccess{
		ShortLinkID: 200,
		IP:          "10.0.0.2",
		DeviceType:  "mobile",
		Browser:     "Safari",
		OS:          "iOS",
		UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
	}

	if access.DeviceType != "mobile" {
		t.Errorf("Expected DeviceType 'mobile', got %s", access.DeviceType)
	}
	if access.OS != "iOS" {
		t.Errorf("Expected OS 'iOS', got %s", access.OS)
	}
}

func TestShortLinkAccess_WithTabletDevice(t *testing.T) {
	access := &ShortLinkAccess{
		ShortLinkID: 300,
		IP:          "10.0.0.3",
		DeviceType:  "tablet",
		Browser:     "Safari",
		OS:          "iPadOS",
	}

	if access.DeviceType != "tablet" {
		t.Errorf("Expected DeviceType 'tablet', got %s", access.DeviceType)
	}
}

func TestShortLinkAccess_WithEmptyLocation(t *testing.T) {
	access := &ShortLinkAccess{
		IP: "10.0.0.4",
	}

	if access.Location != "" {
		t.Errorf("Expected empty Location, got %s", access.Location)
	}
}

