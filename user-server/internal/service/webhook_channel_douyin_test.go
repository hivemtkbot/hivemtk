package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

func newDouyinGenericTestService(t *testing.T) (*WebhookService, context.Context) {
	t.Helper()
	db := testutil.NewTestDB(t,
		&model.MessageHub{}, &model.InboxConversation{}, &model.UnifiedMessage{},
		&model.WebhookEvent{}, &model.IntegrationAccount{},
	)
	return NewWebhookService(db), context.Background()
}

// TestDispatchDouyinGeneric_MsgIDStable 相同内容重推 → MsgID 稳定（幂等键生效）
func TestDispatchDouyinGeneric_MsgIDStable(t *testing.T) {
	svc, ctx := newDouyinGenericTestService(t)
	defer svc.Stop(ctx)

	p1 := &ParsedPayload{EventID: "evt-dy-g1", Sender: "user_001", Content: "你们产品多少钱"}
	hub1, _, err := svc.dispatchDouyin(ctx, "7", p1, []byte("{not-json"))
	if err != nil {
		t.Fatalf("dispatch1: %v", err)
	}
	if hub1 == nil {
		t.Fatal("expected hub from generic branch")
	}

	p2 := &ParsedPayload{EventID: "evt-dy-g2", Sender: "user_001", Content: "你们产品多少钱"}
	hub2, _, err := svc.dispatchDouyin(ctx, "7", p2, []byte("{not-json"))
	if err != nil {
		t.Fatalf("dispatch2: %v", err)
	}
	if hub2 == nil {
		t.Fatal("expected second hub")
	}

	if hub1.MsgID != hub2.MsgID {
		t.Errorf("W-5 未达成：相同内容两次投递 MsgID 不一致 %q vs %q（时间戳残留）", hub1.MsgID, hub2.MsgID)
	}
	if !strings.HasPrefix(hub1.MsgID, "dy_generic_") || !strings.Contains(hub1.MsgID, "mh:") {
		t.Errorf("MsgID 应为内容哈希形态 dy_generic_mh:*，实际 %q", hub1.MsgID)
	}

	wantSuffix := ContentHashMsgID("douyin", "", "你们产品多少钱")
	if hub1.MsgID != "dy_generic_"+wantSuffix {
		t.Errorf("MsgID expected dy_generic_%s, got %s", wantSuffix, hub1.MsgID)
	}
}

// TestDispatchDouyinGeneric_DifferentContentDifferentID 不同内容 → 不同 ID
func TestDispatchDouyinGeneric_DifferentContentDifferentID(t *testing.T) {
	svc, ctx := newDouyinGenericTestService(t)
	defer svc.Stop(ctx)

	p1 := &ParsedPayload{EventID: "evt-dy-c1", Sender: "user_002", Content: "内容甲"}
	h1, _, _ := svc.dispatchDouyin(ctx, "7", p1, []byte("{not-json"))
	p2 := &ParsedPayload{EventID: "evt-dy-c2", Sender: "user_002", Content: "内容乙"}
	h2, _, _ := svc.dispatchDouyin(ctx, "7", p2, []byte("{not-json"))

	if h1 == nil || h2 == nil {
		t.Fatal("expected both hubs")
	}
	if h1.MsgID == h2.MsgID {
		t.Errorf("不同内容不应共享 MsgID: %s", h1.MsgID)
	}
}

// TestDispatchDouyinGeneric_EmptyContentStable 空内容兜底文案也应产生稳定 ID
func TestDispatchDouyinGeneric_EmptyContentStable(t *testing.T) {
	svc, ctx := newDouyinGenericTestService(t)
	defer svc.Stop(ctx)

	p1 := &ParsedPayload{EventID: "evt-dy-e1", Sender: "user_003"}
	h1, _, _ := svc.dispatchDouyin(ctx, "7", p1, []byte("{not-json"))
	p2 := &ParsedPayload{EventID: "evt-dy-e2", Sender: "user_003"}
	h2, _, _ := svc.dispatchDouyin(ctx, "7", p2, []byte("{not-json"))

	if h1 == nil || h2 == nil {
		t.Fatal("expected both hubs")
	}
	if h1.Content != "[douyin generic event]" {
		t.Errorf("expected fallback content, got %q", h1.Content)
	}
	if h1.MsgID != h2.MsgID {
		t.Errorf("空内容兜底 MsgID 仍应稳定: %q vs %q", h1.MsgID, h2.MsgID)
	}
}

// TestDispatchDouyinStructured_MissingMessageIDStable 结构化分支缺 message_id → 相同内容重推 MsgID 稳定
func TestDispatchDouyinStructured_MissingMessageIDStable(t *testing.T) {
	svc, ctx := newDouyinGenericTestService(t)
	defer svc.Stop(ctx)

	raw := []byte(`{"event_type":"im.message.receive_v1","data":{"message":{"message_id":"","content":"在吗"},"from":{"user_id":"user_100"},"conversation":{"type":"chat"}}}`)
	p1 := &ParsedPayload{EventID: "evt-dy-s1", Sender: "user_100", Content: "在吗"}
	hub1, _, err := svc.dispatchDouyin(ctx, "7", p1, raw)
	if err != nil {
		t.Fatalf("dispatch1: %v", err)
	}
	if hub1 == nil {
		t.Fatal("expected hub from structured branch")
	}

	p2 := &ParsedPayload{EventID: "evt-dy-s2", Sender: "user_100", Content: "在吗"}
	hub2, _, _ := svc.dispatchDouyin(ctx, "7", p2, raw)
	if hub2 == nil {
		t.Fatal("expected second hub")
	}

	if hub1.MsgID != hub2.MsgID {
		t.Errorf("M3 未达成：结构化分支相同内容两次投递 MsgID 不一致 %q vs %q（时间戳残留）", hub1.MsgID, hub2.MsgID)
	}
	if !strings.HasPrefix(hub1.MsgID, "dy_mh:") {
		t.Errorf("MsgID 应为内容哈希形态 dy_mh:*，实际 %q", hub1.MsgID)
	}
	want := "dy_" + ContentHashMsgID("douyin", "", "在吗")
	if hub1.MsgID != want {
		t.Errorf("MsgID expected %s, got %s", want, hub1.MsgID)
	}
}

// TestDispatchDouyinStructured_ExplicitMessageIDUnchanged 有 message_id 时保持平台 ID 原样
func TestDispatchDouyinStructured_ExplicitMessageIDUnchanged(t *testing.T) {
	svc, ctx := newDouyinGenericTestService(t)
	defer svc.Stop(ctx)

	raw := []byte(`{"event_type":"im.message.receive_v1","data":{"message":{"message_id":"plat-777","content":"hi"},"from":{"user_id":"u1"}}}`)
	p := &ParsedPayload{EventID: "evt-dy-s3", Sender: "u1", Content: "hi"}
	hub, _, err := svc.dispatchDouyin(ctx, "7", p, raw)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if hub == nil || hub.MsgID != "dy_plat-777" {
		t.Errorf("平台 MessageID 应原样保留为 dy_plat-777，实际 %+v", hub)
	}
}
