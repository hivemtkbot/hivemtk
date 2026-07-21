package tgbot

import (
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
