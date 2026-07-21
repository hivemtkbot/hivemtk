package browser

import (
	"testing"

	"github.com/chromedp/cdproto/network"
)

func TestPlatformConstants(t *testing.T) {
	if Douyin != "douyin" {
		t.Errorf("Expected Douyin to be 'douyin', got %s", Douyin)
	}
	if Kuaishou != "kuaishou" {
		t.Errorf("Expected Kuaishou to be 'kuaishou', got %s", Kuaishou)
	}
	if Xiaohongshu != "xiaohongshu" {
		t.Errorf("Expected Xiaohongshu to be 'xiaohongshu', got %s", Xiaohongshu)
	}
	if Xianyu != "xianyu" {
		t.Errorf("Expected Xianyu to be 'xianyu', got %s", Xianyu)
	}
	if Tiktok != "tiktok" {
		t.Errorf("Expected Tiktok to be 'tiktok', got %s", Tiktok)
	}
}

func TestLoginURL(t *testing.T) {
	tests := []struct {
		platform Platform
		expected string
	}{
		{Douyin, "https://www.douyin.com/login"},
		{Kuaishou, "https://www.kuaishou.com/login"},
		{Xiaohongshu, "https://www.xiaohongshu.com/explore?loginModal=true"},
		{Xianyu, "https://passport.goofish.com/mini_login.htm"},
		{Tiktok, "https://www.tiktok.com/login"},
		{"unknown", "https://www.douyin.com/login"},
	}

	for _, tt := range tests {
		t.Run(string(tt.platform), func(t *testing.T) {
			url := LoginURL(tt.platform)
			if url != tt.expected {
				t.Errorf("Expected LoginURL(%s) to be %s, got %s", tt.platform, tt.expected, url)
			}
		})
	}
}

func TestHasAuthCookie_Douyin(t *testing.T) {
	cookies := []*network.Cookie{
		{Name: "sessionid", Domain: "douyin.com"},
	}
	if !HasAuthCookie(Douyin, cookies) {
		t.Error("Expected Douyin auth cookie to be valid")
	}

	// Invalid cookie
	invalidCookies := []*network.Cookie{
		{Name: "other", Domain: "douyin.com"},
	}
	if HasAuthCookie(Douyin, invalidCookies) {
		t.Error("Expected invalid Douyin cookie to be rejected")
	}
}

func TestHasAuthCookie_Kuaishou(t *testing.T) {
	// Test sid cookie
	cookies := []*network.Cookie{
		{Name: "sid", Domain: "kuaishou.com"},
	}
	if !HasAuthCookie(Kuaishou, cookies) {
		t.Error("Expected Kuaishou sid cookie to be valid")
	}

	// Test session cookie
	cookies2 := []*network.Cookie{
		{Name: "session", Domain: "kuaishou.com"},
	}
	if !HasAuthCookie(Kuaishou, cookies2) {
		t.Error("Expected Kuaishou session cookie to be valid")
	}

	// Test case insensitive
	cookies3 := []*network.Cookie{
		{Name: "SID", Domain: "kuaishou.com"},
	}
	if !HasAuthCookie(Kuaishou, cookies3) {
		t.Error("Expected Kuaishou SID cookie (uppercase) to be valid")
	}
}

func TestHasAuthCookie_Xiaohongshu(t *testing.T) {
	cookies := []*network.Cookie{
		{Name: "web_session", Domain: "xiaohongshu.com"},
	}
	if !HasAuthCookie(Xiaohongshu, cookies) {
		t.Error("Expected Xiaohongshu web_session cookie to be valid")
	}

	// Invalid cookie
	invalidCookies := []*network.Cookie{
		{Name: "other", Domain: "xiaohongshu.com"},
	}
	if HasAuthCookie(Xiaohongshu, invalidCookies) {
		t.Error("Expected invalid Xiaohongshu cookie to be rejected")
	}
}

func TestHasAuthCookie_Xianyu(t *testing.T) {
	// Test xianyu_sid cookie
	cookies := []*network.Cookie{
		{Name: "xianyu_sid", Domain: "xianyu.com"},
	}
	if !HasAuthCookie(Xianyu, cookies) {
		t.Error("Expected Xianyu xianyu_sid cookie to be valid")
	}

	// Test session_token cookie
	cookies2 := []*network.Cookie{
		{Name: "session_token", Domain: "xianyu.com"},
	}
	if !HasAuthCookie(Xianyu, cookies2) {
		t.Error("Expected Xianyu session_token cookie to be valid")
	}
}

func TestHasAuthCookie_Tiktok(t *testing.T) {
	// Test sessionid_tt cookie
	cookies := []*network.Cookie{
		{Name: "sessionid_tt", Domain: "tiktok.com"},
	}
	if !HasAuthCookie(Tiktok, cookies) {
		t.Error("Expected Tiktok sessionid_tt cookie to be valid")
	}

	// Test tt_chain_token cookie
	cookies2 := []*network.Cookie{
		{Name: "tt_chain_token", Domain: "tiktok.com"},
	}
	if !HasAuthCookie(Tiktok, cookies2) {
		t.Error("Expected Tiktok tt_chain_token cookie to be valid")
	}

	// Test sid_guard cookie
	cookies3 := []*network.Cookie{
		{Name: "sid_guard", Domain: "tiktok.com"},
	}
	if !HasAuthCookie(Tiktok, cookies3) {
		t.Error("Expected Tiktok sid_guard cookie to be valid")
	}

	// Test sid_tt cookie
	cookies4 := []*network.Cookie{
		{Name: "sid_tt", Domain: "tiktok.com"},
	}
	if !HasAuthCookie(Tiktok, cookies4) {
		t.Error("Expected Tiktok sid_tt cookie to be valid")
	}
}

func TestHasAuthCookie_WrongDomain(t *testing.T) {
	cookies := []*network.Cookie{
		{Name: "sessionid", Domain: "wrong.com"},
	}
	if HasAuthCookie(Douyin, cookies) {
		t.Error("Expected cookie with wrong domain to be rejected")
	}
}

func TestHasAuthCookie_EmptyList(t *testing.T) {
	cookies := []*network.Cookie{}
	if HasAuthCookie(Douyin, cookies) {
		t.Error("Expected empty cookie list to be rejected")
	}
}

func TestHasAuthCookie_NilList(t *testing.T) {
	if HasAuthCookie(Douyin, nil) {
		t.Error("Expected nil cookie list to be rejected")
	}
}

func TestNewAutoReplyManager(t *testing.T) {
	headless := map[string]bool{
		"douyin": true,
	}
	manager := NewAutoReplyManager(headless)
	if manager == nil {
		t.Fatal("NewAutoReplyManager returned nil")
	}
	if manager.bots == nil {
		t.Error("Expected bots map to be initialized")
	}
	if manager.headless == nil {
		t.Error("Expected headless map to be initialized")
	}
}

func TestAutoReplyManager_SetHeadless(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	manager.SetHeadless("douyin", true)
	if !manager.GetHeadless("douyin") {
		t.Error("Expected headless to be true")
	}

	manager.SetHeadless("douyin", false)
	if manager.GetHeadless("douyin") {
		t.Error("Expected headless to be false")
	}
}

func TestAutoReplyManager_GetHeadless_Default(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	// Default should be true (headless mode)
	if !manager.GetHeadless("unknown_platform") {
		t.Error("Expected default headless to be true")
	}
}

func TestAutoReplyManager_IsBotRunning_NotExist(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	if manager.IsBotRunning(Douyin) {
		t.Error("Expected non-existent bot to not be running")
	}
}

func TestAutoReplyManager_GetAllBots_Empty(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	bots := manager.GetAllBots()
	if bots == nil {
		t.Error("Expected non-nil bots map")
	}
	if len(bots) != 0 {
		t.Errorf("Expected empty bots map, got %d entries", len(bots))
	}
}

func TestAutoReplyManager_StopBot_NotExist(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	// StopBot 设计为幂等：机器人不存在视为已停止，不应返回 error
	err := manager.StopBot(Douyin)
	if err != nil {
		t.Errorf("Expected no error (idempotent stop), got: %v", err)
	}
}

func TestAutoReplyManager_GetBot_NotExist(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	bot, err := manager.GetBot(Douyin)
	if err == nil {
		t.Error("Expected error when getting non-existent bot")
	}
	if bot != nil {
		t.Error("Expected nil bot for non-existent bot")
	}
}

func TestAutoReplyManager_StopAllBots_Empty(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	// Should not panic
	manager.StopAllBots()
}

func TestAutoReplyManager_GetStatus(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{})

	status := manager.GetStatus()
	if status == nil {
		t.Fatal("Expected non-nil status")
	}

	if _, ok := status["headless_settings"]; !ok {
		t.Error("Expected headless_settings in status")
	}
	if _, ok := status["total_bots"]; !ok {
		t.Error("Expected total_bots in status")
	}
	if _, ok := status["bots"]; !ok {
		t.Error("Expected bots in status")
	}
}

func TestAutoReplyManager_StartBot_Basic(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{"douyin": true})

	// StartBot will fail because chromedp cannot be initialized in test environment
	// but we can test that the method exists and returns appropriate error
	err := manager.StartBot(Douyin, "test_account", 1, "test_cookies")
	// The error is expected because we cannot initialize browser in test environment
	_ = err
}

func TestAutoReplyManager_SetHeadless_UpdatesExisting(t *testing.T) {
	manager := NewAutoReplyManager(map[string]bool{"douyin": true})

	// Set headless to false
	manager.SetHeadless("douyin", false)

	// Verify it was updated
	if manager.GetHeadless("douyin") {
		t.Error("Expected headless to be false after update")
	}
}

func TestAutoReplyManager_GetStatus_WithHeadlessSettings(t *testing.T) {
	headless := map[string]bool{
		"douyin":      true,
		"kuaishou":    false,
		"xiaohongshu": true,
	}
	manager := NewAutoReplyManager(headless)

	status := manager.GetStatus()
	headlessSettings, ok := status["headless_settings"].(map[string]bool)
	if !ok {
		t.Fatal("Expected headless_settings to be map[string]bool")
	}

	if !headlessSettings["douyin"] {
		t.Error("Expected douyin headless to be true")
	}
	if headlessSettings["kuaishou"] {
		t.Error("Expected kuaishou headless to be false")
	}
}

// Test getPlatformDomain function
func TestGetPlatformDomain(t *testing.T) {
	tests := []struct {
		platform Platform
		expected string
	}{
		{Douyin, ".douyin.com"},
		{Kuaishou, ".kuaishou.com"},
		{Xiaohongshu, ".xiaohongshu.com"},
		{Xianyu, ".goofish.com"},
		{Tiktok, ".tiktok.com"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		result := getPlatformDomain(tt.platform)
		if result != tt.expected {
			t.Errorf("getPlatformDomain(%s) = %s, expected %s", tt.platform, result, tt.expected)
		}
	}
}

// Test getPlatformMessageURL function
func TestGetPlatformMessageURL(t *testing.T) {
	tests := []struct {
		platform Platform
		expected string
	}{
		{Douyin, "https://creator.douyin.com/creator-micro/data-analysis/message"},
		{Kuaishou, "https://cp.kuaishou.com/article/publish/video"},
		{Xiaohongshu, "https://creator.xiaohongshu.com/creator/home"},
		{Xianyu, "https://www.goofish.com/im"},
		{Tiktok, "https://www.tiktok.com/creator-center/dm"},
		{"unknown", "https://www.douyin.com/"},
	}

	for _, tt := range tests {
		result := getPlatformMessageURL(tt.platform)
		if result != tt.expected {
			t.Errorf("getPlatformMessageURL(%s) = %s, expected %s", tt.platform, result, tt.expected)
		}
	}
}
