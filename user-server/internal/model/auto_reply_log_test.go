package model

import (
	"testing"
)

func TestAutoReplyLog_Fields(t *testing.T) {
	log := &AutoReplyLog{
		ID:            1,
		UserID:        100,
		AccountID:     200,
		RuleID:        300,
		Platform:      "douyin",
		TargetContent: "用户评论：这个产品怎么样？",
		ReplyContent:  "自动回复：您好，这款产品非常好用！",
		Status:        "success",
		ErrorMsg:      "",
	}

	if log.ID != 1 {
		t.Errorf("Expected ID 1, got %d", log.ID)
	}
	if log.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", log.UserID)
	}
	if log.AccountID != 200 {
		t.Errorf("Expected AccountID 200, got %d", log.AccountID)
	}
	if log.RuleID != 300 {
		t.Errorf("Expected RuleID 300, got %d", log.RuleID)
	}
	if log.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", log.Platform)
	}
	if log.TargetContent != "用户评论：这个产品怎么样？" {
		t.Errorf("Expected TargetContent, got %s", log.TargetContent)
	}
	if log.ReplyContent != "自动回复：您好，这款产品非常好用！" {
		t.Errorf("Expected ReplyContent, got %s", log.ReplyContent)
	}
	if log.Status != "success" {
		t.Errorf("Expected Status 'success', got %s", log.Status)
	}
}

func TestAutoReplyLog_WithEmptyFields(t *testing.T) {
	log := &AutoReplyLog{}

	if log.ID != 0 {
		t.Logf("ID is %d (expected 0 before save)", log.ID)
	}
	if log.UserID != 0 {
		t.Logf("UserID is %d (expected 0 before save)", log.UserID)
	}
	if log.Platform != "" {
		t.Errorf("Expected empty Platform, got %s", log.Platform)
	}
}

func TestAutoReplyLog_WithStatusValues(t *testing.T) {
	statuses := []string{"success", "failed", "pending"}

	for _, status := range statuses {
		log := &AutoReplyLog{
			Status: status,
		}
		if log.Status != status {
			t.Errorf("Expected Status %s, got %s", status, log.Status)
		}
	}
}

func TestAutoReplyLog_WithErrorMsg(t *testing.T) {
	log := &AutoReplyLog{
		Status:   "failed",
		ErrorMsg: "API rate limit exceeded",
	}

	if log.Status != "failed" {
		t.Errorf("Expected Status 'failed', got %s", log.Status)
	}
	if log.ErrorMsg != "API rate limit exceeded" {
		t.Errorf("Expected ErrorMsg 'API rate limit exceeded', got %s", log.ErrorMsg)
	}
}

func TestAutoReplyLog_WithPlatforms(t *testing.T) {
	platforms := []string{"douyin", "kuaishou", "xiaohongshu", "xianyu", "tiktok"}

	for _, platform := range platforms {
		log := &AutoReplyLog{
			Platform: platform,
		}
		if log.Platform != platform {
			t.Errorf("Expected Platform %s, got %s", platform, log.Platform)
		}
	}
}
