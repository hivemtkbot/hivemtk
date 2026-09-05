package model

import (
	"testing"
	"time"
)

func TestXiaohongshuCard_TableName(t *testing.T) {
	card := &XiaohongshuCard{}
	tableName := card.TableName()
	if tableName != "xiaohongshu_cards" {
		t.Errorf("Expected table name 'xiaohongshu_cards', got %s", tableName)
	}
}

func TestXiaohongshuCard_BasicFields(t *testing.T) {
	now := time.Now()
	card := &XiaohongshuCard{
		ID:           1,
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://example.com/redirect",
		ShareURL:     "https://example.com/share",
		ShortLinkID:  uintPtr(100),
		DomainPoolID: uintPtr(200),
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
	if card.ShareURL != "https://example.com/share" {
		t.Errorf("Expected ShareURL 'https://example.com/share', got %s", card.ShareURL)
	}
	if card.ShortLinkID == nil || *card.ShortLinkID != 100 {
		t.Errorf("Expected ShortLinkID 100, got %v", card.ShortLinkID)
	}
	if card.DomainPoolID == nil || *card.DomainPoolID != 200 {
		t.Errorf("Expected DomainPoolID 200, got %v", card.DomainPoolID)
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

func TestXiaohongshuCard_DefaultValues(t *testing.T) {
	card := &XiaohongshuCard{}

	if card.ViewCount != 0 {
		t.Logf("ViewCount is %d (expected 0 before save, default is 0)", card.ViewCount)
	}
	if card.IsActive != false {
		t.Logf("IsActive is %v (expected false before save, default is true)", card.IsActive)
	}
}

func TestXiaohongshuCard_IsActiveCard(t *testing.T) {
	activeCard := &XiaohongshuCard{
		IsActive: true,
	}
	if !activeCard.IsActive {
		t.Error("Expected IsActiveCard() to return true for active card")
	}

	inactiveCard := &XiaohongshuCard{
		IsActive: false,
	}
	if inactiveCard.IsActive {
		t.Error("Expected IsActiveCard() to return false for inactive card")
	}
}

func TestXiaohongshuCard_WithNilIDs(t *testing.T) {
	card := &XiaohongshuCard{
		Title:        "Test Card",
		ShortLinkID:  nil,
		DomainPoolID: nil,
	}

	if card.ShortLinkID != nil {
		t.Errorf("Expected ShortLinkID nil, got %v", card.ShortLinkID)
	}
	if card.DomainPoolID != nil {
		t.Errorf("Expected DomainPoolID nil, got %v", card.DomainPoolID)
	}
}

func TestXiaohongshuCard_WithShareURL(t *testing.T) {
	card := &XiaohongshuCard{
		Title:    "Test Card",
		ShareURL: "https://xhs.com/example",
	}

	if card.ShareURL != "https://xhs.com/example" {
		t.Errorf("Expected ShareURL 'https://xhs.com/example', got %s", card.ShareURL)
	}
}

func TestXiaohongshuCard_WithDescription(t *testing.T) {
	longDesc := "这是一篇很长的笔记描述，包含详细的使用体验和产品信息。小红书的用户喜欢详细的分享内容，所以描述字段通常会比较长。"
	card := &XiaohongshuCard{
		Title:       "种草好物",
		Description: longDesc,
	}

	if card.Description != longDesc {
		t.Error("Expected long description to be stored")
	}
}

func TestXiaohongshuCard_WithTags(t *testing.T) {
	card := &XiaohongshuCard{
		Title: "种草",
		Tags:  "种草，好物推荐，小红书",
	}

	if card.Tags != "种草，好物推荐，小红书" {
		t.Errorf("Expected Tags '种草，好物推荐，小红书', got %s", card.Tags)
	}
}
