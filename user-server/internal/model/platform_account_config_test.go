package model

import (
	"testing"
)

func TestReplyRule_Value(t *testing.T) {
	rule := ReplyRule{
		ID:            "rule-123",
		Priority:      10,
		Keywords:      []string{"关键词 1", "关键词 2"},
		ReplyTemplate: "自动回复：{{keyword}}您好！",
		IsActive:      true,
	}

	value, err := rule.Value()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if value == nil {
		t.Fatal("Expected non-nil value")
	}
}

func TestReplyRule_Scan(t *testing.T) {
	rule := &ReplyRule{}
	jsonData := []byte(`{"id":"rule-456","priority":5,"keywords":["test"],"reply_template":"Hello","is_active":true}`)

	err := rule.Scan(jsonData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rule.ID != "rule-456" {
		t.Errorf("Expected ID 'rule-456', got %s", rule.ID)
	}
	if rule.Priority != 5 {
		t.Errorf("Expected Priority 5, got %d", rule.Priority)
	}
	if len(rule.Keywords) != 1 || rule.Keywords[0] != "test" {
		t.Errorf("Expected Keywords ['test'], got %v", rule.Keywords)
	}
	if rule.ReplyTemplate != "Hello" {
		t.Errorf("Expected ReplyTemplate 'Hello', got %s", rule.ReplyTemplate)
	}
	if !rule.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestReplyRule_ScanNil(t *testing.T) {
	rule := &ReplyRule{
		ID: "existing-id",
	}

	err := rule.Scan(nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if rule.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", rule.ID)
	}
}

func TestPlatformAccountConfig_TableName(t *testing.T) {
	config := &PlatformAccountConfig{}
	tableName := config.TableName()
	if tableName != "platform_account_configs" {
		t.Errorf("Expected table name 'platform_account_configs', got %s", tableName)
	}
}

func TestPlatformAccountConfig_BasicFields(t *testing.T) {
	config := &PlatformAccountConfig{
		ID:                 "config-123",
		AccountID:          "account-456",
		Platform:           "douyin",
		RagProductID:       strPtr("rag-789"),
		IsAutoReplyEnabled: true,
		IsRagEnabled:       false,
		MaxDailyQueries:    1000,
	}

	if config.ID != "config-123" {
		t.Errorf("Expected ID 'config-123', got %s", config.ID)
	}
	if config.AccountID != "account-456" {
		t.Errorf("Expected AccountID 'account-456', got %s", config.AccountID)
	}
	if config.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", config.Platform)
	}
	if config.RagProductID == nil || *config.RagProductID != "rag-789" {
		t.Errorf("Expected RagProductID 'rag-789', got %v", config.RagProductID)
	}
	if !config.IsAutoReplyEnabled {
		t.Error("Expected IsAutoReplyEnabled to be true")
	}
	if config.IsRagEnabled {
		t.Error("Expected IsRagEnabled to be false")
	}
	if config.MaxDailyQueries != 1000 {
		t.Errorf("Expected MaxDailyQueries 1000, got %d", config.MaxDailyQueries)
	}
}

func TestPlatformAccountConfig_DefaultValues(t *testing.T) {
	config := &PlatformAccountConfig{}

	if config.IsAutoReplyEnabled != false {
		t.Logf("IsAutoReplyEnabled is %v (expected false before save, default is false)", config.IsAutoReplyEnabled)
	}
	if config.IsRagEnabled != false {
		t.Logf("IsRagEnabled is %v (expected false before save, default is false)", config.IsRagEnabled)
	}
	if config.MaxDailyQueries != 0 {
		t.Logf("MaxDailyQueries is %d (expected 0 before save, default is 1000)", config.MaxDailyQueries)
	}
}

func TestPlatformAccountConfig_WithPlatforms(t *testing.T) {
	platforms := []string{"douyin", "kuaishou", "xiaohongshu", "xianyu", "tiktok"}

	for _, platform := range platforms {
		config := &PlatformAccountConfig{
			Platform: platform,
		}
		if config.Platform != platform {
			t.Errorf("Expected Platform %s, got %s", platform, config.Platform)
		}
	}
}

func TestPlatformAccountConfig_WithNilRagProductID(t *testing.T) {
	config := &PlatformAccountConfig{
		Platform:     "douyin",
		RagProductID: nil,
	}

	if config.RagProductID != nil {
		t.Errorf("Expected RagProductID nil, got %v", config.RagProductID)
	}
}

func TestPlatformAccountConfig_WithReplyRules(t *testing.T) {
	config := &PlatformAccountConfig{
		ReplyRules: []ReplyRule{
			{ID: "rule-1", Priority: 10, Keywords: []string{"你好"}, ReplyTemplate: "您好！", IsActive: true},
			{ID: "rule-2", Priority: 5, Keywords: []string{"价格"}, ReplyTemplate: "请咨询客服", IsActive: true},
		},
	}

	if len(config.ReplyRules) != 2 {
		t.Errorf("Expected 2 reply rules, got %d", len(config.ReplyRules))
	}
}

func strPtr(s string) *string {
	return &s
}
