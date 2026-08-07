package service

import (
	"context"
	"fmt"
	"testing"

	"marketing/internal/cache"
	"marketing/internal/model"
)

// TestInboxIngress_BatchMerge_Scenarios 验证 2026-08-05 重构后的 batch 合并机制。
//
// 架构变更（用户科学方案）：
//   - 入库判断：按 msg_id 查 DB（存在则幂等跳过），逐条检查
//   - 回复判断：查 DB 最后一条消息方向（outbound→不回复，inbound→回复）
//   - AI 合并：batch 内同会话多条 inbound 消息合并一次 AI 回复
//   - 时序处理：timestamp 与锚点比较，历史堆积消息保留原 timestamp
//
// 本测试用 nil DB（hubRepo==nil），验证 batch 内 AI 合并行为：
//   1. 相同会话多条消息 → batch 末尾合并一次 AI 回复
//   2. 不同会话消息 → 各自触发 AI（按 conversation 分组）
//   3. 不同渠道相同会话 → 同 conversation 分组，合并触发
//   4. 不同账号不同会话 → 各自触发 AI
//   5. 内容 hash 函数仍存在（兼容性验证）
func TestInboxIngress_BatchMerge_Scenarios(t *testing.T) {
	ctx := context.Background()

	c := cache.NewMemoryCache()
	svc := NewInboxIngressServiceWithDB(nil, c) // hubRepo=nil 时 persistMessage 直接返回 nil

	trigger := &fakeAITrigger{}
	svc.SetAITrigger(trigger)

	// 辅助函数：构造一条入站消息
	makeEvent := func(channel, accountID, convID, content string, seq int) *model.MessageEvent {
		return &model.MessageEvent{
			Channel:        channel,
			SenderID:       "customer-" + convID,
			Content:        content,
			EventID:        fmt.Sprintf("evt-%s-%s-%d", channel, convID, seq),
			ConversationID: convID,
			MsgType:        model.MsgTypeText,
			Extra:          map[string]interface{}{"account_id": accountID},
		}
	}

	// ========== 场景 1：相同会话多条消息 → batch 末尾合并一次 AI 回复 ==========
	t.Run("场景1_相同会话多条消息_合并一次AI回复", func(t *testing.T) {
		trigger.called = 0
		convID := "conv-batch-1"

		// 一次 batch 上报 3 条同会话消息
		events := []*model.MessageEvent{
			makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "你好", 0),
			makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "请问这个商品还在吗？", 1),
			makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "多少钱？", 2),
		}

		result, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		// 关键：3 条消息合并一次 AI 回复
		if trigger.called != 1 {
			t.Fatalf("3 条同会话消息应合并触发 1 次 AI，实际: %d", trigger.called)
		}
		if !result.TriggeredAI {
			t.Fatal("TriggeredAI 应为 true")
		}
		// 验证合并内容包含 3 条消息
		expectedMerged := "你好\n请问这个商品还在吗？\n多少钱？"
		if trigger.lastContent != expectedMerged {
			t.Fatalf("合并内容应为 %q，实际: %q", expectedMerged, trigger.lastContent)
		}
		// 验证每条消息都被接受
		if len(result.PerEvent) != 3 {
			t.Fatalf("应返回 3 个 PerEvent，实际: %d", len(result.PerEvent))
		}
		for i, r := range result.PerEvent {
			if !r.Accepted {
				t.Fatalf("第 %d 条消息应被接受，实际: %+v", i, r)
			}
		}
		t.Logf("✅ 3 条同会话消息合并触发 1 次 AI: merged_content=%q", trigger.lastContent)
	})

	// ========== 场景 2：不同会话消息 → 各自触发 AI ==========
	t.Run("场景2_不同会话_各自触发AI", func(t *testing.T) {
		trigger.called = 0

		events := []*model.MessageEvent{
			makeEvent(model.ChannelXHS, "xhs-acct-1", "conv-A", "这个多少钱？", 0),
			makeEvent(model.ChannelXHS, "xhs-acct-1", "conv-B", "那个多少钱？", 1),
		}

		result, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		// 不同会话各自触发 AI
		if trigger.called != 2 {
			t.Fatalf("2 个不同会话应触发 2 次 AI，实际: %d", trigger.called)
		}
		if !result.TriggeredAI {
			t.Fatal("TriggeredAI 应为 true")
		}
		t.Logf("✅ 2 个不同会话各自触发 AI: 调用次数=%d", trigger.called)
	})

	// ========== 场景 3：不同渠道相同会话 → 同 conversation 分组合并 ==========
	t.Run("场景3_不同渠道相同会话_合并触发", func(t *testing.T) {
		trigger.called = 0
		convID := "conv-cross-channel"

		events := []*model.MessageEvent{
			makeEvent(model.ChannelXHS, "xhs-acct-1", convID, "小红书消息", 0),
			makeEvent(model.ChannelDouyin, "dy-acct-1", convID, "抖音消息", 1),
		}

		_, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		// 相同 conversation_id，不同渠道 → 合并一次 AI
		if trigger.called != 1 {
			t.Fatalf("相同会话不同渠道应合并触发 1 次 AI，实际: %d", trigger.called)
		}
		expectedMerged := "小红书消息\n抖音消息"
		if trigger.lastContent != expectedMerged {
			t.Fatalf("合并内容应为 %q，实际: %q", expectedMerged, trigger.lastContent)
		}
		t.Logf("✅ 不同渠道相同会话合并触发: merged=%q", trigger.lastContent)
	})

	// ========== 场景 4：不同账号不同会话 → 各自触发 AI ==========
	t.Run("场景4_不同账号不同会话_各自触发AI", func(t *testing.T) {
		trigger.called = 0

		events := []*model.MessageEvent{
			{
				Channel:        model.ChannelXHS,
				SenderID:       "customer-accountA",
				Content:        "账号A消息",
				EventID:        "evt-xhs-acctA-0",
				ConversationID: "conv-cross-account-A",
				MsgType:        model.MsgTypeText,
				Extra:          map[string]interface{}{"account_id": "xhs-acct-A"},
			},
			{
				Channel:        model.ChannelXHS,
				SenderID:       "customer-accountB",
				Content:        "账号B消息",
				EventID:        "evt-xhs-acctB-1",
				ConversationID: "conv-cross-account-B",
				MsgType:        model.MsgTypeText,
				Extra:          map[string]interface{}{"account_id": "xhs-acct-B"},
			},
		}

		result, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		if trigger.called != 2 {
			t.Fatalf("2 个不同账号不同会话应触发 2 次 AI，实际: %d", trigger.called)
		}
		if !result.TriggeredAI {
			t.Fatal("TriggeredAI 应为 true")
		}
		t.Logf("✅ 不同账号不同会话各自触发 AI: 调用次数=%d", trigger.called)
	})

	// ========== 场景 5：sender_type 不再区分，所有消息同等对待 ==========
	// 2026-08-07 删除 isPlatformMessage：msg_id(contentHash) 为唯一去重键，
	// sender_type 统一视为 customer（前端不判定自/他）。
	// 本条测试验证：不同 sender_type 的"新消息"（GetByMsgID 未命中）均触发 AI。
	t.Run("场景5_sender_type平等处理", func(t *testing.T) {
		trigger.called = 0

		events := []*model.MessageEvent{
			{
				Channel:        model.ChannelXHS,
				SenderID:       "customer-1",
				Content:        "客户消息",
				EventID:        "evt-customer-0",
				ConversationID: "conv-filter-1",
				SenderType:     "customer",
				MsgType:        model.MsgTypeText,
				Extra:          map[string]interface{}{"account_id": "xhs-acct-1"},
			},
			{
				Channel:        model.ChannelXHS,
				SenderID:       "self-1",
				Content:        "自己发的消息",
				EventID:        "evt-self-1",
				ConversationID: "conv-filter-1",
				SenderType:     "customer", // 前端统一 customer，self/agent 已废弃
				MsgType:        model.MsgTypeText,
				Extra:          map[string]interface{}{"account_id": "xhs-acct-1"},
			},
		}

		result, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		// 两条消息的 EventID 均未在 DB 中命中 → 视为新消息。
		// 批次合并会将同会话多条消息合并为 1 次 AI 触发调用。
		if trigger.called != 1 {
			t.Fatalf("同会话 2 条消息应合并触发 1 次 AI，实际: %d", trigger.called)
		}
		if len(result.PerEvent) != 2 {
			t.Fatalf("应返回 2 个 PerEvent，实际: %d", len(result.PerEvent))
		}
		if result.PerEvent[0].Accepted != true || result.PerEvent[1].Accepted != true {
			t.Fatalf("两条消息均应被接受: %+v / %+v", result.PerEvent[0], result.PerEvent[1])
		}
		t.Logf("✅ sender_type 不再区分：两条消息同等对待，批次合并触发 1 次 AI")
	})

	// ========== 场景 6：系统消息仅落库不触发 AI ==========
	t.Run("场景6_系统消息仅落库", func(t *testing.T) {
		trigger.called = 0

		events := []*model.MessageEvent{
			{
				Channel:        model.ChannelXHS,
				SenderID:       "system-1",
				Content:        "系统通知",
				EventID:        "evt-system-0",
				ConversationID: "conv-system-1",
				SenderType:     "system",
				MsgType:        model.MsgTypeText,
				Extra:          map[string]interface{}{"account_id": "xhs-acct-1"},
			},
		}

		result, err := svc.HandleIngressBatch(ctx, events)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		if trigger.called != 0 {
			t.Fatalf("系统消息不应触发 AI，实际: %d", trigger.called)
		}
		if result.TriggeredAI {
			t.Fatal("TriggeredAI 应为 false")
		}
		t.Logf("✅ 系统消息仅落库不触发 AI")
	})

	// ========== 场景 7：空 batch 不触发 AI ==========
	t.Run("场景7_空batch", func(t *testing.T) {
		trigger.called = 0

		result, err := svc.HandleIngressBatch(ctx, nil)
		if err != nil {
			t.Fatalf("HandleIngressBatch: %v", err)
		}
		if trigger.called != 0 {
			t.Fatalf("空 batch 不应触发 AI，实际: %d", trigger.called)
		}
		if result.TriggeredAI {
			t.Fatal("TriggeredAI 应为 false")
		}
		t.Logf("✅ 空 batch 不触发 AI")
	})

	// ========== 场景 8：内容 hash 计算验证（函数仍保留，兼容性） ==========
	t.Run("场景8_内容hash计算验证", func(t *testing.T) {
		h1 := contentHashOf("你好")
		h2 := contentHashOf("你好")
		if h1 != h2 {
			t.Fatalf("相同内容应产生相同 hash: %q vs %q", h1, h2)
		}
		if h1 == "" {
			t.Fatal("hash 不应为空")
		}
		t.Logf("✅ 相同内容 hash 一致: %s", h1)

		h3 := contentHashOf("你好啊")
		if h1 == h3 {
			t.Fatalf("不同内容应产生不同 hash: 都=%q", h1)
		}
		t.Logf("✅ 不同内容 hash 不同: %q vs %q", h1, h3)

		h4 := contentHashOf("")
		if h4 != "" {
			t.Fatalf("空内容应返回空 hash，实际: %q", h4)
		}
		t.Logf("✅ 空内容返回空 hash")
	})
}
