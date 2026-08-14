package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// 2026-08-15 P1-4 修复：sendOutbound 在桥接 channel（xiaohongshu/douyin/tiktok/xianyu/
// kuaishou）落库 message_hub 时，必须填充 Extra 场景元数据（dm_target/scenario/
// triggered_by/agent_id/intent/confidence/handler_type）。
//
// 本测试直接调用 sendOutbound，不依赖真实出站 HTTP（桥接出站走 message_hub 落库后由
// bridge 端 downlink 拉取，不在 sendOutbound 阶段做 HTTP 投递——见 webhook_outbound.go
// 注释）。验证：
//   1) Extra 必含 dm_target=conv、scenario=auto_reply、triggered_by=ai_dispatch
//   2) ctx 注入 HandleResult 后,Extra 补 confidence/intent/handler_type
//   3) ctx 注入 agentID 后,Extra 补 agent_id
//   4) 占位账号（<channel>-unknown）触发 undeliverable → status=failed、scenario=undeliverable

func TestSendOutbound_BridgeChannel_ExtraMetadata_P1_4(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{}, &model.InboxConversation{})
	svc := NewWebhookService(db)
	defer svc.Stop(context.Background())

	const (
		platform = "xiaohongshu"
		account  = "acct-p14-real"
		conv     = "conv-p14-1"
	)
	hub := &model.MessageHub{
		MsgID:          "mh:in-p14-1",
		Platform:       platform,
		AccountID:      account,
		Direction:      "inbound",
		MsgType:        "text",
		SenderID:       "sender-1",
		SenderName:     "客户甲",
		Content:        "原始消息",
		ConversationID: conv,
		SentAt:         time.Now(),
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("pre-create inbound failed: %v", err)
	}
	p := &ParsedPayload{
		EventID: "evt-p14-1",
		Sender:  "sender-1",
		Content: "原始消息",
		ChatID:  conv,
	}

	// 模拟 runAIGeneration 注入的 ctx
	result := &HandleResult{
		HandlerType: model.HandlerTypeAI,
		AIReplied:   true,
		Confidence:  0.86,
		Reply:       "您好，关于代码二开请查看 Gitee AGPL-3.0 协议。",
		SalesResponse: &SalesResponse{
			Intent: &dto.RecognizeResult{IntentType: "inquiry"},
		},
	}
	ctx := AgentIDToContext(HandleResultToContext(context.Background(), result), "sales-prod-v1")

	svc.sendOutbound(ctx, ChannelXiaohongshu, account, p, result.Reply, hub, nil)

	// 校验落库的 message_hub.Extra
	var out model.MessageHub
	err := db.Where("direction = ? AND conversation_id = ?", "outbound", conv).
		Order("id DESC").First(&out).Error
	if err != nil {
		t.Fatalf("query outbound failed: %v", err)
	}

	if out.Extra == nil {
		t.Fatalf("outbound.Extra must not be nil, got nil (P1-4 修复目标)")
	}
	if v, _ := out.Extra["dm_target"].(string); v != "conv" {
		t.Errorf("Extra.dm_target expected 'conv', got %q", v)
	}
	if v, _ := out.Extra["scenario"].(string); v != "auto_reply" {
		t.Errorf("Extra.scenario expected 'auto_reply', got %q", v)
	}
	if v, _ := out.Extra["triggered_by"].(string); v != "ai_dispatch" {
		t.Errorf("Extra.triggered_by expected 'ai_dispatch', got %q", v)
	}
	if v, _ := out.Extra["agent_id"].(string); v != "sales-prod-v1" {
		t.Errorf("Extra.agent_id expected 'sales-prod-v1' (from ctx), got %q", v)
	}
	if v, _ := out.Extra["confidence"].(float64); v != 0.86 {
		t.Errorf("Extra.confidence expected 0.86 (from HandleResult), got %v", v)
	}
	if v, _ := out.Extra["intent"].(string); v != "inquiry" {
		t.Errorf("Extra.intent expected 'inquiry' (from SalesResponse.Intent), got %q", v)
	}
	if v, _ := out.Extra["handler_type"].(string); v != string(model.HandlerTypeAI) {
		t.Errorf("Extra.handler_type expected %q, got %q", model.HandlerTypeAI, v)
	}
}

// 占位账号（前端 getAccountId DOM 兜底失败 → account_id=*-unknown）必须被识别为
// undeliverable，Extra 补 undeliverable_reason + scenario=undeliverable，message_hub
// 直接落 status=failed，避免污染 pending 出库队列。
func TestSendOutbound_PlaceholderAccount_Undeliverable_P1_4(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{}, &model.InboxConversation{})
	svc := NewWebhookService(db)
	defer svc.Stop(context.Background())

	const (
		platform = "xiaohongshu"
		conv     = "conv-p14-undel"
	)
	hub := &model.MessageHub{
		MsgID:          "mh:in-p14-undel",
		Platform:       platform,
		AccountID:      "xiaohongshu-unknown",
		Direction:      "inbound",
		MsgType:        "text",
		SenderID:       "sender-undel",
		Content:        "原始消息",
		ConversationID: conv,
		SentAt:         time.Now(),
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("pre-create inbound failed: %v", err)
	}
	p := &ParsedPayload{EventID: "evt-p14-undel", Sender: "sender-undel", Content: "原始消息", ChatID: conv}

	result := &HandleResult{
		HandlerType: model.HandlerTypeAI,
		AIReplied:   true,
		Confidence:  0.7,
		Reply:       "AI 回复（但账号不可达）",
	}
	ctx := HandleResultToContext(context.Background(), result)
	svc.sendOutbound(ctx, ChannelXiaohongshu, "xiaohongshu-unknown", p, result.Reply, hub, nil)

	var out model.MessageHub
	err := db.Where("direction = ? AND conversation_id = ?", "outbound", conv).
		Order("id DESC").First(&out).Error
	if err != nil {
		t.Fatalf("query outbound failed: %v", err)
	}
	if out.Status != "failed" {
		t.Errorf("placeholder account should mark status=failed, got %q", out.Status)
	}
	if v, _ := out.Extra["scenario"].(string); v != "undeliverable" {
		t.Errorf("Extra.scenario expected 'undeliverable', got %q", v)
	}
	if v, _ := out.Extra["undeliverable_reason"].(string); !strings.Contains(v, "placeholder") {
		t.Errorf("Extra.undeliverable_reason expected to contain 'placeholder', got %q", v)
	}
}

// ctx 没有 HandleResult（异常路径,如 runAIGeneration panic 后兜底）时,Extra 仍要
// 有稳定的 dm_target/scenario/triggered_by，但 confidence/intent/handler_type 缺省
// 显式填空为 "unknown"（不允许 nil，否则 trace_learning group by key 漂移）。
func TestSendOutbound_NoCtxResult_StillFillsStableFields_P1_4(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{}, &model.InboxConversation{})
	svc := NewWebhookService(db)
	defer svc.Stop(context.Background())

	const (
		platform = "douyin"
		account  = "acct-p14-noctx"
		conv     = "conv-p14-noctx-1"
	)
	hub := &model.MessageHub{
		MsgID:          "mh:in-p14-noctx",
		Platform:       platform,
		AccountID:      account,
		Direction:      "inbound",
		MsgType:        "text",
		SenderID:       "sender-1",
		Content:        "原始消息",
		ConversationID: conv,
		SentAt:         time.Now(),
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("pre-create inbound failed: %v", err)
	}
	p := &ParsedPayload{EventID: "evt-p14-noctx", Sender: "sender-1", Content: "原始消息", ChatID: conv}

	// ctx 不注入 HandleResult,直接 background
	svc.sendOutbound(context.Background(), ChannelDouyin, account, p, "AI 回复（无 ctx）", hub, nil)

	var out model.MessageHub
	err := db.Where("direction = ? AND conversation_id = ?", "outbound", conv).
		Order("id DESC").First(&out).Error
	if err != nil {
		t.Fatalf("query outbound failed: %v", err)
	}
	if out.Extra == nil {
		t.Fatalf("outbound.Extra must not be nil even without ctx HandleResult")
	}
	// 稳定字段必须有
	for _, key := range []string{"dm_target", "scenario", "triggered_by"} {
		if _, ok := out.Extra[key]; !ok {
			t.Errorf("Extra.%s must exist even without ctx (stable field), got nil", key)
		}
	}
	// agent_id 未注入时显式填空 "unknown"（不允许空串/缺失,否则 group by 漂移）
	if v, _ := out.Extra["agent_id"].(string); v != "unknown" {
		t.Errorf("Extra.agent_id expected 'unknown' (no ctx), got %q", v)
	}
}
