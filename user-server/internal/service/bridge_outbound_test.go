package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// setupBridgeWhitelistForTest 注入测试白名单（B-5 后白名单由 bridge 包运行时注入，
// service 包内测试需自行设置；返回恢复函数避免污染其他测试）。
func setupBridgeWhitelistForTest(t *testing.T, channels ...string) {
	t.Helper()
	prev := make(map[string]struct{}, len(bridgeChannels))
	for k := range bridgeChannels {
		prev[k] = struct{}{}
	}
	SetBridgeChannels(channels)
	t.Cleanup(func() {
		names := make([]string, 0, len(prev))
		for k := range prev {
			names = append(names, k)
		}
		SetBridgeChannels(names)
	})
}

// TestDeliverBridgeOutbound_DirectToOutbox 2026-08-18 二次审核修复回归：
// 主动外联（proactive_reach / douyin_integration / AI Agent reach.*.send）经
// DeliverBridgeOutbound 必须直接落 message_hub(status=pending)，被 ListPendingOutbound
// 拉取（GET /api/bridge/outbox 下发路径），并在 AckOutboundDelivered 后退出待下发队列。
//
// 回归：旧 EnqueueReply 仅推入已无人读取的 httpReplyBuffer，会导致主动外联消息
// 静默丢失（httpReplyBuffer 的 Pull 没有任何生产调用方）。修复后 DeliverBridgeOutbound
// 走 InboxIngressService.DeliverOutbound（与 AI 回复同队列），保证可靠投递。
func TestDeliverBridgeOutbound_DirectToOutbox(t *testing.T) {
	setupBridgeWhitelistForTest(t, "douyin")
	db := testutil.NewTestDB(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	SetGlobalInboxIngressService(svc)
	defer SetGlobalInboxIngressService(nil) // 避免污染其他测试

	const (
		channel        = "douyin"
		accountID      = "acc_proactive_1"
		conversationID = "conv_proactive_1"
		content        = "主动外联消息"
	)

	if err := DeliverBridgeOutbound(context.Background(),
		channel, accountID, conversationID, "text", content, "evt_1"); err != nil {
		t.Fatalf("DeliverBridgeOutbound 失败: %v", err)
	}

	// 验证：消息已落 message_hub 并被 ListPendingOutbound 拉取
	pending, err := svc.ListPendingOutbound(context.Background(), channel, accountID)
	if err != nil {
		t.Fatalf("ListPendingOutbound 失败: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("期望 1 条 pending 消息，实际 %d 条", len(pending))
	}
	got := pending[0]
	if got.Content != content {
		t.Errorf("Content = %q, want %q", got.Content, content)
	}
	if got.AccountID != accountID {
		t.Errorf("AccountID = %q, want %q", got.AccountID, accountID)
	}
	if got.ConversationID != conversationID {
		t.Errorf("ConversationID = %q, want %q", got.ConversationID, conversationID)
	}
	if got.Status != "pending" {
		t.Errorf("Status = %q, want pending", got.Status)
	}
	if got.Direction != "outbound" {
		t.Errorf("Direction = %q, want outbound", got.Direction)
	}
	if got.IsAIReply {
		t.Error("IsAIReply 应为 false（主动外联非 AI 回复）")
	}

	// 验证：ack 后退出待下发队列
	affected, err := svc.AckOutboundDelivered(context.Background(), channel, accountID, []string{got.MsgID})
	if err != nil {
		t.Fatalf("AckOutboundDelivered 失败: %v", err)
	}
	if affected != 1 {
		t.Errorf("AckOutboundDelivered affected = %d, want 1", affected)
	}
	pending2, _ := svc.ListPendingOutbound(context.Background(), channel, accountID)
	if len(pending2) != 0 {
		t.Errorf("ack 后 pending 应为 0，实际 %d", len(pending2))
	}
}

// TestDeliverBridgeOutbound_RejectsPlaceholderAccount 验证 S2 修复：
// 占位账号 `<channel>-unknown` 主动外联时直接拒绝，不污染 outbox 队列。
func TestDeliverBridgeOutbound_RejectsPlaceholderAccount(t *testing.T) {
	setupBridgeWhitelistForTest(t, "douyin")
	db := testutil.NewTestDB(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	SetGlobalInboxIngressService(svc)
	defer SetGlobalInboxIngressService(nil)

	const (
		channel        = "douyin"
		accountID      = "douyin-unknown" // 占位账号
		conversationID = "conv_x"
	)
	err := DeliverBridgeOutbound(context.Background(),
		channel, accountID, conversationID, "text", "测试", "evt_1")
	if err == nil {
		t.Fatal("占位账号应被拒绝，但 DeliverBridgeOutbound 返回 nil")
	}

	// 验证：未落库
	pending, _ := svc.ListPendingOutbound(context.Background(), channel, accountID)
	if len(pending) != 0 {
		t.Errorf("占位账号消息不应落库，实际有 %d 条", len(pending))
	}
}

// TestDeliverBridgeOutbound_RejectsUnsupportedChannel 验证：非白名单渠道直接拒绝。
func TestDeliverBridgeOutbound_RejectsUnsupportedChannel(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	SetGlobalInboxIngressService(svc)
	defer SetGlobalInboxIngressService(nil)

	err := DeliverBridgeOutbound(context.Background(),
		"evil_unknown", "acc_1", "conv_1", "text", "测试", "evt_1")
	if err == nil {
		t.Fatal("非白名单渠道应被拒绝，但 DeliverBridgeOutbound 返回 nil")
	}
}

// TestDeliverBridgeOutbound_NotReady 验证：未装配（globalInboxIngressService == nil）时
// 返回 errBridgeOutboundNotReady，便于装配时序问题排查。
func TestDeliverBridgeOutbound_NotReady(t *testing.T) {
	prev := GlobalInboxIngressService()
	defer SetGlobalInboxIngressService(prev)
	SetGlobalInboxIngressService(nil)

	err := DeliverBridgeOutbound(context.Background(),
		"douyin", "acc_1", "conv_1", "text", "测试", "evt_1")
	if err == nil {
		t.Fatal("未装配应返回 error")
	}
	if err.Error() != "bridge outbound not ready: InboxIngressService not registered" {
		t.Errorf("err = %q, want errBridgeOutboundNotReady", err.Error())
	}
}

// TestSetBridgeChannels_SingleSource B-5 白名单单源化回归：
// service 包不再手工维护渠道清单，白名单完全由 SetBridgeChannels（bridge init 从
// channelgw.Default 注入）决定；空注入必须被忽略以防误清空。
func TestSetBridgeChannels_SingleSource(t *testing.T) {
	setupBridgeWhitelistForTest(t, "douyin", "xianyu")

	if !isBridgeChannel("douyin") || !isBridgeChannel("xianyu") {
		t.Error("注入的渠道应在白名单内")
	}
	if isBridgeChannel("tiktok") {
		t.Error("未注入渠道不应在白名单内")
	}

	// 空注入忽略：白名单保持不变
	SetBridgeChannels(nil)
	if !isBridgeChannel("douyin") {
		t.Error("空注入应被忽略，白名单不应被清空")
	}

	// 重建语义：以最近一次有效注入为准
	SetBridgeChannels([]string{"kuaishou"})
	if isBridgeChannel("douyin") || !isBridgeChannel("kuaishou") {
		t.Error("白名单应以最近一次注入为准（运行时单一来源）")
	}
}
