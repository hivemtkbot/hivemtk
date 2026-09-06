package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestAnyExistsByMsgIDs_AcrossAccount_P0_6(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	if err := db.Exec("DELETE FROM message_hub WHERE platform = ?", "douyin_web").Error; err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	repo := &MessageHubRepository{db: db}
	ctx := context.Background()

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

	probe := []string{"m_same", "m_cross_out", "m_cross_in", "m_not_exist"}
	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", probe)
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}

	expects := map[string]bool{
		"m_same":      true,
		"m_cross_out": true,
		"m_cross_in":  true,
		"m_not_exist": false,
	}
	for id, want := range expects {
		if got := out[id]; got != want {
			t.Errorf("AnyExistsByMsgIDs[%s] = %v，期望 %v", id, got, want)
		}
	}
}

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

	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if !out["m_shared"] {
		t.Errorf("douyin_web 应存在 m_shared，实际不存在")
	}

	out2, err := repo.AnyExistsByMsgIDs(ctx, "xhs_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if !out2["m_shared"] {
		t.Errorf("xhs_web 应存在 m_shared，实际不存在")
	}

	out3, err := repo.AnyExistsByMsgIDs(ctx, "tiktok_web", []string{"m_shared"})
	if err != nil {
		t.Fatalf("AnyExistsByMsgIDs 失败: %v", err)
	}
	if out3["m_shared"] {
		t.Errorf("tiktok_web 不应存在 m_shared，实际存在")
	}
}

func TestAnyExistsByMsgIDs_EmptyAndNil_P0_6(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	repo := &MessageHubRepository{db: db}
	ctx := context.Background()

	out, err := repo.AnyExistsByMsgIDs(ctx, "douyin_web", nil)
	if err != nil {
		t.Errorf("空 msgIDs 不应报错: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("空 msgIDs 应返回空 map，实际 %d", len(out))
	}

	var nilRepo *MessageHubRepository
	out2, err := nilRepo.AnyExistsByMsgIDs(ctx, "douyin_web", []string{"x"})
	if err != nil {
		t.Errorf("nil repo 不应报错: %v", err)
	}
	if len(out2) != 0 {
		t.Errorf("nil repo 应返回空 map，实际 %d", len(out2))
	}
}
