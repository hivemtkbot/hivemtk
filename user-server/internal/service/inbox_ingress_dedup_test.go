package service

import (
	"context"
	"fmt"
	"testing"

	"marketing/internal/cache"
	"marketing/internal/model"
)

// TestInboxIngress_ContentDedup_Scenarios 验证 2026-08-05 架构重构后的内容 hash 去重逻辑。
//
// 架构变更背景：
//   - Bridge 端不再做内容指纹去重，所有消息都上报到统一收信中心
//   - 服务端在 HandleIngressMessage 中用 内容 hash (SHA-256 前 16 字符) +
//     Redis SetNX 做去重，TTL 5 分钟
//   - 重复内容 → 直接丢弃，不落库，不触发 AI（QueuedForAI=false）
//   - 非重复内容 → 落库 + 触发 AI（QueuedForAI=true）
//
// 用户诉求："重复则丢弃，第一条入库"
//   - 第 1 条消息 SetNX 成功 → 落库 + 触发 AI
//   - 第 2..N 条相同内容 SetNX 失败 → 直接 return，不调用 persistMessage
//
// 本测试构造模拟消息验证以下场景：
//   1. 相同内容 + 相同会话 → 第二条被去重（丢弃，不触发 AI）
//   2. 相同内容 + 不同会话 → 都触发 AI（非重复）
//   3. 不同内容 + 相同会话 → 都触发 AI（非重复）
//   4. 不同渠道的相同内容 → 都触发 AI（非重复，渠道隔离）
//   5. 不同账号的相同内容 → 都触发 AI（非重复，账号隔离）
func TestInboxIngress_ContentDedup_Scenarios(t *testing.T) {
	ctx := context.Background()

	c := cache.NewMemoryCache()
	svc := NewInboxIngressServiceWithDB(nil, c) // hubRepo=nil 时 persistMessage 直接返回 nil

	trigger := &fakeAITrigger{}
	svc.SetAITrigger(trigger)

	// 辅助函数：构造一条入站消息
	makeEvent := func(channel, accountID, convID, content string) *model.MessageEvent {
		return &model.MessageEvent{
			Channel:        channel,
			SenderID:       "customer-" + convID,
			Content:        content,
			EventID:        fmt.Sprintf("evt-%s-%s-%d", channel, convID, trigger.called),
			ConversationID: convID,
			MsgType:        model.MsgTypeText,
			Extra:          map[string]interface{}{"account_id": accountID},
		}
	}

	// ========== 场景 1：相同内容 + 相同会话 → 第二条被去重（丢弃，不落库） ==========
	t.Run("场景1_相同内容相同会话_第二条去重丢弃", func(t *testing.T) {
		trigger.called = 0
		convID := "conv-dedup-1"
		content := "你好，请问这个商品还在吗？"

		// 第一条：应触发 AI
		ev1 := makeEvent(model.ChannelXHS, "xhs-acct-1", convID, content)
		r1, err := svc.HandleIngressMessage(ctx, ev1)
		if err != nil {
			t.Fatalf("第一条 handle: %v", err)
		}
		if !r1.QueuedForAI {
			t.Fatalf("第一条应触发 AI（QueuedForAI=true），实际: %+v", r1)
		}
		if trigger.called != 1 {
			t.Fatalf("第一条后 AI 应被调用 1 次，实际: %d", trigger.called)
		}
		t.Logf("✅ 第一条消息触发 AI: content=%q conv=%s", content[:10], convID)

		// 第二条（相同内容+相同会话）：应被去重丢弃，不落库，不触发 AI
		ev2 := makeEvent(model.ChannelXHS, "xhs-acct-1", convID, content)
		ev2.EventID = "evt-dup-2" // 不同的 eventID，但内容相同
		r2, err := svc.HandleIngressMessage(ctx, ev2)
		if err != nil {
			t.Fatalf("第二条 handle: %v", err)
		}
		if r2.QueuedForAI {
			t.Fatalf("第二条应被去重丢弃（QueuedForAI=false），实际: %+v", r2)
		}
		if trigger.called != 1 {
			t.Fatalf("第二条后 AI 仍应只被调用 1 次（去重不触发），实际: %d", trigger.called)
		}
		if r2.Reason == "" {
			t.Fatal("去重时 Reason 不应为空")
		}
		if r2.Reason != "duplicate content within 5min; dropped" {
			t.Fatalf("去重 Reason 应为 'duplicate content within 5min; dropped'，实际: %q", r2.Reason)
		}
		t.Logf("✅ 第二条消息被去重丢弃: reason=%q AI 调用次数仍为 %d", r2.Reason, trigger.called)
	})

	// ========== 场景 2：相同内容 + 不同会话 → 都触发 AI ==========
	t.Run("场景2_相同内容不同会话_都触发AI", func(t *testing.T) {
		trigger.called = 0
		content := "这个多少钱？"

		// 会话 A
		ev1 := makeEvent(model.ChannelXHS, "xhs-acct-1", "conv-A", content)
		r1, _ := svc.HandleIngressMessage(ctx, ev1)
		if !r1.QueuedForAI {
			t.Fatalf("会话 A 应触发 AI，实际: %+v", r1)
		}
		t.Logf("✅ 会话 A 触发 AI: content=%q", content)

		// 会话 B（相同内容，不同会话）
		ev2 := makeEvent(model.ChannelXHS, "xhs-acct-1", "conv-B", content)
		r2, _ := svc.HandleIngressMessage(ctx, ev2)
		if !r2.QueuedForAI {
			t.Fatalf("会话 B 应触发 AI（不同会话非重复），实际: %+v", r2)
		}
		if trigger.called != 2 {
			t.Fatalf("两个不同会话应触发 AI 2 次，实际: %d", trigger.called)
		}
		t.Logf("✅ 会话 B 触发 AI: content=%q（相同内容但不同会话，非重复）", content)
	})

	// ========== 场景 3：不同内容 + 相同会话 → 都触发 AI ==========
	t.Run("场景3_不同内容相同会话_都触发AI", func(t *testing.T) {
		trigger.called = 0
		convID := "conv-diff-content"

		// 第一条
		ev1 := makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "第一条消息")
		r1, _ := svc.HandleIngressMessage(ctx, ev1)
		if !r1.QueuedForAI {
			t.Fatalf("第一条应触发 AI，实际: %+v", r1)
		}
		t.Logf("✅ 第一条触发 AI: content=%q", "第一条消息")

		// 第二条（不同内容，相同会话）
		ev2 := makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "第二条消息")
		r2, _ := svc.HandleIngressMessage(ctx, ev2)
		if !r2.QueuedForAI {
			t.Fatalf("第二条应触发 AI（不同内容非重复），实际: %+v", r2)
		}
		if trigger.called != 2 {
			t.Fatalf("两条不同内容应触发 AI 2 次，实际: %d", trigger.called)
		}
		t.Logf("✅ 第二条触发 AI: content=%q（不同内容，非重复）", "第二条消息")
	})

	// ========== 场景 4：不同渠道的相同内容 → 都触发 AI ==========
	t.Run("场景4_不同渠道相同内容_都触发AI", func(t *testing.T) {
		trigger.called = 0
		content := "请问可以包邮吗？"
		convID := "conv-cross-channel"

		// 小红书
		ev1 := makeEvent(model.ChannelXHS, "xhs-acct-1", convID, content)
		r1, _ := svc.HandleIngressMessage(ctx, ev1)
		if !r1.QueuedForAI {
			t.Fatalf("小红书应触发 AI，实际: %+v", r1)
		}
		t.Logf("✅ 小红书触发 AI: content=%q", content)

		// 抖音（相同内容，相同会话 ID，但不同渠道）
		ev2 := makeEvent(model.ChannelDouyin, "dy-acct-1", convID, content)
		r2, _ := svc.HandleIngressMessage(ctx, ev2)
		if !r2.QueuedForAI {
			t.Fatalf("抖音应触发 AI（不同渠道非重复），实际: %+v", r2)
		}
		if trigger.called != 2 {
			t.Fatalf("两个渠道应触发 AI 2 次，实际: %d", trigger.called)
		}
		t.Logf("✅ 抖音触发 AI: content=%q（相同内容但不同渠道，非重复）", content)
	})

	// ========== 场景 5：不同账号的相同内容 → 都触发 AI ==========
	t.Run("场景5_不同账号相同内容_都触发AI", func(t *testing.T) {
		trigger.called = 0
		content := "什么时候发货？"
		convID := "conv-cross-account"

		// 账号 A
		ev1 := makeEvent(model.ChannelXHS, "xhs-acct-A", convID, content)
		r1, _ := svc.HandleIngressMessage(ctx, ev1)
		if !r1.QueuedForAI {
			t.Fatalf("账号 A 应触发 AI，实际: %+v", r1)
		}
		t.Logf("✅ 账号 A 触发 AI: content=%q", content)

		// 账号 B（相同内容，相同渠道，相同会话 ID，但不同账号）
		ev2 := makeEvent(model.ChannelXHS, "xhs-acct-B", convID, content)
		r2, _ := svc.HandleIngressMessage(ctx, ev2)
		if !r2.QueuedForAI {
			t.Fatalf("账号 B 应触发 AI（不同账号非重复），实际: %+v", r2)
		}
		if trigger.called != 2 {
			t.Fatalf("两个账号应触发 AI 2 次，实际: %d", trigger.called)
		}
		t.Logf("✅ 账号 B 触发 AI: content=%q（相同内容但不同账号，非重复）", content)
	})

	// ========== 场景 6：内容 hash 计算验证 ==========
	t.Run("场景6_内容hash计算验证", func(t *testing.T) {
		// 相同内容应产生相同 hash
		h1 := contentHashOf("你好")
		h2 := contentHashOf("你好")
		if h1 != h2 {
			t.Fatalf("相同内容应产生相同 hash: %q vs %q", h1, h2)
		}
		if h1 == "" {
			t.Fatal("hash 不应为空")
		}
		t.Logf("✅ 相同内容 hash 一致: %s", h1)

		// 不同内容应产生不同 hash
		h3 := contentHashOf("你好啊")
		if h1 == h3 {
			t.Fatalf("不同内容应产生不同 hash: 都=%q", h1)
		}
		t.Logf("✅ 不同内容 hash 不同: %q vs %q", h1, h3)

		// 空内容应返回空 hash
		h4 := contentHashOf("")
		if h4 != "" {
			t.Fatalf("空内容应返回空 hash，实际: %q", h4)
		}
		t.Logf("✅ 空内容返回空 hash")
	})
}
