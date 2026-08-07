package service

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// TestInboxIngress_DeliverOutbound_OutboxLifecycle 验证人工座席代发经 DeliverOutbound
// 落库 message_hub(direction=outbound, status=pending)，被 ListPendingOutbound 拉取（GET /api/bridge/outbox 下发路径），
// 并在 AckOutboundDelivered 后退出待下发队列。
//
// 回归：旧 EnqueueReply 仅推入已无人读取的 httpReplyBuffer，会导致人工回复静默丢失
// （2026-08-06 三通道架构后 ingest 改为即时返回，buffer 长轮询路径已废弃）。
func TestInboxIngress_DeliverOutbound_OutboxLifecycle(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)

	const (
		channel        = "douyin_web"
		accountID      = "acc_manual_1"
		conversationID = "conv_manual_1"
		content        = "您好，这是人工回复"
	)

	h := &model.MessageHub{
		MsgID:          ContentHashMsgID(channel, conversationID, content),
		Platform:       channel,
		AccountID:      accountID,
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		SenderID:       "agent_1",
		ReceiverID:     conversationID,
		Content:        content,
		ConversationID: conversationID,
		IsAIReply:      false,
		IsRead:         true,
	}
	if err := svc.DeliverOutbound(context.Background(), h); err != nil {
		t.Fatalf("DeliverOutbound 失败: %v", err)
	}

	// 1) 应出现在待下发队列（扩展端 outbox 拉取路径）
	pending, err := svc.ListPendingOutbound(context.Background(), channel, accountID)
	if err != nil {
		t.Fatalf("ListPendingOutbound 失败: %v", err)
	}
	var found *model.MessageHub
	for _, m := range pending {
		if m.MsgID == h.MsgID {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatalf("人工回复未出现在待下发队列，可能静默丢失（旧 EnqueueReply 缺陷）")
	}
	if found.Status != "pending" || found.Direction != "outbound" {
		t.Fatalf("期望 outbound/pending，实际 direction=%q status=%q", found.Direction, found.Status)
	}

	// 2) 扩展端确认下发后，应退出待下发队列
	n, err := svc.AckOutboundDelivered(context.Background(), channel, accountID, []string{h.MsgID})
	if err != nil {
		t.Fatalf("AckOutboundDelivered 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("期望确认 1 条，实际 %d", n)
	}
	pending2, err := svc.ListPendingOutbound(context.Background(), channel, accountID)
	if err != nil {
		t.Fatalf("ListPendingOutbound(2) 失败: %v", err)
	}
	for _, m := range pending2 {
		if m.MsgID == h.MsgID {
			t.Fatalf("确认后人工回复仍出现在待下发队列")
		}
	}
}
