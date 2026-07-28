package bridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/service"
)

// TestBridgeOutbound_DeliversViaWebSocket 验证桥接出站接线：
// WebhookService.sendOutbound 在桥接渠道(douyin_web)下，经 service.DeliverBridgeOutbound
// 注册的回调把 AI 回复经 BridgeHub 通过 WebSocket 投递到 Chrome 扩展。
// 无需真实 Postgres；直接以 hub 内存客户端接收帧。
func TestBridgeOutbound_DeliversViaWebSocket(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)

	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, hub, service.NewInboxIngressServiceWithDB(nil, nil))
	SetBridgeReachAdapter(adapter)

	if err := service.DeliverBridgeOutbound(context.Background(), ChannelDouyinWeb, "acc1", "conv1", "text", "你好，这是AI回复", "evt-1"); err != nil {
		t.Fatalf("DeliverBridgeOutbound error: %v", err)
	}

	select {
	case raw := <-c.send:
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("unmarshal frame: %v", err)
		}
		if frame["type"] != FrameOutboundReply {
			t.Fatalf("期望 outbound_reply 帧，实际 %v", frame["type"])
		}
		reply, ok := frame["reply"].(map[string]any)
		if !ok {
			t.Fatalf("缺少 reply 字段: %v", frame)
		}
		if reply["content"] != "你好，这是AI回复" {
			t.Fatalf("回复内容错误: %v", reply["content"])
		}
		if reply["conversation_id"] != "conv1" {
			t.Fatalf("会话ID错误: %v", reply["conversation_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("超时：未收到 WebSocket 下发帧")
	}
}
