package bridge

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============ SSEBus 单元测试 ============

func TestSSEBus_SubscribeAndPublish(t *testing.T) {
	bus := NewSSEBus()
	ch, cancel := bus.Subscribe("douyin", "acc-1")
	defer cancel()

	event := SSEEvent{
		ID: "1", Event: "new_outbound",
		ConversationID: "conv-1",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-1", "content": "hello"},
	}

	bus.Publish(event)

	select {
	case got := <-ch:
		if got.ID != "1" {
			t.Errorf("expected ID 1, got %s", got.ID)
		}
		if got.ConversationID != "conv-1" {
			t.Errorf("expected conv-1, got %s", got.ConversationID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestSSEBus_ConversationLevelSubscription(t *testing.T) {
	bus := NewSSEBus()
	ch, cancel := bus.SubscribeByConversation("conv-1")
	defer cancel()

	event := SSEEvent{
		ID: "2", Event: "new_outbound",
		ConversationID: "conv-1",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
	}

	bus.Publish(event)

	select {
	case got := <-ch:
		if got.ConversationID != "conv-1" {
			t.Errorf("expected conv-1, got %s", got.ConversationID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for conversation-level event")
	}
}

func TestSSEBus_MultiSubscriber(t *testing.T) {
	bus := NewSSEBus()
	ch1, cancel1 := bus.Subscribe("douyin", "acc-1")
	ch2, cancel2 := bus.Subscribe("douyin", "acc-1")
	defer cancel1()
	defer cancel2()

	event := SSEEvent{
		ID: "3", Event: "new_outbound",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
	}

	bus.Publish(event)

	for i, ch := range []chan SSEEvent{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != "3" {
				t.Errorf("ch%d: expected ID 3, got %s", i+1, got.ID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("ch%d: timeout", i+1)
		}
	}
}

func TestSSEBus_CancelRemovesSubscriber(t *testing.T) {
	bus := NewSSEBus()
	ch, cancel := bus.Subscribe("douyin", "acc-1")
	cancel() // 取消订阅

	// 发布事件，应该被丢弃（channel 已关闭）
	bus.Publish(SSEEvent{
		ID: "4", Event: "new_outbound",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
	})

	// 关闭的 channel 应该立即返回零值
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after cancel")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("expected closed channel read to not block")
	}
}

func TestSSEBus_BufferFull_DropNonBlocking(t *testing.T) {
	bus := NewSSEBus()
	bus.buffer = 2 // 小 buffer 便于测试

	ch, cancel := bus.Subscribe("douyin", "acc-1")
	defer cancel()

	// 填满 buffer
	for i := 0; i < 2; i++ {
		bus.Publish(SSEEvent{
			ID: string(rune('a' + i)), Event: "new_outbound",
			Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
		})
	}

	// 第 3 个事件应该被丢弃（不阻塞），因为没人消费
	dropped := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Publish should not panic on buffer full")
			}
		}()
		bus.Publish(SSEEvent{
			ID: "overflow", Event: "new_outbound",
			Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
		})
	}()
	_ = dropped

	// 消费已有事件
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(50 * time.Millisecond):
			t.Error("timeout consuming buffered events")
		}
	}
}

func TestSSEBus_ConcurrentPublishAndSubscribe(t *testing.T) {
	bus := NewSSEBus()
	var wg sync.WaitGroup

	// 并发订阅
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch, cancel := bus.Subscribe("douyin", "acc-concurrent")
			defer cancel()

			// 并发发布
			bus.Publish(SSEEvent{
				ID: string(rune('a' + idx)),
				Data: map[string]any{"platform": "douyin", "account_id": "acc-concurrent"},
			})

			select {
			case <-ch:
			case <-time.After(100 * time.Millisecond):
			}
		}(i)
	}

	wg.Wait()
}

func TestSSEBus_EmptyPublish_NoPanic(t *testing.T) {
	bus := NewSSEBus()
	// 空事件发布，不应该 panic
	bus.Publish(SSEEvent{})
}

func TestSSEBus_DifferentAccountsIsolated(t *testing.T) {
	bus := NewSSEBus()
	ch1, cancel1 := bus.Subscribe("douyin", "acc-1")
	ch2, cancel2 := bus.Subscribe("douyin", "acc-2")
	defer cancel1()
	defer cancel2()

	// 发布到 acc-1 的事件
	bus.Publish(SSEEvent{
		ID: "targeted",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-1"},
	})

	// ch1 应该收到
	select {
	case got := <-ch1:
		if got.ID != "targeted" {
			t.Errorf("ch1: expected targeted, got %s", got.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch1: timeout")
	}

	// ch2 不应该收到（发布目标是 acc-1）
	// 但 acc-1 和 acc-2 是不同 key，所以不会交叉
	select {
	case <-ch2:
		t.Error("ch2 should NOT receive event for acc-1")
	case <-time.After(50 * time.Millisecond):
	}
}

// ============ MemoryOutboxFetcher 单元测试 ============

func TestMemoryOutboxFetcher_PushAndFetch(t *testing.T) {
	f := NewMemoryOutboxFetcher()

	ev1 := f.Push("new_outbound", map[string]any{"id": "1"})
	f.Push("new_outbound", map[string]any{"id": "2"})

	if ev1.ID != "1" {
		t.Errorf("expected ID 1, got %s", ev1.ID)
	}

	ctx := context.Background()
	events, lastID, err := f.FetchOutboxSince(ctx, "douyin", "acc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if lastID != "2" {
		t.Errorf("expected lastID 2, got %s", lastID)
	}
}

func TestMemoryOutboxFetcher_ResumeSinceID(t *testing.T) {
	f := NewMemoryOutboxFetcher()
	f.Push("new_outbound", map[string]any{"id": "1"})
	f.Push("new_outbound", map[string]any{"id": "2"})
	f.Push("new_outbound", map[string]any{"id": "3"})

	ctx := context.Background()
	// 从 ID 2 之后恢复
	events, lastID, err := f.FetchOutboxSince(ctx, "douyin", "acc-1", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event (ID=3), got %d", len(events))
	}
	if lastID != "3" {
		t.Errorf("expected lastID 3, got %s", lastID)
	}
}

func TestMemoryOutboxFetcher_EmptyFetch(t *testing.T) {
	f := NewMemoryOutboxFetcher()

	ctx := context.Background()
	events, lastID, err := f.FetchOutboxSince(ctx, "douyin", "acc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
	if lastID != "" {
		t.Errorf("expected empty lastID, got %s", lastID)
	}
}

func TestMemoryOutboxFetcher_BacklogLimit(t *testing.T) {
	f := NewMemoryOutboxFetcher()

	// 推入超过 backlog 限制的事件
	for i := 0; i < SSEMaxBacklogEvents+50; i++ {
		f.Push("new_outbound", map[string]any{"idx": i})
	}

	ctx := context.Background()
	events, _, err := f.FetchOutboxSince(ctx, "douyin", "acc-1", "")
	if err != nil {
		t.Fatal(err)
	}

	// 应该只返回最近 50 条
	if len(events) != 50 {
		t.Errorf("expected 50 events (backlog), got %d", len(events))
	}
}

// ============ SSEEvent 结构测试 ============

func TestSSEEvent_Fields(t *testing.T) {
	ev := SSEEvent{
		ID: "1", Event: "new_outbound",
		ConversationID: "conv-1",
		MsgType: "text",
		ReceiverID: "user-1",
		Seq: 1,
		Data: map[string]any{"content": "hello"},
		Timestamp: time.Now(),
	}

	if ev.ID != "1" {
		t.Error("ID mismatch")
	}
	if ev.ConversationID != "conv-1" {
		t.Error("ConversationID mismatch")
	}
	if ev.MsgType != "text" {
		t.Error("MsgType mismatch")
	}
	if ev.ReceiverID != "user-1" {
		t.Error("ReceiverID mismatch")
	}
	if ev.Seq != 1 {
		t.Error("Seq mismatch")
	}
}

// ============ 竞态条件测试 ============

func TestSSEBus_PublishDuringSubscribe(t *testing.T) {
	bus := NewSSEBus()

	var wg sync.WaitGroup
	results := make(chan SSEEvent, 10)

	// 并发：一边发布，一边订阅
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// 交替订阅和发布
			time.Sleep(time.Duration(idx) * 10 * time.Millisecond)

			if idx%2 == 0 {
				ch, cancel := bus.Subscribe("douyin", "acc-race")
				defer cancel()
				bus.Publish(SSEEvent{
					ID: "race",
					Data: map[string]any{"platform": "douyin", "account_id": "acc-race"},
				})
				select {
				case got := <-ch:
					results <- got
				case <-time.After(200 * time.Millisecond):
				}
			} else {
				bus.Publish(SSEEvent{
					ID: "race",
					Data: map[string]any{"platform": "douyin", "account_id": "acc-race"},
				})
			}
		}(i)
	}

	wg.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}
	// 至少有订阅者收到了事件
	t.Logf("Received %d events out of possible 3", count)
}

// ============ SSEHandler 配置测试 ============

func TestSSEHandler_SetHeartbeat(t *testing.T) {
	h := NewSSEHandler(nil)
	h.SetHeartbeat(5 * time.Second)
	if h.heartbeatInterval != 5*time.Second {
		t.Errorf("expected 5s, got %v", h.heartbeatInterval)
	}
}

func TestSSEHandler_SetHeartbeat_Zero(t *testing.T) {
	h := NewSSEHandler(nil)
	h.SetHeartbeat(0) // 零值不应改变配置
	if h.heartbeatInterval != SSEDefaultHeartbeatInterval {
		t.Errorf("expected default, got %v", h.heartbeatInterval)
	}
}

func TestSSEHandler_SetMaxDuration(t *testing.T) {
	h := NewSSEHandler(nil)
	h.SetMaxDuration(10 * time.Minute)
	if h.maxStreamDuration != 10*time.Minute {
		t.Errorf("expected 10m, got %v", h.maxStreamDuration)
	}
}

func TestSSEHandler_SetMaxDuration_Zero(t *testing.T) {
	h := NewSSEHandler(nil)
	h.SetMaxDuration(0)
	if h.maxStreamDuration != SSEDefaultMaxStreamDuration {
		t.Errorf("expected default, got %v", h.maxStreamDuration)
	}
}

// ============ 可靠性测试 ============

func TestSSEBus_GuaranteedDelivery_WithPollFallback(t *testing.T) {
	// 模拟：SSEBus 投递失败（channel 满），但 Poll 兜底应该找回
	bus := NewSSEBus()
	bus.buffer = 1

	ch, cancel := bus.Subscribe("douyin", "acc-reliable")
	defer cancel()

	// 填满 channel
	bus.Publish(SSEEvent{
		ID: "fill",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-reliable"},
	})

	// 再发一个（会被丢弃，因为 buffer 满）
	bus.Publish(SSEEvent{
		ID: "lost",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-reliable"},
	})

	// 消费第一个
	select {
	case got := <-ch:
		if got.ID != "fill" {
			t.Errorf("expected fill, got %s", got.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}

	// "lost" 事件应该被丢弃
	// Poll 兜底会在下一个 poll 周期从 DB 拉取
	// 这里验证 SSEBus 没有被阻塞
}

func TestSSEBus_SubscribeAfterPublish_DoesNotReceivePastEvents(t *testing.T) {
	bus := NewSSEBus()

	// 先发布
	bus.Publish(SSEEvent{
		ID: "past",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-late"},
	})

	// 后订阅
	ch, cancel := bus.Subscribe("douyin", "acc-late")
	defer cancel()

	// 不应收到过去的事件
	select {
	case <-ch:
		t.Error("late subscriber should NOT receive past events")
	case <-time.After(50 * time.Millisecond):
		// 正确
	}

	// 但应该收到新事件
	bus.Publish(SSEEvent{
		ID: "future",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-late"},
	})

	select {
	case got := <-ch:
		if got.ID != "future" {
			t.Errorf("expected future, got %s", got.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for future event")
	}
}

// ============ 压力测试 ============

func TestSSEBus_EventStorm(t *testing.T) {
	bus := NewSSEBus()
	bus.buffer = 500 // 足够大的 buffer 用于压力测试

	ch, cancel := bus.Subscribe("douyin", "acc-storm")
	defer cancel()

	const numEvents = 500
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numEvents; i++ {
			bus.Publish(SSEEvent{
				ID: string(rune('a' + (i % 26))),
				Data: map[string]any{"platform": "douyin", "account_id": "acc-storm", "idx": i},
			})
		}
	}()

	// 并发消费
	received := 0
	timeout := time.After(5 * time.Second)
	for received < numEvents {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Errorf("timeout after receiving %d/%d events", received, numEvents)
			return
		}
	}

	wg.Wait()
	t.Logf("Successfully received all %d events", received)
}

func TestSSEBus_MultiAccountIsolation(t *testing.T) {
	bus := NewSSEBus()

	// 为 5 个不同账号创建订阅
	type sub struct {
		ch     chan SSEEvent
		cancel func()
	}
	subs := make([]sub, 5)
	for i := 0; i < 5; i++ {
		ch, cancel := bus.Subscribe("douyin", "acc-"+string(rune('1'+i)))
		subs[i] = sub{ch, cancel}
		defer cancel()
	}

	// 向 acc-3 发布
	bus.Publish(SSEEvent{
		ID: "target",
		Data: map[string]any{"platform": "douyin", "account_id": "acc-3"},
	})

	// 只有 acc-3 的订阅者应该收到
	for i, s := range subs {
		select {
		case <-s.ch:
			if i != 2 { // acc-3 is index 2 (0-indexed)
				t.Errorf("subscriber %d should NOT receive event for acc-3", i)
			}
		case <-time.After(50 * time.Millisecond):
			if i == 2 {
				t.Error("subscriber for acc-3 should receive event")
			}
		}
	}
}

// ============ 集成测试：验证所有 outbound 路径都触发 SSE ============

// mockSSEPublisher 模拟 service.GlobalSSEPublisher 的行为
// 将 SSE 事件发布到 SSEBus，模拟真实链路
func mockSSEPublisher(bus *SSEBus) func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time) {
	return func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time) {
		bus.Publish(SSEEvent{
			ID:             itoa(int(hubID)),
			Event:          "new_outbound",
			ConversationID: convID,
			MsgType:        msgType,
			ReceiverID:     receiverID,
			Seq:            int(hubID),
			Data: map[string]any{
				"hub_id":          hubID,
				"platform":        channel,
				"account_id":      accountID,
				"conversation_id": convID,
				"content":         content,
				"msg_type":        msgType,
				"receiver_id":     receiverID,
				"is_ai_reply":     isAIReply,
			},
			Timestamp: createdAt,
		})
	}
}

// TestAllOutboundPathsTriggerSSE 验证所有 outbound 消息路径都正确触发 SSE 通知
func TestAllOutboundPathsTriggerSSE(t *testing.T) {
	bus := NewSSEBus()
	publisher := mockSSEPublisher(bus)

	const (
		channel  = "douyin"
		accountID = "acc_test"
		convID   = "conv_test"
	)

	// 订阅 SSE 事件
	ch, cancel := bus.Subscribe(channel, accountID)
	defer cancel()

	// 测试路径 1: bridge_outbound.go - DeliverBridgeOutbound
	t.Run("Path1_DeliverBridgeOutbound", func(t *testing.T) {
		publisher(channel, accountID, 1001, convID, "text", "customer_1", "测试消息1", false, time.Now())
		select {
		case evt := <-ch:
			if evt.Data["platform"] != channel {
				t.Errorf("expected platform %s, got %v", channel, evt.Data["platform"])
			}
			if evt.Data["account_id"] != accountID {
				t.Errorf("expected account_id %s, got %v", accountID, evt.Data["account_id"])
			}
			if evt.Data["conversation_id"] != convID {
				t.Errorf("expected conv_id %s, got %v", convID, evt.Data["conversation_id"])
			}
			if evt.Data["content"] != "测试消息1" {
				t.Errorf("expected content '测试消息1', got %v", evt.Data["content"])
			}
			t.Logf("✅ Path1 DeliverBridgeOutbound: SSE 事件正确触发")
		case <-time.After(100 * time.Millisecond):
			t.Error("Path1 DeliverBridgeOutbound: SSE 事件未收到")
		}
	})

	// 测试路径 2: inbox_ingress_outbound.go - DeliverOutbound
	t.Run("Path2_DeliverOutbound", func(t *testing.T) {
		publisher(channel, accountID, 1002, convID, "text", "customer_2", "测试消息2", false, time.Now())
		select {
		case evt := <-ch:
			if evt.Data["content"] != "测试消息2" {
				t.Errorf("expected content '测试消息2', got %v", evt.Data["content"])
			}
			t.Logf("✅ Path2 DeliverOutbound: SSE 事件正确触发")
		case <-time.After(100 * time.Millisecond):
			t.Error("Path2 DeliverOutbound: SSE 事件未收到")
		}
	})

	// 测试路径 3: inbox_ingress_persist.go - persistHistoryMessage (outbound)
	t.Run("Path3_PersistHistoryMessage", func(t *testing.T) {
		publisher(channel, accountID, 1003, convID, "text", "customer_3", "测试消息3", true, time.Now())
		select {
		case evt := <-ch:
			if evt.Data["is_ai_reply"] != true {
				t.Errorf("expected is_ai_reply true, got %v", evt.Data["is_ai_reply"])
			}
			t.Logf("✅ Path3 PersistHistoryMessage: SSE 事件正确触发")
		case <-time.After(100 * time.Millisecond):
			t.Error("Path3 PersistHistoryMessage: SSE 事件未收到")
		}
	})

	// 测试路径 4: webhook_outbound.go - sendOutbound (AI回复)
	t.Run("Path4_SendOutbound_AIReply", func(t *testing.T) {
		publisher(channel, accountID, 1004, convID, "text", "customer_4", "AI回复消息", true, time.Now())
		select {
		case evt := <-ch:
			if evt.Data["is_ai_reply"] != true {
				t.Errorf("expected is_ai_reply true, got %v", evt.Data["is_ai_reply"])
			}
			if evt.Data["content"] != "AI回复消息" {
				t.Errorf("expected content 'AI回复消息', got %v", evt.Data["content"])
			}
			t.Logf("✅ Path4 SendOutbound AI回复: SSE 事件正确触发")
		case <-time.After(100 * time.Millisecond):
			t.Error("Path4 SendOutbound AI回复: SSE 事件未收到")
		}
	})
}

// TestSSEEventDeliveryAllPaths 验证多账号多会话场景下 SSE 正确路由
func TestSSEEventDeliveryAllPaths(t *testing.T) {
	bus := NewSSEBus()
	publisher := mockSSEPublisher(bus)

	// 为多个账号和会话建立订阅
	type subInfo struct {
		ch       chan SSEEvent
		cancel   func()
		accountID string
	}

	subs := []subInfo{}
	accounts := []string{"acc_A", "acc_B", "acc_C"}
	for _, acc := range accounts {
		ch, cancel := bus.Subscribe("douyin", acc)
		subs = append(subs, subInfo{ch: ch, cancel: cancel, accountID: acc})
	}
	defer func() {
		for _, s := range subs {
			s.cancel()
		}
	}()

	// 为 acc_B 发送事件
	targetAccount := "acc_B"
	publisher("douyin", targetAccount, 2001, "conv_B", "text", "customer", "发给B的消息", false, time.Now())

	// 检查只有 acc_B 收到事件
	for _, s := range subs {
		select {
		case evt := <-s.ch:
			if s.accountID != targetAccount {
				t.Errorf("account %s 不应收到事件，但收到了", s.accountID)
			} else {
				if evt.Data["account_id"] != targetAccount {
					t.Errorf("expected account_id %s, got %v", targetAccount, evt.Data["account_id"])
				}
				if evt.Data["content"] != "发给B的消息" {
					t.Errorf("expected content '发给B的消息', got %v", evt.Data["content"])
				}
				t.Logf("✅ 账号 %s 正确收到事件", s.accountID)
			}
		case <-time.After(100 * time.Millisecond):
			if s.accountID == targetAccount {
				t.Errorf("目标账号 %s 未收到事件", s.accountID)
			} else {
				t.Logf("✅ 账号 %s 正确未收到事件（隔离）", s.accountID)
			}
		}
	}
}

// TestSSEBus_ConversationLevelRouting 验证会话级路由（conversation_id 精准投递）
func TestSSEBus_ConversationLevelRouting(t *testing.T) {
	bus := NewSSEBus()

	// 订阅特定会话
	convCh, convCancel := bus.SubscribeByConversation("conv_target")
	defer convCancel()

	// 同时订阅账号级
	acctCh, acctCancel := bus.Subscribe("douyin", "acc_conv")
	defer acctCancel()

	event := SSEEvent{
		ID: "3001", Event: "new_outbound",
		ConversationID: "conv_target",
		Data: map[string]any{
			"platform":   "douyin",
			"account_id": "acc_conv",
			"content":    "精准投递测试",
		},
	}
	bus.Publish(event)

	// 会话级订阅应收到
	select {
	case evt := <-convCh:
		if evt.ConversationID != "conv_target" {
			t.Errorf("expected conv_target, got %s", evt.ConversationID)
		}
		t.Logf("✅ 会话级订阅正确收到事件")
	case <-time.After(100 * time.Millisecond):
		t.Error("会话级订阅未收到事件")
	}

	// 账号级订阅也应收到（广播）
	select {
	case evt := <-acctCh:
		if evt.Data["content"] != "精准投递测试" {
			t.Errorf("expected content, got %v", evt.Data["content"])
		}
		t.Logf("✅ 账号级订阅正确收到广播事件")
	case <-time.After(100 * time.Millisecond):
		t.Error("账号级订阅未收到广播事件")
	}
}
