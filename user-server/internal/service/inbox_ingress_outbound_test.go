package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// 验证三通道架构的核心 DB 路径：下行队列查询（通道C·GET /api/bridge/outbox）
// 与已下发确认（通道B·POST /api/bridge/outbox/ack）。
// 这两个 handler 是 ListPendingOutbound / AckOutboundDelivered 的薄封装，
// 本测试直接覆盖其底层 DB 逻辑（本地 PG 8232，连接失败时 t.Skip）。
func TestInboxIngress_OutboundRoundTrip(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	// 插入一条 pending 出站（模拟 AI 占位 → 入下发队列）
	if err := db.Create(&model.MessageHub{
		MsgID:          "ob1",
		Platform:       "douyin",
		AccountID:      "1",
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		ConversationID: "c1",
		Content:        "AI 回复",
	}).Error; err != nil {
		t.Fatalf("插入下发消息失败: %v", err)
	}

	// 通道C·拉取下发队列
	msgs, err := svc.ListPendingOutbound(ctx, "douyin", "1")
	if err != nil {
		t.Fatalf("ListPendingOutbound 失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MsgID != "ob1" {
		t.Fatalf("下发队列应返回 1 条 ob1, 实际 %+v", msgs)
	}

	// 通道B·确认已下发
	n, err := svc.AckOutboundDelivered(ctx, "douyin", "1", []string{"ob1"})
	if err != nil {
		t.Fatalf("AckOutboundDelivered 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("ack 应标记 1 条, 实际 %d", n)
	}

	// 确认后队列清空
	msgs2, _ := svc.ListPendingOutbound(ctx, "douyin", "1")
	if len(msgs2) != 0 {
		t.Fatalf("ack 后下发队列应清空, 实际 %d", len(msgs2))
	}
}

// 越权 ack 归属校验：用其它 account 确认他人消息不生效（通道B 归属校验）。
func TestInboxIngress_AckOutboundScopeIsolation(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	db.Create(&model.MessageHub{
		MsgID:          "ob2",
		Platform:       "douyin",
		AccountID:      "1",
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		ConversationID: "c1",
		Content:        "AI 回复",
	})

	// 用 account=2 确认 account=1 的消息 → 应不生效
	n, err := svc.AckOutboundDelivered(ctx, "douyin", "2", []string{"ob2"})
	if err != nil {
		t.Fatalf("AckOutboundDelivered 失败: %v", err)
	}
	if n != 0 {
		t.Fatalf("越权 ack 不应生效, 实际 %d", n)
	}
	// 原消息仍在队列
	msgs, _ := svc.ListPendingOutbound(ctx, "douyin", "1")
	if len(msgs) != 1 {
		t.Fatalf("越权 ack 不应清除原账号消息, 实际 %d", len(msgs))
	}
}
