package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
)

// fakeAITrigger 记录是否被调用，用于验证“新消息触发 AI / 历史消息不触发 AI”语义
type fakeAITrigger struct {
	called     int
	lastChannel string
	lastAccount string
	lastConv    string
	lastCust    string
	lastContent string
	lastEventID string
}

func (f *fakeAITrigger) TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string) {
	f.called++
	f.lastChannel = channel
	f.lastAccount = accountID
	f.lastConv = conversationID
	f.lastCust = customerID
	f.lastContent = content
	f.lastEventID = eventID
}

// TestInbox_NewMessageTriggersAI_HistoryDoesNot 验证两种实际需求的核心语义：
//  1. 实时新消息（inbound）必须触发 AI 客服
//  2. 历史消息（history）必须仅落库、不触发 AI
//
// 使用 nil DB（hubRepo==nil 时持久化 no-op），无需真实 Postgres 即可密封验证。
func TestInbox_NewMessageTriggersAI_HistoryDoesNot(t *testing.T) {
	svc := NewInboxIngressServiceWithDB(nil, nil)
	tr := &fakeAITrigger{}
	svc.SetAITrigger(tr)

	// 1) 实时新消息（inbound）必须触发 AI
	inEv := &model.MessageEvent{
		EventID:        "e1",
		Channel:        "douyin_web",
		ConversationID: "conv1",
		SenderID:       "cust1",
		Content:        "你好，我要咨询",
		Extra:          map[string]any{"account_id": "acc1"},
		Timestamp:      time.Now(),
	}
	if _, err := svc.HandleIngressMessage(context.Background(), inEv); err != nil {
		t.Fatalf("HandleIngressMessage(inbound) error: %v", err)
	}
	if tr.called != 1 {
		t.Fatalf("期望新消息触发 AI 1 次，实际 %d", tr.called)
	}
	if tr.lastContent != "你好，我要咨询" || tr.lastConv != "conv1" || tr.lastAccount != "acc1" || tr.lastEventID != "e1" {
		t.Fatalf("AI 触发参数错误: %+v", tr)
	}

	// 2) 历史消息（history）必须不触发 AI，仅落库
	histEv := &model.MessageEvent{
		EventID:        "e2",
		Channel:        "douyin_web",
		ConversationID: "conv1",
		SenderID:       "cust1",
		Content:        "历史记录一条",
		Extra:          map[string]any{"account_id": "acc1"},
		Timestamp:      time.Now(),
	}
	if err := svc.PersistBridgeHistory(context.Background(), histEv, "inbound"); err != nil {
		t.Fatalf("PersistBridgeHistory error: %v", err)
	}
	if tr.called != 1 {
		t.Fatalf("历史消息不应触发 AI；期望调用次数仍为 1，实际 %d", tr.called)
	}
}
