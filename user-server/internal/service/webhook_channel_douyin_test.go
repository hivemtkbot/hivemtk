package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// ============================================================================
// M14 W-5：抖音 generic 分支 MsgID 稳定化回归测试
// 原实现用 UnixNano 时间戳生成 MsgID，同一事件重推/并发到达必然生成不同 ID，
// 幂等去重完全失效。修复后 generic 分支 MsgID 基于「渠道+内容」哈希，天然稳定。
// ============================================================================

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

	// 与 ContentHashMsgID 锚点一致：dy_generic_ + mh:<fnv32(douyin|content)>
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
