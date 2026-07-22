package service

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"
)

// setupBlacklistServiceTestDB 初始化测试数据库（包含 customer_sessions + user_blacklist）
func setupBlacklistServiceTestDB(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.UserBlacklist{},
		&model.AgentStatus{},
	)
	// 让全局 db 指向测试库（service 内 New*() 使用）
	db.SetTestDB(database)
}

func TestBlacklistUser_Success(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()
	ctx := context.Background()

	// 准备：1 个 AI 状态会话（有 user_id）
	sess, err := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_1",
		UserName:  "访客A",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// 拉黑
	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID:    sess.ID,
		Reason:       "辱骂客服",
		OperatorID:   101,
		OperatorName: "客服甲",
		TTLHours:     0, // 永久
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}

	// 验证：黑名单已添加
	ok, err := svc.IsUserBlacklisted("u_1", model.PlatformWeb)
	if err != nil {
		t.Fatalf("IsUserBlacklisted: %v", err)
	}
	if !ok {
		t.Error("expected user u_1 to be blacklisted")
	}

	// 验证：会话已 closed
	got, _ := svc.GetSessionByID(sess.ID)
	if got.Status != model.SessionStatusClosed {
		t.Errorf("status = %s, want closed", got.Status)
	}
}

func TestBlacklistUser_NoUserID(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "", // 关键：无 user_id
	})

	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "test",
	}); err == nil {
		t.Error("expected error when user_id is empty")
	}
}

func TestBlacklistUser_Idempotent(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_idem",
	})

	// 第一次拉黑
	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "first",
	}); err != nil {
		t.Fatalf("first BlacklistUser: %v", err)
	}
	// 第二次拉黑（更新 reason）
	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "second",
	}); err != nil {
		t.Fatalf("second BlacklistUser: %v", err)
	}
	// 仍应是黑名单状态
	ok, _ := svc.IsUserBlacklisted("u_idem", model.PlatformWeb)
	if !ok {
		t.Error("expected still blacklisted")
	}
}

func TestUnblacklistUser_Success(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_unban",
	})
	_ = svc.BlacklistUser(&BlacklistRequest{SessionID: sess.ID, Reason: "test"})

	if err := svc.UnblacklistUser("u_unban", model.PlatformWeb); err != nil {
		t.Fatalf("UnblacklistUser: %v", err)
	}
	ok, _ := svc.IsUserBlacklisted("u_unban", model.PlatformWeb)
	if ok {
		t.Error("expected user to be un-blacklisted")
	}
}

func TestIsUserBlacklisted_NotExist(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()
	ok, err := svc.IsUserBlacklisted("never_existed", model.PlatformWeb)
	if err != nil {
		t.Fatalf("IsUserBlacklisted: %v", err)
	}
	if ok {
		t.Error("expected not blacklisted")
	}
}

func TestListBlacklist_Pagination(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// 拉黑 3 个不同访客
	for i := 0; i < 3; i++ {
		uid := "u_list_" + string(rune('A'+i))
		sess, _ := svc.CreateSession(&CreateSessionRequest{
			Platform: model.PlatformWeb, AccountID: "acc", UserID: uid,
		})
		_ = svc.BlacklistUser(&BlacklistRequest{SessionID: sess.ID, Reason: "r"})
	}

	rows, total, err := svc.ListBlacklist(1, 10)
	if err != nil {
		t.Fatalf("ListBlacklist: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 3 {
		t.Errorf("rows = %d, want 3", len(rows))
	}
}

func TestBlacklistUser_TTLExpiry(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(&CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc", UserID: "u_ttl",
	})
	// 临时拉黑 1 小时
	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID: sess.ID, Reason: "ttl", TTLHours: 1,
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}
	ok, _ := svc.IsUserBlacklisted("u_ttl", model.PlatformWeb)
	if !ok {
		t.Error("expected active blacklist (TTL not expired)")
	}
}

// TestCreateSession_RejectedByBlacklist 已被拉黑访客无法创建新会话
//
// 修复二次审核发现的 CreateSession 未串联黑名单校验的遗漏：
// 拉黑应同时影响后续会话创建入口，否则访客通过新会话绕过黑名单。
func TestCreateSession_RejectedByBlacklist(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// 创建第一个会话并拉黑
	sess1, _ := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_x",
		UserID:    "u_banned",
	})
	if err := svc.BlacklistUser(&BlacklistRequest{
		SessionID: sess1.ID, Reason: "spam",
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}

	// 同一 user_id 创建新会话 → 应被拒绝
	_, err := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_x",
		UserID:    "u_banned",
	})
	if err == nil {
		t.Fatal("expected CreateSession to be rejected for blacklisted user")
	}
	if !contains(err.Error(), "黑名单") {
		t.Errorf("error message should mention 黑名单, got: %v", err)
	}

	// 解除拉黑 → 新会话应恢复
	if err := svc.UnblacklistUser("u_banned", model.PlatformWeb); err != nil {
		t.Fatalf("UnblacklistUser: %v", err)
	}
	sess2, err := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_x",
		UserID:    "u_banned",
	})
	if err != nil {
		t.Fatalf("after unblacklist CreateSession: %v", err)
	}
	if sess2 == nil {
		t.Error("expected new session after unblacklist")
	}
}

// TestCreateSession_AllowDifferentPlatform 跨 platform 独立
//
// 同一 user_id 在 platform=web 拉黑后，在 platform=douyin 仍可创建会话
// （黑名单按 platform 维度隔离）
func TestCreateSession_AllowDifferentPlatform(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_y",
		UserID:    "u_platform",
	})
	_ = svc.BlacklistUser(&BlacklistRequest{SessionID: sess.ID, Reason: "x"})

	// web 已拉黑 → 拒绝
	if _, err := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_y",
		UserID:    "u_platform",
	}); err == nil {
		t.Error("expected rejection on web")
	}

	// douyin 未拉黑 → 通过
	sess2, err := svc.CreateSession(&CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "acc_y",
		UserID:    "u_platform",
	})
	if err != nil {
		t.Fatalf("douyin CreateSession: %v", err)
	}
	if sess2.Platform != model.PlatformDouyin {
		t.Errorf("platform = %v, want douyin", sess2.Platform)
	}
}

// contains 简单子串匹配（避免引入 strings 包与现有 import 冲突）
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
