package dto

import (
	"testing"
	"time"
)

func TestCreateLiveCodeRequest_Fields(t *testing.T) {
	req := CreateLiveCodeRequest{
		Name:            "Test",
		ShortLink:       "test-link",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
	}
	if req.Name != "Test" {
		t.Errorf("Expected Name 'Test', got '%s'", req.Name)
	}
	if req.ShortLink != "test-link" {
		t.Errorf("Expected ShortLink 'test-link', got '%s'", req.ShortLink)
	}
}

func TestUpdateLiveCodeRequest_Fields(t *testing.T) {
	req := UpdateLiveCodeRequest{
		Name:      "Updated",
		ShortLink: "updated-link",
	}
	if req.Name != "Updated" {
		t.Errorf("Expected Name 'Updated', got '%s'", req.Name)
	}
}

func TestLiveCodeResponse_Fields(t *testing.T) {
	now := time.Now()
	resp := LiveCodeResponse{
		ID:              "1",
		Name:            "Test",
		ShortLink:       "test-link",
		ShortDomainID:   1,
		EntryDomainID:   2,
		LandingDomainID: 3,
		Status:          1,
		CreatedAt:       now,
	}
	if resp.ID != "1" {
		t.Errorf("Expected ID '1', got '%s'", resp.ID)
	}
	if resp.CreatedAt != now {
		t.Error("Expected CreatedAt to match")
	}
}

func TestAccessShortLinkRequest_Fields(t *testing.T) {
	req := AccessShortLinkRequest{
		ShortCode: "abc123",
		UserAgent: "Mozilla/5.0",
		IP:        "127.0.0.1",
		Referer:   "https://example.com",
	}
	if req.ShortCode != "abc123" {
		t.Errorf("Expected ShortCode 'abc123', got '%s'", req.ShortCode)
	}
}

func TestGenerateQRCodeRequest_Fields(t *testing.T) {
	req := GenerateQRCodeRequest{
		ExpireDays: 30,
		Status:     1,
	}
	if req.ExpireDays != 30 {
		t.Errorf("Expected ExpireDays 30, got %d", req.ExpireDays)
	}
}

func TestShareLiveCodeRequest_Fields(t *testing.T) {
	req := ShareLiveCodeRequest{
		IPAddress: "127.0.0.1",
		UserAgent: "Mozilla/5.0",
	}
	if req.IPAddress != "127.0.0.1" {
		t.Errorf("Expected IPAddress '127.0.0.1', got '%s'", req.IPAddress)
	}
}

func TestLiveCodeListResponse_Fields(t *testing.T) {
	resp := LiveCodeListResponse{
		Total: 10,
		List:  []*LiveCodeResponse{},
	}
	if resp.Total != 10 {
		t.Errorf("Expected Total 10, got %d", resp.Total)
	}
	if len(resp.List) != 0 {
		t.Errorf("Expected empty List, got %d items", len(resp.List))
	}
}

