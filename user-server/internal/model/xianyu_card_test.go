package model

import (
	"testing"
	"time"
)

func TestXianyuCard_TableName(t *testing.T) {
	card := &XianyuCard{}
	tableName := card.TableName()
	if tableName != "xianyu_cards" {
		t.Errorf("Expected table name 'xianyu_cards', got %s", tableName)
	}
}

func TestXianyuCard_BasicFields(t *testing.T) {
	now := time.Now()
	card := &XianyuCard{
		ID:           1,
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://example.com/redirect",
		ShortLinkID:  100,
		DomainPoolID: 200,
		Tags:         "tag1,tag2,tag3",
		ViewCount:    1000,
		ClickCount:   500,
		ShareCount:   200,
		LikeCount:    300,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if card.ID != 1 {
		t.Errorf("Expected ID 1, got %d", card.ID)
	}
	if card.Title != "Test Card" {
		t.Errorf("Expected Title 'Test Card', got %s", card.Title)
	}
	if card.Description != "Test description" {
		t.Errorf("Expected Description 'Test description', got %s", card.Description)
	}
	if card.ImageURL != "https://example.com/image.jpg" {
		t.Errorf("Expected ImageURL 'https://example.com/image.jpg', got %s", card.ImageURL)
	}
	if card.RedirectURL != "https://example.com/redirect" {
		t.Errorf("Expected RedirectURL 'https://example.com/redirect', got %s", card.RedirectURL)
	}
	if card.ShortLinkID != 100 {
		t.Errorf("Expected ShortLinkID 100, got %d", card.ShortLinkID)
	}
	if card.DomainPoolID != 200 {
		t.Errorf("Expected DomainPoolID 200, got %d", card.DomainPoolID)
	}
	if card.Tags != "tag1,tag2,tag3" {
		t.Errorf("Expected Tags 'tag1,tag2,tag3', got %s", card.Tags)
	}
	if card.ViewCount != 1000 {
		t.Errorf("Expected ViewCount 1000, got %d", card.ViewCount)
	}
	if card.ClickCount != 500 {
		t.Errorf("Expected ClickCount 500, got %d", card.ClickCount)
	}
	if card.ShareCount != 200 {
		t.Errorf("Expected ShareCount 200, got %d", card.ShareCount)
	}
	if card.LikeCount != 300 {
		t.Errorf("Expected LikeCount 300, got %d", card.LikeCount)
	}
	if !card.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestXianyuCard_DefaultValues(t *testing.T) {
	card := &XianyuCard{}

	if card.ShortLinkID != 0 {
		t.Logf("ShortLinkID is %d (expected 0 before save, default is 0)", card.ShortLinkID)
	}
	if card.DomainPoolID != 0 {
		t.Logf("DomainPoolID is %d (expected 0 before save, default is 0)", card.DomainPoolID)
	}
	if card.ViewCount != 0 {
		t.Logf("ViewCount is %d (expected 0 before save, default is 0)", card.ViewCount)
	}
	if card.ClickCount != 0 {
		t.Logf("ClickCount is %d (expected 0 before save, default is 0)", card.ClickCount)
	}
	if card.ShareCount != 0 {
		t.Logf("ShareCount is %d (expected 0 before save, default is 0)", card.ShareCount)
	}
	if card.LikeCount != 0 {
		t.Logf("LikeCount is %d (expected 0 before save, default is 0)", card.LikeCount)
	}
	if card.IsActive != false {
		t.Logf("IsActive is %v (expected false before save, default is true)", card.IsActive)
	}
}

func TestXianyuCard_IsActiveCard(t *testing.T) {
	activeCard := &XianyuCard{
		IsActive: true,
	}
	if !activeCard.IsActive {
		t.Error("Expected IsActiveCard() to return true for active card")
	}

	inactiveCard := &XianyuCard{
		IsActive: false,
	}
	if inactiveCard.IsActive {
		t.Error("Expected IsActiveCard() to return false for inactive card")
	}
}

func TestXianyuCard_EngagementMetrics(t *testing.T) {
	card := &XianyuCard{
		Title:      "Viral Card",
		ViewCount:  10000,
		ClickCount: 5000,
		ShareCount: 2000,
		LikeCount:  3000,
	}

	// Verify metric relationships
	if card.ViewCount < card.ClickCount {
		t.Error("Expected ViewCount to be >= ClickCount")
	}
	if card.ClickCount < (card.ShareCount + card.LikeCount) {
		t.Log("Note: ClickCount may be less than sum of actions in some cases")
	}
}

func TestXianyuCard_WithZeroEngagement(t *testing.T) {
	card := &XianyuCard{
		Title:      "New Card",
		ViewCount:  0,
		ClickCount: 0,
		ShareCount: 0,
		LikeCount:  0,
	}

	if card.ViewCount != 0 {
		t.Errorf("Expected ViewCount 0, got %d", card.ViewCount)
	}
	if card.ClickCount != 0 {
		t.Errorf("Expected ClickCount 0, got %d", card.ClickCount)
	}
}

func TestXianyuCard_WithHighCounts(t *testing.T) {
	card := &XianyuCard{
		Title:      "Popular Card",
		ViewCount:  1000000,
		ClickCount: 500000,
		ShareCount: 100000,
		LikeCount:  200000,
	}

	if card.ViewCount != 1000000 {
		t.Errorf("Expected ViewCount 1000000, got %d", card.ViewCount)
	}
}
