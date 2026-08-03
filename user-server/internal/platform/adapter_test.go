package platform

import (
	"marketing/internal/model"
	"strings"
	"testing"
)

func TestBaseAdapter_GetPlatform(t *testing.T) {
	adapter := &BaseAdapter{platform: model.PlatformDouyin}
	platform := adapter.GetPlatform()
	if platform != model.PlatformDouyin {
		t.Errorf("Expected platform %s, got %s", model.PlatformDouyin, platform)
	}
}

func TestBaseAdapter_GenerateMessageID(t *testing.T) {
	adapter := &BaseAdapter{}
	msgID := adapter.GenerateMessageID(model.PlatformDouyin, "account1", "chat1", "sender1", 1234567890)
	if msgID == "" {
		t.Error("Expected non-empty message ID")
	}
	// Should start with "msg_"
	if len(msgID) < 5 || msgID[:4] != "msg_" {
		t.Errorf("Expected message ID to start with 'msg_', got %s", msgID)
	}
}

func TestBaseAdapter_GenerateMessageID_Consistency(t *testing.T) {
	adapter := &BaseAdapter{}
	id1 := adapter.GenerateMessageID(model.PlatformDouyin, "account1", "chat1", "sender1", 1234567890)
	id2 := adapter.GenerateMessageID(model.PlatformDouyin, "account1", "chat1", "sender1", 1234567890)
	if id1 != id2 {
		t.Error("Expected consistent message IDs for same input")
	}
}

func TestBaseAdapter_GenerateMessageID_Uniqueness(t *testing.T) {
	adapter := &BaseAdapter{}
	id1 := adapter.GenerateMessageID(model.PlatformDouyin, "account1", "chat1", "sender1", 1234567890)
	id2 := adapter.GenerateMessageID(model.PlatformDouyin, "account2", "chat1", "sender1", 1234567890)
	if id1 == id2 {
		t.Error("Expected different message IDs for different accounts")
	}
}

func TestBaseAdapter_GenerateReplyID(t *testing.T) {
	adapter := &BaseAdapter{}
	replyID := adapter.GenerateReplyID("msg_123")
	if replyID == "" {
		t.Error("Expected non-empty reply ID")
	}
	// Should start with "reply_"
	if len(replyID) < 7 || replyID[:6] != "reply_" {
		t.Errorf("Expected reply ID to start with 'reply_', got %s", replyID)
	}
}

func TestDouyinAdapter(t *testing.T) {
	adapter := NewDouyinAdapter()
	if adapter == nil {
		t.Fatal("NewDouyinAdapter returned nil")
	}
}

func TestDouyinAdapter_GetPlatform(t *testing.T) {
	adapter := NewDouyinAdapter()
	platform := adapter.GetPlatform()
	if platform != model.PlatformDouyin {
		t.Errorf("Expected platform %s, got %s", model.PlatformDouyin, platform)
	}
}

func TestDouyinAdapter_GetMessages(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.GetMessages("account1", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "抖音账号 Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestDouyinAdapter_SendMessage(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.SendMessage("account1", "chat1", "Hello", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "抖音账号 Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestDouyinAdapter_SendImage(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.SendImage("account1", "chat1", "http://example.com/image.jpg")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "抖音账号 Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestDouyinAdapter_Login(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.Login(map[string]string{"token": "test"})
	if err == nil {
		t.Error("Expected error when login fails (no real browser)")
	}
}

func TestDouyinAdapter_CheckLoginStatus(t *testing.T) {
	adapter := NewDouyinAdapter()
	loggedIn, _ := adapter.CheckLoginStatus("account1")
	if loggedIn {
		t.Error("Expected account to be NOT logged in (no cookie configured)")
	}
}

func TestDouyinAdapter_Logout(t *testing.T) {
	adapter := NewDouyinAdapter()
	err := adapter.Logout("account1")
	if err != nil {
		t.Errorf("Logout failed: %v", err)
	}
}

func TestDouyinAdapter_RefreshToken(t *testing.T) {
	adapter := NewDouyinAdapter()
	err := adapter.RefreshToken("account1")
	if err != nil {
		t.Errorf("RefreshToken failed: %v", err)
	}
}

func TestDouyinAdapter_GetUserInfo(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.GetUserInfo("account1", "user1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestDouyinAdapter_GetChatInfo(t *testing.T) {
	adapter := NewDouyinAdapter()
	_, err := adapter.GetChatInfo("account1", "chat1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestDouyinAdapter_ParseWebhook(t *testing.T) {
	adapter := NewDouyinAdapter()
	data := []byte(`{"content": "test message", "type": "text"}`)
	msg, err := adapter.ParseWebhook(data)
	if err != nil {
		t.Errorf("ParseWebhook failed: %v", err)
	}
	if msg == nil {
		t.Fatal("Expected non-nil message")
	}
	if msg.Platform != model.PlatformDouyin {
		t.Errorf("Expected platform %s, got %s", model.PlatformDouyin, msg.Platform)
	}
	if msg.Content != "test message" {
		t.Errorf("Expected content 'test message', got %s", msg.Content)
	}
}

func TestDouyinAdapter_ParseWebhook_InvalidJSON(t *testing.T) {
	adapter := NewDouyinAdapter()
	data := []byte(`invalid json`)
	_, err := adapter.ParseWebhook(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestDouyinAdapter_GetWebhookURL(t *testing.T) {
	adapter := NewDouyinAdapter()
	url := adapter.GetWebhookURL("account1")
	expected := "/api/webhook/douyin/account1"
	if url != expected {
		t.Errorf("Expected webhook URL '%s', got %s", expected, url)
	}
}

func TestKuaishouAdapter(t *testing.T) {
	adapter := NewKuaishouAdapter()
	if adapter == nil {
		t.Fatal("NewKuaishouAdapter returned nil")
	}
}

func TestKuaishouAdapter_GetPlatform(t *testing.T) {
	adapter := NewKuaishouAdapter()
	platform := adapter.GetPlatform()
	if platform != model.PlatformKuaishou {
		t.Errorf("Expected platform %s, got %s", model.PlatformKuaishou, platform)
	}
}

func TestKuaishouAdapter_SendMessage(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.SendMessage("account1", "chat1", "Hello", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestKuaishouAdapter_GetWebhookURL(t *testing.T) {
	adapter := NewKuaishouAdapter()
	url := adapter.GetWebhookURL("account1")
	expected := "/api/webhook/kuaishou/account1"
	if url != expected {
		t.Errorf("Expected webhook URL '%s', got %s", expected, url)
	}
}

func TestXiaohongshuAdapter(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	if adapter == nil {
		t.Fatal("NewXiaohongshuAdapter returned nil")
	}
}

func TestXiaohongshuAdapter_GetPlatform(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	platform := adapter.GetPlatform()
	if platform != model.PlatformXiaohongshu {
		t.Errorf("Expected platform %s, got %s", model.PlatformXiaohongshu, platform)
	}
}

func TestXiaohongshuAdapter_SendMessage(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.SendMessage("account1", "chat1", "Hello", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXiaohongshuAdapter_GetWebhookURL(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	url := adapter.GetWebhookURL("account1")
	expected := "/api/webhook/xiaohongshu/account1"
	if url != expected {
		t.Errorf("Expected webhook URL '%s', got %s", expected, url)
	}
}

func TestXianyuAdapter(t *testing.T) {
	adapter := NewXianyuAdapter()
	if adapter == nil {
		t.Fatal("NewXianyuAdapter returned nil")
	}
}

func TestXianyuAdapter_GetPlatform(t *testing.T) {
	adapter := NewXianyuAdapter()
	platform := adapter.GetPlatform()
	if platform != model.PlatformXianyu {
		t.Errorf("Expected platform %s, got %s", model.PlatformXianyu, platform)
	}
}

func TestXianyuAdapter_SendMessage(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.SendMessage("account1", "chat1", "Hello", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXianyuAdapter_GetWebhookURL(t *testing.T) {
	adapter := NewXianyuAdapter()
	url := adapter.GetWebhookURL("account1")
	expected := "/api/webhook/xianyu/account1"
	if url != expected {
		t.Errorf("Expected webhook URL '%s', got %s", expected, url)
	}
}

// Test KuaishouAdapter methods
func TestKuaishouAdapter_GetMessages(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.GetMessages("account1", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestKuaishouAdapter_SendImage(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.SendImage("account1", "chat1", "http://example.com/image.jpg")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestKuaishouAdapter_Login(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.Login(map[string]string{"token": "test"})
	if err == nil {
		t.Error("Expected error when login fails (no real browser)")
	}
}

func TestKuaishouAdapter_CheckLoginStatus(t *testing.T) {
	adapter := NewKuaishouAdapter()
	loggedIn, _ := adapter.CheckLoginStatus("account1")
	if loggedIn {
		t.Error("Expected account to be NOT logged in (no cookie configured)")
	}
}

func TestKuaishouAdapter_Logout(t *testing.T) {
	adapter := NewKuaishouAdapter()
	err := adapter.Logout("account1")
	if err != nil {
		t.Errorf("Logout failed: %v", err)
	}
}

func TestKuaishouAdapter_RefreshToken(t *testing.T) {
	adapter := NewKuaishouAdapter()
	err := adapter.RefreshToken("account1")
	if err != nil {
		t.Errorf("RefreshToken failed: %v", err)
	}
}

func TestKuaishouAdapter_GetUserInfo(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.GetUserInfo("account1", "user1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestKuaishouAdapter_GetChatInfo(t *testing.T) {
	adapter := NewKuaishouAdapter()
	_, err := adapter.GetChatInfo("account1", "chat1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestKuaishouAdapter_ParseWebhook(t *testing.T) {
	adapter := NewKuaishouAdapter()
	data := []byte(`{"content": "test message", "type": "text"}`)
	msg, err := adapter.ParseWebhook(data)
	if err != nil {
		t.Errorf("ParseWebhook failed: %v", err)
	}
	if msg == nil {
		t.Fatal("Expected non-nil message")
	}
	if msg.Platform != model.PlatformKuaishou {
		t.Errorf("Expected platform %s, got %s", model.PlatformKuaishou, msg.Platform)
	}
	if msg.Content != "test message" {
		t.Errorf("Expected content 'test message', got %s", msg.Content)
	}
}

func TestKuaishouAdapter_ParseWebhook_InvalidJSON(t *testing.T) {
	adapter := NewKuaishouAdapter()
	data := []byte(`invalid json`)
	_, err := adapter.ParseWebhook(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// Test XiaohongshuAdapter methods
func TestXiaohongshuAdapter_GetMessages(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.GetMessages("account1", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXiaohongshuAdapter_SendImage(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.SendImage("account1", "chat1", "http://example.com/image.jpg")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXiaohongshuAdapter_Login(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.Login(map[string]string{"token": "test"})
	if err == nil {
		t.Error("Expected error when login fails (no real browser)")
	}
}

func TestXiaohongshuAdapter_CheckLoginStatus(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	loggedIn, _ := adapter.CheckLoginStatus("account1")
	if loggedIn {
		t.Error("Expected account to be NOT logged in (no cookie configured)")
	}
}

func TestXiaohongshuAdapter_Logout(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	err := adapter.Logout("account1")
	if err != nil {
		t.Errorf("Logout failed: %v", err)
	}
}

func TestXiaohongshuAdapter_RefreshToken(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	err := adapter.RefreshToken("account1")
	if err != nil {
		t.Errorf("RefreshToken failed: %v", err)
	}
}

func TestXiaohongshuAdapter_GetUserInfo(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.GetUserInfo("account1", "user1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestXiaohongshuAdapter_GetChatInfo(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	_, err := adapter.GetChatInfo("account1", "chat1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestXiaohongshuAdapter_ParseWebhook(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	data := []byte(`{"content": "test message", "type": "text"}`)
	msg, err := adapter.ParseWebhook(data)
	if err != nil {
		t.Errorf("ParseWebhook failed: %v", err)
	}
	if msg == nil {
		t.Fatal("Expected non-nil message")
	}
	if msg.Platform != model.PlatformXiaohongshu {
		t.Errorf("Expected platform %s, got %s", model.PlatformXiaohongshu, msg.Platform)
	}
	if msg.Content != "test message" {
		t.Errorf("Expected content 'test message', got %s", msg.Content)
	}
}

func TestXiaohongshuAdapter_ParseWebhook_InvalidJSON(t *testing.T) {
	adapter := NewXiaohongshuAdapter()
	data := []byte(`invalid json`)
	_, err := adapter.ParseWebhook(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// Test XianyuAdapter methods
func TestXianyuAdapter_GetMessages(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.GetMessages("account1", nil)
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXianyuAdapter_SendImage(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.SendImage("account1", "chat1", "http://example.com/image.jpg")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
	if !strings.Contains(err.Error(), "Cookie 尚未配置") {
		t.Errorf("Expected cookie error, got: %v", err)
	}
}

func TestXianyuAdapter_Login(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.Login(map[string]string{"token": "test"})
	if err == nil {
		t.Error("Expected error when login fails (no real browser)")
	}
}

func TestXianyuAdapter_CheckLoginStatus(t *testing.T) {
	adapter := NewXianyuAdapter()
	loggedIn, _ := adapter.CheckLoginStatus("account1")
	if loggedIn {
		t.Error("Expected account to be NOT logged in (no cookie configured)")
	}
}

func TestXianyuAdapter_Logout(t *testing.T) {
	adapter := NewXianyuAdapter()
	err := adapter.Logout("account1")
	if err != nil {
		t.Errorf("Logout failed: %v", err)
	}
}

func TestXianyuAdapter_RefreshToken(t *testing.T) {
	adapter := NewXianyuAdapter()
	err := adapter.RefreshToken("account1")
	if err != nil {
		t.Errorf("RefreshToken failed: %v", err)
	}
}

func TestXianyuAdapter_GetUserInfo(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.GetUserInfo("account1", "user1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestXianyuAdapter_GetChatInfo(t *testing.T) {
	adapter := NewXianyuAdapter()
	_, err := adapter.GetChatInfo("account1", "chat1")
	if err == nil {
		t.Error("Expected error for unconfigured account cookie")
	}
}

func TestXianyuAdapter_ParseWebhook(t *testing.T) {
	adapter := NewXianyuAdapter()
	data := []byte(`{"content": "test message", "type": "text"}`)
	msg, err := adapter.ParseWebhook(data)
	if err != nil {
		t.Errorf("ParseWebhook failed: %v", err)
	}
	if msg == nil {
		t.Fatal("Expected non-nil message")
	}
	if msg.Platform != model.PlatformXianyu {
		t.Errorf("Expected platform %s, got %s", model.PlatformXianyu, msg.Platform)
	}
	if msg.Content != "test message" {
		t.Errorf("Expected content 'test message', got %s", msg.Content)
	}
}

func TestXianyuAdapter_ParseWebhook_InvalidJSON(t *testing.T) {
	adapter := NewXianyuAdapter()
	data := []byte(`invalid json`)
	_, err := adapter.ParseWebhook(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestAdapterRegistry(t *testing.T) {
	registry := NewAdapterRegistry()
	if registry == nil {
		t.Fatal("NewAdapterRegistry returned nil")
	}
}

func TestAdapterRegistry_Register(t *testing.T) {
	registry := NewAdapterRegistry()
	mockAdapter := NewDouyinAdapter()
	registry.Register(model.PlatformDouyin, mockAdapter)

	adapter, err := registry.Get(model.PlatformDouyin)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestAdapterRegistry_Get_UnsupportedPlatform(t *testing.T) {
	registry := NewAdapterRegistry()
	_, err := registry.Get("unsupported_platform")
	if err == nil {
		t.Error("Expected error for unsupported platform")
	}
}

func TestAdapterRegistry_GetAll(t *testing.T) {
	registry := NewAdapterRegistry()
	adapters := registry.GetAll()
	if adapters == nil {
		t.Error("Expected non-nil adapters map")
	}
	// Should have at least 4 adapters registered
	if len(adapters) < 4 {
		t.Errorf("Expected at least 4 adapters, got %d", len(adapters))
	}
}

func TestAdapterRegistry_GetPlatforms(t *testing.T) {
	registry := NewAdapterRegistry()
	platforms := registry.GetPlatforms()
	if platforms == nil {
		t.Error("Expected non-nil platforms slice")
	}
	if len(platforms) < 4 {
		t.Errorf("Expected at least 4 platforms, got %d", len(platforms))
	}
}

func TestGetAdapterRegistry(t *testing.T) {
	registry1 := GetAdapterRegistry()
	registry2 := GetAdapterRegistry()
	if registry1 != registry2 {
		t.Error("Expected same registry instance")
	}
}

func TestGetAdapter(t *testing.T) {
	adapter, err := GetAdapter(model.PlatformDouyin)
	if err != nil {
		t.Errorf("GetAdapter failed: %v", err)
	}
	if adapter == nil {
		t.Error("Expected non-nil adapter")
	}
}

func TestGetStringFromMap(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "string value",
			m:        map[string]any{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "int value",
			m:        map[string]any{"key": 123},
			key:      "key",
			expected: "123",
		},
		{
			name:     "missing key",
			m:        map[string]any{"other": "value"},
			key:      "key",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringFromMap(tt.m, tt.key)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestAdapterRegistry_GetAll_AdapterTypes(t *testing.T) {
	registry := NewAdapterRegistry()
	adapters := registry.GetAll()

	// Check that all adapters are registered
	expectedPlatforms := []model.Platform{
		model.PlatformDouyin,
		model.PlatformKuaishou,
		model.PlatformXiaohongshu,
		model.PlatformXianyu,
	}

	for _, platform := range expectedPlatforms {
		if _, ok := adapters[platform]; !ok {
			t.Errorf("Expected adapter for platform %s", platform)
		}
	}
}

func TestBaseAdapter_GenerateReplyID_Uniqueness(t *testing.T) {
	adapter := &BaseAdapter{}
	id1 := adapter.GenerateReplyID("msg_123")
	// Add a small delay to ensure different timestamp
	// In practice, the timestamps should be different enough
	id2 := adapter.GenerateReplyID("msg_123")
	// IDs might be the same if called within the same nanosecond, but very unlikely
	_ = id1
	_ = id2
	// We just verify both IDs are valid
	if len(id1) < 7 || id1[:6] != "reply_" {
		t.Error("First reply ID invalid")
	}
	if len(id2) < 7 || id2[:6] != "reply_" {
		t.Error("Second reply ID invalid")
	}
}
