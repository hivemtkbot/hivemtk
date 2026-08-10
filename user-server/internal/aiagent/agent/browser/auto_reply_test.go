package browser

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

var ErrRuleMatch = errors.New("rule match error")

// MockRuleMatcher 用于测试的模拟规则匹配器
type MockRuleMatcher struct {
	matchingRule *model.AutoReplyRule
	testError    error
	logs         []logEntry
}

type logEntry struct {
	userID, accountID, ruleID               uint
	platform, target, reply, status, errMsg string
}

func (m *MockRuleMatcher) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	return m.matchingRule, m.testError
}

func (m *MockRuleMatcher) AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	m.logs = append(m.logs, logEntry{
		userID: userID, accountID: accountID, ruleID: ruleID,
		platform: platform, target: target, reply: reply, status: status, errMsg: errMsg,
	})
	return nil
}

// Test AutoReplyBot methods that don't require browser initialization
func TestAutoReplyBot_IsRunning(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
		cookies:   "test_cookies",
		isRunning: false,
		headless:  true,
	}

	if bot.IsRunning() {
		t.Error("Expected bot to not be running initially")
	}

	bot.isRunning = true
	if !bot.IsRunning() {
		t.Error("Expected bot to be running after setting isRunning to true")
	}
}

func TestAutoReplyBot_GetPlatform(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Xiaohongshu,
		account:   "test_account",
		accountID: 1,
	}

	if bot.GetPlatform() != Xiaohongshu {
		t.Errorf("Expected platform Xiaohongshu, got %s", bot.GetPlatform())
	}
}

func TestAutoReplyBot_GetAccount(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Xianyu,
		account:   "my_account",
		accountID: 1,
	}

	if bot.GetAccount() != "my_account" {
		t.Errorf("Expected account 'my_account', got %s", bot.GetAccount())
	}
}

func TestAutoReplyBot_IsHeadless(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Tiktok,
		account:   "test_account",
		accountID: 1,
		headless:  true,
	}

	if !bot.IsHeadless() {
		t.Error("Expected headless to be true")
	}

	bot.headless = false
	if bot.IsHeadless() {
		t.Error("Expected headless to be false after setting to false")
	}
}

func TestAutoReplyBot_SetHeadless(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
		headless:  true,
	}

	bot.SetHeadless(false)
	if bot.IsHeadless() {
		t.Error("Expected headless to be false after SetHeadless(false)")
	}

	bot.SetHeadless(true)
	if !bot.IsHeadless() {
		t.Error("Expected headless to be true after SetHeadless(true)")
	}
}

func TestAutoReplyBot_GetAssistant(t *testing.T) {
	mockAssistant := &Assistant{}
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
		assistant: mockAssistant,
	}

	assistant := bot.GetAssistant()
	if assistant != mockAssistant {
		t.Error("Expected to return the same assistant instance")
	}
}

func TestAutoReplyBot_Stop_WhenNotRunning(t *testing.T) {
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
		isRunning: false,
	}

	err := bot.Stop()
	if err == nil {
		t.Error("Expected error when stopping bot that is not running")
	}
}

func TestAutoReplyBot_MarkAsRead(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	err := bot.MarkAsRead("message_123")
	if err != nil {
		t.Errorf("MarkAsRead should not return error, got %v", err)
	}
}

func TestAutoReplyBot_SendReply_UnsupportedPlatform(t *testing.T) {
	bot := &AutoReplyBot{
		platform: "unsupported_platform",
	}

	err := bot.SendReply("msg_123", "reply content")
	if err == nil {
		t.Error("Expected error for unsupported platform")
	}
}

func TestAutoReplyBot_GetUnreadMessages_UnsupportedPlatform(t *testing.T) {
	bot := &AutoReplyBot{
		platform: "unsupported_platform",
	}

	_, err := bot.getUnreadMessages()
	if err == nil {
		t.Error("Expected error for unsupported platform")
	}
}

func TestAutoReplyBot_ProcessMessage_NoRule(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	mockMatcher := &MockRuleMatcher{
		matchingRule: nil, // No matching rule
	}

	msg := Message{
		ID:         "msg_123",
		SenderID:   "sender_1",
		SenderName: "Test User",
		Content:    "Hello",
		Timestamp:  time.Now(),
		IsRead:     false,
		Platform:   "douyin",
	}

	// ProcessMessage should not return error when no rule matches
	err := bot.processMessage(msg, mockMatcher, 1)
	if err != nil {
		t.Errorf("processMessage with no rule should not return error, got %v", err)
	}
}

func TestAutoReplyBot_ProcessMessage_WithRule(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	mockMatcher := &MockRuleMatcher{
		matchingRule: &model.AutoReplyRule{
			ID:           1,
			Platform:     "douyin",
			Keywords:     "hello",
			ReplyContent: "Hello! How can I help you?",
		},
	}

	msg := Message{
		ID:         "msg_123",
		SenderID:   "sender_1",
		SenderName: "Test User",
		Content:    "hello",
		Timestamp:  time.Now(),
		IsRead:     false,
		Platform:   "douyin",
	}

	// ProcessMessage will try to send reply which requires browser
	// but we can verify it attempts to process the message
	err := bot.processMessage(msg, mockMatcher, 1)
	// Error is expected in test environment when trying to send reply
	if err == nil {
		t.Log("Expected error when sending reply without browser")
	}
}

func TestAutoReplyBot_ProcessMessage_MatcherError(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	mockMatcher := &MockRuleMatcher{
		matchingRule: nil,
		testError:    ErrRuleMatch,
	}

	msg := Message{
		ID:         "msg_123",
		SenderID:   "sender_1",
		SenderName: "Test User",
		Content:    "hello",
		Timestamp:  time.Now(),
		IsRead:     false,
		Platform:   "douyin",
	}

	err := bot.processMessage(msg, mockMatcher, 1)
	if err == nil {
		t.Error("Expected error when matcher returns error")
	}
}

func TestAutoReplyBot_CheckAndReplyMessages(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	mockMatcher := &MockRuleMatcher{}

	// checkAndReplyMessages will fail in test environment but we can test it exists
	err := bot.checkAndReplyMessages(mockMatcher, 1)
	// Error is expected without browser
	_ = err
}

func TestMessage_Struct(t *testing.T) {
	msg := Message{
		ID:         "123",
		SenderID:   "sender_1",
		SenderName: "Test User",
		Content:    "Hello World",
		Timestamp:  time.Now(),
		IsRead:     false,
		Platform:   "douyin",
	}

	if msg.ID != "123" {
		t.Errorf("Expected ID '123', got %s", msg.ID)
	}
	if msg.SenderID != "sender_1" {
		t.Errorf("Expected SenderID 'sender_1', got %s", msg.SenderID)
	}
	if msg.SenderName != "Test User" {
		t.Errorf("Expected SenderName 'Test User', got %s", msg.SenderName)
	}
	if msg.Content != "Hello World" {
		t.Errorf("Expected Content 'Hello World', got %s", msg.Content)
	}
	if msg.IsRead {
		t.Error("Expected IsRead to be false")
	}
	if msg.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", msg.Platform)
	}
}

func TestAutoReplyBot_isCookieExpired(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
	}

	// This method requires browser to evaluate JavaScript
	expired := bot.isCookieExpired()
	_ = expired
}

func TestAutoReplyBot_setupPlatform(t *testing.T) {
	t.Skip("Skipping test that requires browser initialization")
	bot := &AutoReplyBot{
		platform:  Douyin,
		account:   "test_account",
		accountID: 1,
		cookies:   "test=cookie; session=abc123",
	}

	// This method requires browser to navigate
	err := bot.setupPlatform()
	_ = err
}
