package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestAnyExistsByMsgIDs_AcrossAccount_P0_6
// 验证 P0-6：AnyExistsByMsgIDs 在跨账号探测场景下正确识别"被其他账号持有"的 msg_id。
//
// 安全语义：
//   - ack 端点：扩展用 (channel, account_id_A) 请求 ack msg_id=X
//   - 若 X 在 channel 下被 account_id_B 持有（inbound 或 outbound），则调用方不能"看到归属"
//   - AnyExistsByMsgIDs 仅返回"是否存在"布尔值，不返回归属账号（防越权）
//
// 测试矩阵：
//   - same_account_pending        → 存在（自账号持 outbound）
//   - cross_account_pending       → 存在（被探测：防越权关键）
//   - cross_account_inbound       → 存在（customer 消息持有同 msg_id）
//   - non_existent                → 不存在（真 GC 回收或伪造）
//   - mixed list（4 条）           → 正确返回 3 true + 1 false
func TestAnyExistsByMsgIDs_AcrossAccount_P0_6(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub WHERE platform = ?", "douyin_web").Error; err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	repo := &MessageHubRepository{db: db}
	ctx := context.Background()

	// seed
	seeds := []*model.MessageHub{
		{Platform: "douyin_web", AccountID: "acc_A", ConversationID: "c1", MsgID: "m_same", MsgType: "text", Content: "self", Direction: "outbound", Status: "pending"},
		{Platform: "douyin_web", AccountID: "acc_B", ConversationID: "c2", MsgID: "m_cross_out", MsgType: "text", Content: "cross out", Direction: "outbound", Status: "pending"},
		{Platform: "douyin_web", AccountID: "acc_B", ConversationID: "c3", MsgID: "m_cross_in", MsgType: "text", Content: "cross in", Direction: "inbound", Status: "delivered"},
	}
	for _, h := range seeds {
		if err := db.Create(h).Error; err != nil {
			t.Fatalf("seed %s 失败: %v", h.MsgID, err)
		}
	}

	// 探测：账号 A 询问 4 条 msg_id 是否存在
	probe := []string{"m_same", "m_cross_out", "m_cross_in", "m_not_exist"}
	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", probe)
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}

	// 期望：m_same=true, m_cross_out=true, m_cross_in=true, m_not_exist=false
	expects := map[string]bool{
		"m_same":       true,
		"m_cross_out":  true,
		"m_cross_in":   true,
		"m_not_exist":  false,
	}
	for id, want := range expects {
		if got := out[id]; got != want {
			t.Errorf("AnyExistsByMsgIDs[%s] = %v，期望 %v", id, got, want)
		}
	}
}

// TestAnyExistsByMsgIDs_ChannelIsolation_P0_6
// 验证 P0-6：AnyExistsByMsgIDs 严格按 channel 隔离，不跨 channel 返回。
//
// 场景：
//   - 同一 msg_id="m_shared" 在 douyin_web 和 xhs_web 下各 1 条
//   - 探测 channel=douyin_web 时，仅 douyin_web 的存在性被返回（xhs 不应干扰）
func TestAnyExistsByMsgIDs_ChannelIsolation_P0_6(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub").Error; err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	repo := &MessageHubRepository{db: db}
	ctx := context.Background()

	if err := db.Create(&model.MessageHub{
		Platform: "douyin_web", AccountID: "acc_dy", ConversationID: "c1", MsgID: "m_shared",
		MsgType: "text", Content: "dy", Direction: "outbound", Status: "pending",
	}).Error; err != nil {
		t.Fatalf("seed dy 失败: %v", err)
	}
	if err := db.Create(&model.MessageHub{
		Platform: "xhs_web", AccountID: "acc_xhs", ConversationID: "c1", MsgID: "m_shared",
		MsgType: "text", Content: "xhs", Direction: "outbound", Status: "pending",
	}).Error; err != nil {
		t.Fatalf("seed xhs 失败: %v", err)
	}

	// 探测 douyin_web
	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if !out["m_shared"] {
		t.Errorf("douyin_web 应存在 m_shared，实际不存在")
	}

	// 探测 xhs_web
	out2, err := repo.AnyExistsByMsgIDs(ctx, "xhs_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if !out2["m_shared"] {
		t.Errorf("xhs_web 应存在 m_shared，实际不存在")
	}

	// 探测一个不存在的 channel
	out3, err := repo.AnyExistsByMsgIDs(ctx, "tiktok_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if out3["m_shared"] {
		t.Errorf("tiktok_web 不应存在 m_shared，实际存在")
	}
}

// TestAnyExistsByMsgIDs_EmptyAndNil_P0_6
// 验证 P0-6：边界条件（空 msgIDs / nil repo）安全返回。
func TestAnyExistsByMsgIDs_EmptyAndNil_P0_6(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	repo := &MessageHubRepository{db: db}
	ctx := context.Background()

	// 空 msgIDs → 空 map
	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", nil)
	if err != nil {
		t.Errorf("空 msgIDs 不应报错: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("空 msgIDs 应返回空 map，实际 %d", len(out))
	}

	// nil repo → 空 map
	var nilRepo *MessageHubRepository
	out2, err := nilRepo.AnyExistsByMsgIDs(ctx, "douyin_web", []string{"x"})
	if err != nil {
		t.Errorf("nil repo 不应报错: %v", err)
	}
	if len(out2) != 0 {
		t.Errorf("nil repo 应返回空 map，实际 %d", len(out2))
	}
}
