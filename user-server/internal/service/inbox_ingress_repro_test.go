// 复现测试：小红书渠道特定账号消息入站后 AI 未触发
//
// 数据库观察到的现象：
//   - inbox_conversations.id=2268, platform=xhs_web, account=69c730300000000034018cb2
//   - 5 条 inbound 消息入库 message_hub (id 2630, 2644-2647)
//   - customer_sessions/session_messages/ai_suggestions 全部 0 条 → AI 编排器未运行
//
// 对比同账号 working 的雪大王/三叶草 6/1 条 inbound 都有 customer_sessions + ai_suggestions。
// 复现思路：直接调用 InboxIngressService.HandleIngressMessage，验证 AI trigger 是否被调用。

package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
)

func TestReproduce_XhsWeb_NoAITrigger(t *testing.T) {
	svc := NewInboxIngressServiceWithDB(nil, nil)
	tr := &fakeAITrigger{}
	svc.SetAITrigger(tr)

	// 复现 msg_hub.id=2630 的入参（异常 msg_id：含中文冒号分隔）
	ev := &model.MessageEvent{
		EventID:        "69c62722000000003203b0fe:你好:2",
		Channel:        "xiaohongshu",
		ConversationID: "69c62722000000003203b0fe",
		SenderID:       "69c62722000000003203b0fe",
		SenderName:     "小红薯69C69EDE",
		Content:        "你好",
		MsgType:        "text",
		Timestamp:      time.Now(),
		Extra:          map[string]any{
			"account_id":  "69c730300000000034018cb2",
			"bridge":      true,
			"sender_type": "customer",
		},
	}
	if _, err := svc.HandleIngressMessage(context.Background(), ev); err != nil {
		t.Fatalf("HandleIngressMessage error: %v", err)
	}
	if tr.called != 1 {
		t.Fatalf("期望 AI 触发 1 次，实际 %d。问题：xhs_web 渠道特定消息未触发 AI 编排器", tr.called)
	}
	t.Logf("✅ AI trigger 被正确调用: channel=%s accountID=%s", tr.lastChannel, tr.lastAccount)
}
