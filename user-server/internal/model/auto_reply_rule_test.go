package model

import (
	"testing"
	"time"
)

func TestAutoReplyRule_Fields(t *testing.T) {
	nowStr := "09:00"
	endStr := "18:00"
	rule := &AutoReplyRule{
		ID:           1,
		UserID:       100,
		Platform:     "douyin",
		Keywords:     "关键词 1，关键词 2",
		ReplyContent: "自动回复内容",
		Frequency:    60,
		DailyLimit:   100,
		StartTime:    &nowStr,
		EndTime:      &endStr,
		IsActive:     true,
	}

	if rule.ID != 1 {
		t.Errorf("Expected ID 1, got %d", rule.ID)
	}
	if rule.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", rule.UserID)
	}
	if rule.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", rule.Platform)
	}
	if rule.Keywords != "关键词 1，关键词 2" {
		t.Errorf("Expected Keywords, got %s", rule.Keywords)
	}
	if rule.ReplyContent != "自动回复内容" {
		t.Errorf("Expected ReplyContent, got %s", rule.ReplyContent)
	}
	if rule.Frequency != 60 {
		t.Errorf("Expected Frequency 60, got %d", rule.Frequency)
	}
	if rule.DailyLimit != 100 {
		t.Errorf("Expected DailyLimit 100, got %d", rule.DailyLimit)
	}
	if !rule.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestAutoReplyRule_DefaultValues(t *testing.T) {
	rule := &AutoReplyRule{}

	if rule.Frequency != 0 {
		t.Logf("Frequency is %d (expected 0 before save, default is 60)", rule.Frequency)
	}
	if rule.DailyLimit != 0 {
		t.Logf("DailyLimit is %d (expected 0 before save, default is 100)", rule.DailyLimit)
	}
	if rule.IsActive != false {
		t.Logf("IsActive is %v (expected false before save, default is true)", rule.IsActive)
	}
}

func TestAutoReplyRule_WithNilTime(t *testing.T) {
	rule := &AutoReplyRule{
		Platform:  "douyin",
		StartTime: nil,
		EndTime:   nil,
	}

	if rule.StartTime != nil {
		t.Errorf("Expected StartTime nil, got %v", rule.StartTime)
	}
	if rule.EndTime != nil {
		t.Errorf("Expected EndTime nil, got %v", rule.EndTime)
	}
}

func TestAutoReplyRule_WithPlatforms(t *testing.T) {
	platforms := []string{"douyin", "kuaishou", "xiaohongshu", "xianyu", "tiktok"}

	for _, platform := range platforms {
		rule := &AutoReplyRule{
			Platform: platform,
		}
		if rule.Platform != platform {
			t.Errorf("Expected Platform %s, got %s", platform, rule.Platform)
		}
	}
}

func TestAutoReplyRule_WithDisabled(t *testing.T) {
	rule := &AutoReplyRule{
		IsActive: false,
	}

	if rule.IsActive {
		t.Error("Expected IsActive to be false")
	}
}

func TestRateLimitTestResult(t *testing.T) {
	now := time.Now()
	result := &RateLimitTestResult{
		Platform:  "douyin",
		UserID:    100,
		AccountID: 200,
		TestID:    1,
		Allowed:   true,
		ErrorMsg:  "",
		Timestamp: now,
	}

	if result.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", result.Platform)
	}
	if result.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", result.UserID)
	}
	if result.AccountID != 200 {
		t.Errorf("Expected AccountID 200, got %d", result.AccountID)
	}
	if result.TestID != 1 {
		t.Errorf("Expected TestID 1, got %d", result.TestID)
	}
	if !result.Allowed {
		t.Error("Expected Allowed to be true")
	}
	if result.ErrorMsg != "" {
		t.Errorf("Expected empty ErrorMsg, got %s", result.ErrorMsg)
	}
}

func TestRateLimitTestResult_WithError(t *testing.T) {
	result := &RateLimitTestResult{
		Platform: "douyin",
		Allowed:  false,
		ErrorMsg: "Rate limit exceeded",
	}

	if result.Allowed {
		t.Error("Expected Allowed to be false")
	}
	if result.ErrorMsg != "Rate limit exceeded" {
		t.Errorf("Expected ErrorMsg 'Rate limit exceeded', got %s", result.ErrorMsg)
	}
}
