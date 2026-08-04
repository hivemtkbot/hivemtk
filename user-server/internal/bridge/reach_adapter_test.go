package bridge

import (
	"context"
	"testing"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/service"
)

func TestBridgeReachAdapter_OnlineDeliversToWS(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)

	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, hub, service.NewInboxIngressServiceWithDB(nil, nil))
	id, err := adapter.SendDouyin(context.Background(), "acc1", "conv1", "text", "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty message id")
	}
	select {
	case <-c.send:
		// ok
	default:
		t.Fatal("expected reply delivered over WebSocket")
	}
}

func TestBridgeReachAdapter_OfflineDelegatesToInner(t *testing.T) {
	hub := NewBridgeHub() // acc not registered -> offline

	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, hub, service.NewInboxIngressServiceWithDB(nil, nil))
	// 非桥接账号离线时，委托 IntegrationReachAdapter（抖音无 API 实现 -> 返回错误）
	_, err := adapter.SendDouyin(context.Background(), "accOff", "conv1", "text", "hello")
	if err == nil {
		t.Fatal("expected inner adapter error for offline non-bridge account")
	}
}

func TestBridgeReachAdapter_XHSAndTikTokOnline(t *testing.T) {
	hub := NewBridgeHub()
	hub.Register(newTestClient(ChannelXHSWeb, "x1"))
	hub.Register(newTestClient(ChannelTikTok, "t1"))

	adapter := NewBridgeReachAdapter(&tooluse.IntegrationReachAdapter{}, hub, service.NewInboxIngressServiceWithDB(nil, nil))
	if _, err := adapter.SendXHS(context.Background(), "x1", "c", "text", "hi"); err != nil {
		t.Fatalf("xhs deliver err: %v", err)
	}
	if _, err := adapter.SendTikTok(context.Background(), "t1", "c", "text", "hi"); err != nil {
		t.Fatalf("tiktok deliver err: %v", err)
	}
}
