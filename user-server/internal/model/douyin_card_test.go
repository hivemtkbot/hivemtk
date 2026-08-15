package model

import (
	"testing"
	"time"
)

func TestDouyinCard_TableName(t *testing.T) {
	card := &DouyinCard{}
	tableName := card.TableName()
	if tableName != "douyin_cards" {
		t.Errorf("Expected table name 'douyin_cards', got %s", tableName)
	}
}

func TestDouyinCard_BasicFields(t *testing.T) {
	now := time.Now()
	card := &DouyinCard{
		ID:           1,
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://example.com/redirect",
		ShortLinkID:  100,
		DomainPoolID: 200,
		Tags:         "tag1,tag2,tag3",
		ViewCount:    1000,
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
	if !card.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestDouyinCard_DefaultValues(t *testing.T) {
	card := &DouyinCard{}

	if card.ShortLinkID != 0 {
		t.Logf("ShortLinkID is %d (expected 0 before save, default is 0)", card.ShortLinkID)
	}
	if card.DomainPoolID != 0 {
		t.Logf("DomainPoolID is %d (expected 0 before save, default is 0)", card.DomainPoolID)
	}
	if card.ViewCount != 0 {
		t.Logf("ViewCount is %d (expected 0 before save, default is 0)", card.ViewCount)
	}
	if card.IsActive != false {
		t.Logf("IsActive is %v (expected false before save, default is true)", card.IsActive)
	}
}

func TestDouyinCard_IsActiveCard(t *testing.T) {
	activeCard := &DouyinCard{
		IsActive: true,
	}
	if !activeCard.IsActive {
		t.Error("Expected IsActiveCard() to return true for active card")
	}

	inactiveCard := &DouyinCard{
		IsActive: false,
	}
	if inactiveCard.IsActive {
		t.Error("Expected IsActiveCard() to return false for inactive card")
	}
}

func TestDouyinCard_WithEmptyShortLinkID(t *testing.T) {
	card := &DouyinCard{
		Title:       "Test Card",
		ShortLinkID: 0, 
	}

	if card.ShortLinkID != 0 {
		t.Errorf("Expected ShortLinkID 0, got %d", card.ShortLinkID)
	}
}

func TestDouyinCard_WithEmptyDomainPoolID(t *testing.T) {
	card := &DouyinCard{
		Title:        "Test Card",
		DomainPoolID: 0, 
	}

	if card.DomainPoolID != 0 {
		t.Errorf("Expected DomainPoolID 0, got %d", card.DomainPoolID)
	}
}

func TestDouyinCard_WithMultipleTags(t *testing.T) {
	card := &DouyinCard{
		Tags: "抖音，卡片，营销，推广",
	}

	if card.Tags != "抖音，卡片，营销，推广" {
		t.Errorf("Expected Tags '抖音，卡片，营销，推广', got %s", card.Tags)
	}
}

func TestDouyinCard_WithHighViewCount(t *testing.T) {
	card := &DouyinCard{
		Title:     "Viral Card",
		ViewCount: 1000000,
	}

	if card.ViewCount != 1000000 {
		t.Errorf("Expected ViewCount 1000000, got %d", card.ViewCount)
	}
}

func TestDouyinCard_WithURLs(t *testing.T) {
	card := &DouyinCard{
		ImageURL:    "https://cdn.example.com/images/card1.jpg",
		RedirectURL: "https://m.example.com/product/123",
	}

	if card.ImageURL != "https://cdn.example.com/images/card1.jpg" {
		t.Errorf("Expected ImageURL, got %s", card.ImageURL)
	}
	if card.RedirectURL != "https://m.example.com/product/123" {
		t.Errorf("Expected RedirectURL, got %s", card.RedirectURL)
	}
}

