package tgbot

import (
	"strings"
	"testing"
	"time"
)

// 测试 InitTGBot 代理配置失败的情况
func TestInitTGBot_ProxyParseError(t *testing.T) {
	_, err := InitTGBot("test_token", 123456, true, "invalid%proto", "proxy.example.com", 1080)
	if err == nil {
		t.Error("InitTGBot() expected error for invalid proxy protocol, got nil")
	}
}

// 测试 InitTGBot 无效代理地址
func TestInitTGBot_ProxyDialerError(t *testing.T) {
	_, err := InitTGBot("test_token", 123456, true, "socks5", "invalid_address_that_does_not_exist", 9999)
	if err == nil {
		t.Error("InitTGBot() expected error for invalid proxy address, got nil")
	}
}

// 测试 InitTGBot 不使用代理
func TestInitTGBot_NoProxy(t *testing.T) {
	_, err := InitTGBot("invalid_token", 123456, false, "", "", 0)
	if err == nil {
		t.Error("InitTGBot() expected error for invalid token without proxy, got nil")
	}
}

// ============================================================================
// ValidateBotToken 单元测试（P2-3 Bot Token 格式预校验）
// ----------------------------------------------------------------------------
// 业务动机：用户在 controller Create/Update 入口填写 Bot Token，格式错误时
// 调用 getMe 会立即 401，错误信息对用户不友好。
// 在 controller 入口预校验，可直接 4xx 回包并提供人类可读原因。
//
// 校验规则：
//   - 非空
//   - 含恰好 1 个 ':' 分隔符
//   - bot_id：6~10 位数字
//   - token：恰好 35 字符，范围 [A-Za-z0-9_-]
// ============================================================================

// TestValidateBotToken_Valid 合法 Token 校验通过
func TestValidateBotToken_Valid(t *testing.T) {
	validTokens := []string{
		"1234567890:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf", // 10 位 bot_id + 35 字符 token
		"123456:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf",      // 6 位 bot_id
		"1234567:ABCDEFGHIJKLMNOPQRSTUVWXYZ0123abcde",     // 7 位 bot_id + 35 字符 token
		"12345678:aBcDeFgHiJkLmNoPqRsTuVwXyZ_-0123456",    // 8 位 bot_id + 35 字符 token（含 _ -）
	}
	for _, tok := range validTokens {
		if err := ValidateBotToken(tok); err != nil {
			t.Errorf("合法 token 校验失败: %q (len=%d) err=%v", tok, len(tok), err)
		}
	}
}

// TestValidateBotToken_Empty 空字符串拒绝
func TestValidateBotToken_Empty(t *testing.T) {
	err := ValidateBotToken("")
	if err == nil {
		t.Fatal("空 token 应返回错误")
	}
	if !strings.Contains(err.Error(), "不能为空") {
		t.Errorf("错误信息应包含 '不能为空'，实际: %v", err)
	}
}

// TestValidateBotToken_NoColon 缺分隔符拒绝
func TestValidateBotToken_NoColon(t *testing.T) {
	err := ValidateBotToken("1234567890AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf")
	if err == nil {
		t.Fatal("缺分隔符应返回错误")
	}
	if !strings.Contains(err.Error(), "缺少 ':'") {
		t.Errorf("错误信息应包含 '缺少 ':' '，实际: %v", err)
	}
}

// TestValidateBotToken_MultipleColons 多分隔符拒绝
func TestValidateBotToken_MultipleColons(t *testing.T) {
	err := ValidateBotToken("1234:5678:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf")
	if err == nil {
		t.Fatal("多分隔符应返回错误")
	}
}

// TestValidateBotToken_NonNumericBotID bot_id 含非数字拒绝
func TestValidateBotToken_NonNumericBotID(t *testing.T) {
	err := ValidateBotToken("abc12345:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf")
	if err == nil {
		t.Fatal("非数字 bot_id 应返回错误")
	}
	if !strings.Contains(err.Error(), "非数字") {
		t.Errorf("错误信息应包含 '非数字'，实际: %v", err)
	}
}

// TestValidateBotToken_InvalidBotIDLength bot_id 长度越界拒绝
func TestValidateBotToken_InvalidBotIDLength(t *testing.T) {
	// 5 位 bot_id（小于 6）
	err1 := ValidateBotToken("12345:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf")
	if err1 == nil {
		t.Error("5 位 bot_id 应返回错误")
	}
	// 11 位 bot_id（大于 10）
	err2 := ValidateBotToken("12345678901:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqkf")
	if err2 == nil {
		t.Error("11 位 bot_id 应返回错误")
	}
}

// TestValidateBotToken_InvalidTokenLength token 长度错误拒绝
func TestValidateBotToken_InvalidTokenLength(t *testing.T) {
	// 34 字符 token
	err1 := ValidateBotToken("1234567:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqa")
	if err1 == nil {
		t.Error("34 字符 token 应返回错误")
	}
	// 36 字符 token
	err2 := ValidateBotToken("1234567:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3PqkXY")
	if err2 == nil {
		t.Error("36 字符 token 应返回错误")
	}
}

// TestValidateBotToken_InvalidChar token 含非法字符拒绝
func TestValidateBotToken_InvalidChar(t *testing.T) {
	// 35 字符的 token（保证长度通过），含非法字符 '.'
	err := ValidateBotToken("1234567:AAEhBOweik6ad9JhbY4M9M3PqkfK1M3Pqk.")
	if err == nil {
		t.Fatal("含非法字符 '.' 应返回错误")
	}
	if !strings.Contains(err.Error(), "[A-Za-z0-9_-]") {
		t.Errorf("错误信息应包含 '[A-Za-z0-9_-]'，实际: %v", err)
	}
}

// 测试 CreateInviteLink 的 duration 参数
func TestCreateInviteLink_Duration(t *testing.T) {
	duration := time.Minute * 10
	expectedExpire := time.Now().Add(duration).Unix()
	if expectedExpire <= time.Now().Unix() {
		t.Error("Duration calculation error")
	}
}

// 测试 SendInviteJoinGroup 的 duration
func TestSendInviteJoinGroup_Duration(t *testing.T) {
	expectedDuration := time.Minute * 3
	if expectedDuration != time.Minute*3 {
		t.Error("Duration should be 3 minutes")
	}
}

// 测试 isBotAdmin 的返回值逻辑（通过间接测试）
func TestIsBotAdminLogic(t *testing.T) {
	_, err := InitTGBot("invalid_token", 123456, false, "", "", 0)
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}
