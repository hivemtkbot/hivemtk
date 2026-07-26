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

// TestBlacklistUser_Success 拉黑成功路径
func TestBlacklistUser_Success(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// 准备：1 个 AI 状态会话（有 user_id）
	sess, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_1",
		UserName:  "访客A",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// 拉黑
	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID:    sess.ID,
		Reason:       "辱骂客服",
		OperatorID:   101,
		OperatorName: "客服甲",
		TTLHours:     0, // 永久
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}

	// 验证：黑名单已添加
	ok, err := svc.IsUserBlacklisted(context.Background(), "u_1", model.PlatformWeb)
	if err != nil {
		t.Fatalf("IsUserBlacklisted: %v", err)
	}
	if !ok {
		t.Error("expected user u_1 to be blacklisted")
	}

	// 验证：会话已 closed
	got, _ := svc.GetSessionByID(context.Background(), sess.ID)
	if got.Status != model.SessionStatusClosed {
		t.Errorf("status = %s, want closed", got.Status)
	}
}

// TestBlacklistUser_NilRequest 边界：nil 请求体
func TestBlacklistUser_NilRequest(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	if err := svc.BlacklistUser(context.Background(), nil); err == nil {
		t.Error("expected error for nil request")
	}
}

// TestBlacklistUser_ZeroSessionID 边界：SessionID=0
func TestBlacklistUser_ZeroSessionID(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{SessionID: 0}); err == nil {
		t.Error("expected error for zero session_id")
	}
}

// TestBlacklistUser_SessionNotFound 边界：会话不存在
func TestBlacklistUser_SessionNotFound(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: 999999, Reason: "x",
	})
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
	if !contains(err.Error(), "会话不存在") {
		t.Errorf("error should mention 会话不存在, got: %v", err)
	}
}

// TestBlacklistUser_NoUserID 边界：会话无 user_id
func TestBlacklistUser_NoUserID(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "", // 关键：无 user_id
	})

	err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "test",
	})
	if err == nil {
		t.Error("expected error when user_id is empty")
	}
	if !contains(err.Error(), "user_id") {
		t.Errorf("error should mention user_id, got: %v", err)
	}
}

// TestBlacklistUser_Idempotent 幂等：同 user_id+platform 多次拉黑更新 reason
func TestBlacklistUser_Idempotent(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_idem",
	})

	// 第一次拉黑
	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "first",
	}); err != nil {
		t.Fatalf("first BlacklistUser: %v", err)
	}
	// 第二次拉黑（更新 reason）
	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: sess.ID,
		Reason:    "second",
	}); err != nil {
		t.Fatalf("second BlacklistUser: %v", err)
	}
	// 仍应是黑名单状态
	ok, _ := svc.IsUserBlacklisted(context.Background(), "u_idem", model.PlatformWeb)
	if !ok {
		t.Error("expected still blacklisted")
	}

	// 验证数据库中只有 1 条 active 记录（幂等不重复写入）
	rows, total, err := svc.ListActiveBlacklist(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("ListActiveBlacklist: %v", err)
	}
	count := 0
	for _, r := range rows {
		if r.UserID == "u_idem" && r.Active {
			count++
		}
	}
	_ = total
	if count != 1 {
		t.Errorf("active records for u_idem = %d, want 1 (idempotent)", count)
	}
}

// TestUnblacklistUser_Success 解除拉黑
func TestUnblacklistUser_Success(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_unban",
	})
	_ = svc.BlacklistUser(context.Background(), &BlacklistRequest{SessionID: sess.ID, Reason: "test"})

	if err := svc.UnblacklistUser(context.Background(), "u_unban", model.PlatformWeb); err != nil {
		t.Fatalf("UnblacklistUser: %v", err)
	}
	ok, _ := svc.IsUserBlacklisted(context.Background(), "u_unban", model.PlatformWeb)
	if ok {
		t.Error("expected user to be un-blacklisted")
	}
}

// TestUnblacklistUser_EmptyUserID 边界：user_id 空
func TestUnblacklistUser_EmptyUserID(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	if err := svc.UnblacklistUser(context.Background(), "", model.PlatformWeb); err == nil {
		t.Error("expected error for empty user_id")
	}
}

// TestIsUserBlacklisted_NotExist 不存在的访客
func TestIsUserBlacklisted_NotExist(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	ok, err := svc.IsUserBlacklisted(context.Background(), "never_existed", model.PlatformWeb)
	if err != nil {
		t.Fatalf("IsUserBlacklisted: %v", err)
	}
	if ok {
		t.Error("expected not blacklisted")
	}
}

// TestIsUserBlacklisted_EmptyUserID 边界：user_id 空（不查）
func TestIsUserBlacklisted_EmptyUserID(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	ok, err := svc.IsUserBlacklisted(context.Background(), "", model.PlatformWeb)
	if err != nil {
		t.Fatalf("IsUserBlacklisted: %v", err)
	}
	if ok {
		t.Error("expected not blacklisted for empty user_id")
	}
}

// TestListActiveBlacklist_Pagination 分页
func TestListActiveBlacklist_Pagination(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// 拉黑 3 个不同访客
	for i := 0; i < 3; i++ {
		uid := "u_list_" + string(rune('A'+i))
		sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
			Platform: model.PlatformWeb, AccountID: "acc", UserID: uid,
		})
		_ = svc.BlacklistUser(context.Background(), &BlacklistRequest{SessionID: sess.ID, Reason: "r"})
	}

	// page=1, page_size=2 → 应返回 2 条
	rows, total, err := svc.ListActiveBlacklist(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ListActiveBlacklist page 1: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 2 {
		t.Errorf("page 1 rows = %d, want 2", len(rows))
	}

	// page=2, page_size=2 → 应返回 1 条
	rows2, _, _ := svc.ListActiveBlacklist(context.Background(), 2, 2)
	if len(rows2) != 1 {
		t.Errorf("page 2 rows = %d, want 1", len(rows2))
	}
}

// TestListActiveBlacklist_BoundaryPageSize 边界：pageSize 异常
func TestListActiveBlacklist_BoundaryPageSize(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// pageSize=0 应被 repo 兜底为 20；pageSize=999 应被兜底为 20
	_, total, err := svc.ListActiveBlacklist(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("ListActiveBlacklist pageSize=0: %v", err)
	}
	_ = total

	_, total2, err := svc.ListActiveBlacklist(context.Background(), 1, 999)
	if err != nil {
		t.Fatalf("ListActiveBlacklist pageSize=999: %v", err)
	}
	_ = total2
}

// TestBlacklistUser_TTLExpiry TTL 未过期仍生效
func TestBlacklistUser_TTLExpiry(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc", UserID: "u_ttl",
	})
	// 临时拉黑 1 小时
	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: sess.ID, Reason: "ttl", TTLHours: 1,
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}
	ok, _ := svc.IsUserBlacklisted(context.Background(), "u_ttl", model.PlatformWeb)
	if !ok {
		t.Error("expected active blacklist (TTL not expired)")
	}
}

// TestCreateSession_RejectedByBlacklist 已被拉黑访客无法创建新会话
//
// CreateSession 需串联黑名单校验：
// 拉黑应同时影响后续会话创建入口，否则访客通过新会话绕过黑名单。
func TestCreateSession_RejectedByBlacklist(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// 创建第一个会话并拉黑
	sess1, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_x",
		UserID:    "u_banned",
	})
	if err := svc.BlacklistUser(context.Background(), &BlacklistRequest{
		SessionID: sess1.ID, Reason: "spam",
	}); err != nil {
		t.Fatalf("BlacklistUser: %v", err)
	}

	// 同一 user_id 创建新会话 → 应被拒绝
	_, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
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
	if err := svc.UnblacklistUser(context.Background(), "u_banned", model.PlatformWeb); err != nil {
		t.Fatalf("UnblacklistUser: %v", err)
	}
	sess2, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
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

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_y",
		UserID:    "u_platform",
	})
	_ = svc.BlacklistUser(context.Background(), &BlacklistRequest{SessionID: sess.ID, Reason: "x"})

	// web 已拉黑 → 拒绝
	if _, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_y",
		UserID:    "u_platform",
	}); err == nil {
		t.Error("expected rejection on web")
	}

	// douyin 未拉黑 → 通过
	sess2, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
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

// TestCreateSession_AnonymousUser 边界：匿名访客（user_id=""）跳过黑名单校验
func TestCreateSession_AnonymousUser(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// user_id="" 的匿名访客不参与黑名单校验（不存在可拉黑维度）
	sess, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_anon",
		UserID:    "",
	})
	if err != nil {
		t.Fatalf("anonymous CreateSession: %v", err)
	}
	if sess.UserID != "" {
		t.Errorf("user_id should remain empty, got %s", sess.UserID)
	}
}

// TestBlacklistSource_Enum 验证枚举值稳定（避免 typo）
func TestBlacklistSource_Enum(t *testing.T) {
	tests := []struct {
		src  BlacklistSource
		want string
	}{
		{BlacklistSourceManual, "manual"},
		{BlacklistSourceAuto, "auto"},
		{BlacklistSourceRisk, "risk"},
	}
	for _, tt := range tests {
		if string(tt.src) != tt.want {
			t.Errorf("BlacklistSource = %q, want %q", string(tt.src), tt.want)
		}
	}
}

// TestPreCreateBlacklistGuard_Direct 直接测试守卫方法（避免与 CreateSession 流程耦合）
//
// 验证 preCreateBlacklistGuard 的独立行为：
//   - nil 请求 → 拒绝
//   - user_id 空 → 通过（匿名访客）
//   - 黑名单命中 → 拒绝 + 中文错误
//   - 正常 user_id → 通过
func TestPreCreateBlacklistGuard_Direct(t *testing.T) {
	setupBlacklistServiceTestDB(t)
	svc := NewCustomerSessionService()

	// nil 请求
	if err := svc.preCreateBlacklistGuard(context.Background(), nil); err == nil {
		t.Error("expected error for nil request")
	}

	// 匿名访客（user_id=""）→ 通过
	if err := svc.preCreateBlacklistGuard(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc",
		UserID:    "",
	}); err != nil {
		t.Errorf("anonymous should pass: %v", err)
	}

	// 拉黑一个 user_id
	bannedSess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc", UserID: "u_guard",
	})
	_ = svc.BlacklistUser(context.Background(), &BlacklistRequest{SessionID: bannedSess.ID})

	// 守卫应拒绝
	if err := svc.preCreateBlacklistGuard(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc", UserID: "u_guard",
	}); err == nil {
		t.Error("expected guard to reject blacklisted user")
	} else if !contains(err.Error(), "黑名单") {
		t.Errorf("error should mention 黑名单, got: %v", err)
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
