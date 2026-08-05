package bridge

import (
	"context"
	"testing"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/service"
)

// TestBridgeOutbound_DeliversViaHTTPBuffer 验证桥接出站接线（HTTP-only 模式）：
// WebhookService.sendOutbound 在桥接渠道(douyin_web)下，经 service.DeliverBridgeOutbound
// 注册的回调把 AI 回复入 httpReplyBuffer，等下次同会话 /api/bridge/ingest 长轮询拉到。
//
// 历史：WS 模式验证 c.send channel 收到 Frame；HTTP 模式无 WS，验证 buffer 中有匹配 reply 即可。
func TestBridgeOutbound_DeliversViaHTTPBuffer(t *testing.T) {
	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, service.NewInboxIngressServiceWithDB(nil, nil))
	SetBridgeReachAdapter(adapter)

	if err := service.DeliverBridgeOutbound(context.Background(), ChannelDouyinWeb, "acc1", "conv1", "text", "你好，这是AI回复", "evt-1"); err != nil {
		t.Fatalf("DeliverBridgeOutbound error: %v", err)
	}

	reply := adapter.httpReplyBuffer.Pull(ChannelDouyinWeb, "conv1", "evt-1")
	if reply == nil {
		t.Fatal("buffer 中未找到 AI reply")
	}
	if reply.Content != "你好，这是AI回复" {
		t.Errorf("reply.Content = %q, want %q", reply.Content, "你好，这是AI回复")
	}
	if reply.ConversationID != "conv1" {
		t.Errorf("reply.ConversationID = %q, want conv1", reply.ConversationID)
	}
	if reply.ReplyToEventID != "evt-1" {
		t.Errorf("reply.ReplyToEventID = %q, want evt-1", reply.ReplyToEventID)
	}
	if reply.AccountID != "acc1" {
		t.Errorf("reply.AccountID = %q, want acc1", reply.AccountID)
	}
}
