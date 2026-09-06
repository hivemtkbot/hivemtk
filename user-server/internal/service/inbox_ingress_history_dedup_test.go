package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func TestPersistBridgeHistory_HistoryEchoDedupHit_Skipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-xhs-hd"
		conv     = "conv-xhs-hd-1"
		content  = "你好呀！😊 很高兴与你交流～有任何产品问题随时问我"
		hashID   = "mh:hd000001"
	)

	oldTime := time.Now().Add(-1 * time.Hour)
	if err := db.Create(&model.MessageHub{
		MsgID:          hashID,
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		Content:        content,
		SentAt:         oldTime,
	}).Error; err != nil {
		t.Fatalf("插入历史 outbound 失败: %v", err)
	}

	histEvent := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs-001",
		SenderType:     "customer",
		Content:        content,
		EventID:        hashID,
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	if err := svc.PersistBridgeHistory(ctx, histEvent, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory 不应报错（命中即幂等跳过）：%v", err)
	}

	var inboundDupCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='inbound' AND msg_id=?", platform, conv, hashID).
		Count(&inboundDupCount)
	if inboundDupCount != 0 {
		t.Fatalf("history 中的 AI 回复不得重入库为 inbound（应幂等跳过），实际 inbound 条数=%d", inboundDupCount)
	}

	var outboundRow model.MessageHub
	if err := db.Where("msg_id=? AND direction='outbound'", hashID).First(&outboundRow).Error; err != nil {
		t.Fatalf("原 outbound 应仍在：%v", err)
	}
	if outboundRow.Status != "delivered" {
		t.Fatalf("原 outbound 状态不应被修改，仍应 delivered，实际=%s", outboundRow.Status)
	}

	var totalInboundForConv int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='inbound'", platform, conv).
		Count(&totalInboundForConv)
	if totalInboundForConv != 0 {
		t.Fatalf("history 命中幂等跳过后，该会话不应产生任何 inbound，实际=%d", totalInboundForConv)
	}
}

// 对照（防过度跳过）：history 中的全新消息（msg_id 不存在任何记录）必须正常入库为 inbound，
// 证明修复仅对已存在 msg_id 生效，不会误拦真实历史消息。
func TestPersistBridgeHistory_NewHistoryMessage_NotSkipped(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-xhs-nh"
		conv     = "conv-xhs-nh-1"
		content  = "这是页面加载时回填的真实历史消息"
		newMsgID = "mh:hd0000ff"
	)

	histEvent := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs-002",
		SenderType:     "customer",
		Content:        content,
		EventID:        newMsgID,
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}
	if err := svc.PersistBridgeHistory(ctx, histEvent, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory 不应报错：%v", err)
	}

	var row model.MessageHub
	if err := db.Where("msg_id=? AND direction='inbound'", newMsgID).First(&row).Error; err != nil {
		t.Fatalf("全新 history 应落库为 inbound：%v", err)
	}
	if row.Content != content {
		t.Fatalf("入库内容不符：got=%s want=%s", row.Content, content)
	}
}

// 回归测试：history 中存在与"已入库 outbound"同 MsgID 的项时，outbox 不应被新追加。
// 这间接验证：修复后不会因 history 回填而触发新一轮 outbound（消息无新增）。
func TestPersistBridgeHistory_HistoryEcho_NoNewOutboundAppended(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "xiaohongshu"
		account  = "acct-xhs-nout"
		conv     = "conv-xhs-nout-1"
		content  = "已下发的 AI 回复"
		hashID   = "mh:nout0001"
	)

	if err := db.Create(&model.MessageHub{
		MsgID:          hashID,
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		Content:        content,
		SentAt:         time.Now().Add(-30 * time.Minute),
	}).Error; err != nil {
		t.Fatalf("预置 outbound 失败: %v", err)
	}

	if err := svc.PersistBridgeHistory(ctx, &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       "customer-xhs-003",
		SenderType:     "customer",
		Content:        content,
		EventID:        hashID,
		ConversationID: conv,
		Extra:          map[string]interface{}{"account_id": account},
	}, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory 不应报错：%v", err)
	}

	var outboundCount int64
	db.Model(&model.MessageHub{}).
		Where("platform=? AND conversation_id=? AND direction='outbound'", platform, conv).
		Count(&outboundCount)
	if outboundCount != 1 {
		t.Fatalf("history 命中幂等后该会话 outbound 应仍为 1 条，实际=%d", outboundCount)
	}
}
