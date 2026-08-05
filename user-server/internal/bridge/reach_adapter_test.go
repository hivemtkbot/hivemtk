package bridge

import (
	"context"
	"testing"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/service"
)

// TestBridgeReachAdapter_OnlineDeliversToBuffer 验证 HTTP-only 模式下，桥接渠道（抖音）的 AI 回复
// 经 deliverHTTP → httpReplyBuffer，等下次同 (channel, account, conversation) 的
// /api/bridge/ingest 长轮询拉到。
//
// 历史：WS 模式下靠 c.send 通道收 reply（被 hub.Deliver 写入）；HTTP 模式无 WS 长连接，
// reply 直接入 in-memory buffer（256 容量 FIFO）。本测试覆盖"无 WebSocket 也能投递"的关键路径。
func TestBridgeReachAdapter_OnlineDeliversToBuffer(t *testing.T) {
	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, service.NewInboxIngressServiceWithDB(nil, nil))
	id, err := adapter.SendDouyin(context.Background(), "acc1", "conv1", "text", "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty message id")
	}
	// 验证 reply 已入 buffer
	reply := adapter.httpReplyBuffer.Pull(ChannelDouyinWeb, "conv1", "")
	if reply == nil {
		t.Fatal("expected reply in httpReplyBuffer, got nil")
	}
	if reply.Content != "hello" {
		t.Errorf("reply.Content = %q, want %q", reply.Content, "hello")
	}
	if reply.Channel != ChannelDouyinWeb {
		t.Errorf("reply.Channel = %q, want %q", reply.Channel, ChannelDouyinWeb)
	}
	if reply.AccountID != "acc1" {
		t.Errorf("reply.AccountID = %q, want %q", reply.AccountID, "acc1")
	}
}

// TestBridgeReachAdapter_OfflineStillDeliversToBuffer 验证 HTTP-only 模式下无 WebSocket 连接时
// 也能把 reply 入 buffer（HTTP 模式无"离线"概念，buffer 始终可达）。
func TestBridgeReachAdapter_OfflineStillDeliversToBuffer(t *testing.T) {
	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, service.NewInboxIngressServiceWithDB(nil, nil))
	_, err := adapter.SendDouyin(context.Background(), "accOff", "conv1", "text", "hello")
	if err != nil {
		t.Fatalf("HTTP-only 模式不依赖扩展在线，期望 nil err，实际 %v", err)
	}
	reply := adapter.httpReplyBuffer.Pull(ChannelDouyinWeb, "conv1", "")
	if reply == nil || reply.Content != "hello" {
		t.Errorf("buffer 中应已存有 reply: %+v", reply)
	}
}

// TestBridgeReachAdapter_XHSAndTikTokDeliversToBuffer 验证小红书和 TikTok 渠道的 reply 走各自 channel 的 buffer。
func TestBridgeReachAdapter_XHSAndTikTokDeliversToBuffer(t *testing.T) {
	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, service.NewInboxIngressServiceWithDB(nil, nil))
	if _, err := adapter.SendXHS(context.Background(), "x1", "c", "text", "hi-xhs"); err != nil {
		t.Fatalf("xhs deliver err: %v", err)
	}
	if _, err := adapter.SendTikTok(context.Background(), "t1", "c", "text", "hi-tiktok"); err != nil {
		t.Fatalf("tiktok deliver err: %v", err)
	}
	if reply := adapter.httpReplyBuffer.Pull(ChannelXHSWeb, "c", ""); reply == nil || reply.Content != "hi-xhs" {
		t.Errorf("XHS buffer 应有 reply，实际 %+v", reply)
	}
	if reply := adapter.httpReplyBuffer.Pull(ChannelTikTok, "c", ""); reply == nil || reply.Content != "hi-tiktok" {
		t.Errorf("TikTok buffer 应有 reply，实际 %+v", reply)
	}
}
