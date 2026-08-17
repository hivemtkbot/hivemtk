package service

import (
	"context"
	"testing"
)

// TestWechatService_GetFirstActiveAccount_NilDB nil DB 时安全返回错误
func TestWechatService_GetFirstActiveAccount_NilDB(t *testing.T) {
	svc := NewWechatService(nil)
	acc, err := svc.GetFirstActiveAccount(context.Background())
	if err == nil {
		t.Fatalf("expected error when db is nil, got acc=%+v", acc)
	}
}

// TestWechatService_SendCustomMessage_NilDB 验证 SendCustomMessage 在 db=nil 时优雅降级
func TestWechatService_SendCustomMessage_NilDB(t *testing.T) {
	svc := NewWechatService(nil)
	_, err := svc.SendCustomMessage(context.Background(), 0, "openid", "text", "hello")
	if err == nil {
		t.Fatalf("expected error when db is nil, got nil")
	}
}

// TestWechatService_GetAccount_NilDB 验证 GetAccount 在 db=nil 时优雅降级
func TestWechatService_GetAccount_NilDB(t *testing.T) {
	svc := NewWechatService(nil)
	_, err := svc.GetAccount(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected error when db is nil, got nil")
	}
}

// TestWechatService_ListAccounts_NilDB 验证 ListAccounts 在 db=nil 时优雅降级
func TestWechatService_ListAccounts_NilDB(t *testing.T) {
	svc := NewWechatService(nil)
	accs, err := svc.ListAccounts(context.Background())
	if err == nil {
		t.Fatalf("expected error when db is nil, got accs=%+v", accs)
	}
}
