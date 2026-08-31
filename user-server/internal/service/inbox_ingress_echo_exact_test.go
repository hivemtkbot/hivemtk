package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// T2 验收①：入站回显携带出站行同款平台消息 ID（wamid）→ 精确拦截。
func TestInboxIngress_SelfEcho_ExactPlatformMsgID_Blocked(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "whatsapp"
		account  = "wa-acc-1"
		conv     = "8613800000000"
		wamid    = "wamid.EXACT001"
	)
	if err := db.Create(&model.MessageHub{
		MsgID:          wamid,
		Platform:       platform,
		AccountID:      account,
		Direction:      "outbound",
		Status:         "delivered",
		MsgType:        "text",
		ConversationID: conv,
		SenderID:       account,
		Content:        "这是 AI 回复",
		SentAt:         time.Now().Add(-10 * time.Second),
	}).Error; err != nil {
		t.Fatalf("插入 outbound 失败: %v", err)
	}

	event := &model.MessageEvent{
		Channel:        platform,
		SenderID:       conv,
		Content:        "完全不同的内容——即使启发式不命中也应被精确 ID 拦截",
		EventID:        "evt-in-1",
		ConversationID: conv,
		Extra:          map[string]any{"account_id": account, "channel_msg_id": wamid},
	}
	decision, err := svc.interceptInbound(ctx, event)
	if err != nil {
		t.Fatalf("interceptInbound 失败: %v", err)
	}
	if decision == nil || !decision.Blocked || !decision.IsSelfEcho {
		t.Fatalf("平台 msg_id 精确回显应被拦截, got: %+v", decision)
	}
	if decision.Reason != "self-echo(platform msg_id exact match)" {
		t.Fatalf("应走精确匹配分支, got reason=%s", decision.Reason)
	}
}

// T2 验收②：同款 msg_id 但方向是 inbound（客户消息 ID 撞车防护）→ 不拦截。
func TestInboxIngress_ExactMatch_InboundRowNotBlocked(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressServiceWithDB(db, nil)
	ctx := context.Background()

	const (
		platform = "whatsapp"
		account  = "wa-acc-2"
		conv     = "8613900000000"
		msgID    = "wamid.INBOUNDONLY1"
	)
	if err := db.Create(&model.MessageHub{
		MsgID:          msgID,
		Platform:       platform,
		AccountID:      account,
		Direction:      "inbound", // 方向守卫：入站行不得用于回显判定
		MsgType:        "text",
		ConversationID: conv,
		SenderID:       conv,
		Content:        "客户消息",
		SentAt:         time.Now(),
	}).Error; err != nil {
		t.Fatalf("插入 inbound 失败: %v", err)
	}

	event := &model.MessageEvent{
		Channel:        platform,
		SenderID:       conv,
		Content:        "客户的新消息",
		EventID:        "evt-in-2",
		ConversationID: conv,
		Extra:          map[string]any{"account_id": account, "channel_msg_id": msgID},
	}
	decision, err := svc.interceptInbound(ctx, event)
	if err != nil {
		t.Fatalf("interceptInbound 失败: %v", err)
	}
	if decision != nil && decision.Blocked && decision.IsSelfEcho {
		t.Fatalf("inbound 行不应触发精确回显拦截, got: %+v", decision)
	}
}

// T2 验收③：UpdateMsgID 只回写 outbound 行；inbound 行与空 ID 不动。
func TestMessageHub_UpdateMsgID_DirectionGuard(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	repo := repository.NewMessageHubRepositoryWithDB(db)
	ctx := context.Background()

	out := &model.MessageHub{MsgID: "wa-out-1", Platform: "whatsapp", AccountID: "a1", Direction: "outbound", MsgType: "text", ConversationID: "c1", Content: "x", SentAt: time.Now()}
	in := &model.MessageHub{MsgID: "wamid.IN2", Platform: "whatsapp", AccountID: "a1", Direction: "inbound", MsgType: "text", ConversationID: "c1", Content: "y", SentAt: time.Now()}
	if err := db.Create(out).Error; err != nil {
		t.Fatalf("create out: %v", err)
	}
	if err := db.Create(in).Error; err != nil {
		t.Fatalf("create in: %v", err)
	}

	if err := repo.UpdateMsgID(ctx, out.ID, "wamid.OUT2"); err != nil {
		t.Fatalf("UpdateMsgID: %v", err)
	}
	gotOut, _ := repo.GetByID(ctx, out.ID)
	if gotOut.MsgID != "wamid.OUT2" {
		t.Fatalf("outbound 行应回写为 wamid.OUT2, got %s", gotOut.MsgID)
	}
	if err := repo.UpdateMsgID(ctx, in.ID, "wamid.SHOULDNOT"); err != nil {
		t.Fatalf("UpdateMsgID inbound: %v", err)
	}
	gotIn, _ := repo.GetByID(ctx, in.ID)
	if gotIn.MsgID != "wamid.IN2" {
		t.Fatalf("inbound 行不得被回写, got %s", gotIn.MsgID)
	}
	if err := repo.UpdateMsgID(ctx, out.ID, ""); err != nil {
		t.Fatalf("empty id should be no-op: %v", err)
	}
}

// T2 验收④：channelMsgIDOf 过滤服务端自造占位 ID（不可能与平台 ID 匹配，省一次查询）。
func TestChannelMsgIDOf_FiltersPlaceholder(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"wamid.REAL123", "wamid.REAL123"},
		{"wa-out-1690000000", ""},
		{"tg-out-1690000000", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := channelMsgIDOf(&model.MessageEvent{Extra: map[string]any{"channel_msg_id": c.raw}})
		if got != c.want {
			t.Fatalf("channel_msg_id=%q want %q got %q", c.raw, c.want, got)
		}
	}
	if channelMsgIDOf(nil) != "" || channelMsgIDOf(&model.MessageEvent{}) != "" {
		t.Fatalf("nil/empty extra should return empty")
	}
}
