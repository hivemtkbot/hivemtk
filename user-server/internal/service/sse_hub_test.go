package service


import (
	"context"
	"sync"
	"testing"
	"time"
)


// 1. 创建客户端
func TestSSEClient_New(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls, SSETopicIntentRecogn})
	if c.ID(context.Background()) != "c-1" {
		t.Errorf("expected c-1, got %s", c.ID(context.Background()))
	}
	if c.IP(context.Background()) != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", c.IP(context.Background()))
	}
	if len(c.Topics(context.Background())) != 2 {
		t.Errorf("expected 2 topics, got %d", len(c.Topics(context.Background())))
	}
}

// 2. IsSubscribed
func TestSSEClient_IsSubscribed(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	if !c.IsSubscribed(context.Background(), SSETopicLLMCalls) {
		t.Error("expected subscribed to llm_calls")
	}
	if c.IsSubscribed(context.Background(), SSETopicIntentRecogn) {
		t.Error("expected NOT subscribed to intent_recognition")
	}
}

// 3. Send 成功
func TestSSEClient_SendSuccess(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	event := SSEEvent{Topic: SSETopicLLMCalls, EventType: "test", Data: "hello"}
	if !c.Send(context.Background(), event) {
		t.Error("expected send success")
	}
	select {
	case got := <-c.Events(context.Background()):
		if got.EventType != "test" {
			t.Errorf("expected test, got %s", got.EventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

// 4. Send 缓冲区满返回 false
func TestSSEClient_SendBufferFull(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	for i := 0; i < SSEClientBufferSize; i++ {
		if !c.Send(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "test"}) {
			t.Fatalf("expected send success on iteration %d", i)
		}
	}
	if c.Send(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "overflow"}) {
		t.Error("expected send fail on buffer full")
	}
}

// 5. Close 关闭后 Send 返回 false
func TestSSEClient_Close(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	c.Close(context.Background())
	if !c.Closed(context.Background()) {
		t.Error("expected closed")
	}
	if c.Send(context.Background(), SSEEvent{Topic: SSETopicLLMCalls}) {
		t.Error("expected send fail after close")
	}
}

// 6. CloseCh 接收信号
func TestSSEClient_CloseCh(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	go func() {
		time.Sleep(50 * time.Millisecond)
		c.Close(context.Background())
	}()
	select {
	case <-c.CloseCh(context.Background()):
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for close signal")
	}
}

// 7. Close 重复调用安全
func TestSSEClient_CloseIdempotent(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	c.Close(context.Background())
	c.Close(context.Background()) 
	c.Close(context.Background())
}


// 8. Register / Unregister
func TestSSEHub_RegisterUnregister(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	if err := hub.Register(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if hub.GetClientCount(context.Background()) != 1 {
		t.Errorf("expected 1 client, got %d", hub.GetClientCount(context.Background()))
	}
	hub.Unregister(context.Background(), "c-1")
	if hub.GetClientCount(context.Background()) != 0 {
		t.Errorf("expected 0 clients, got %d", hub.GetClientCount(context.Background()))
	}
}

// 9. Register nil 客户端
func TestSSEHub_RegisterNil(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	if err := hub.Register(context.Background(), nil); err == nil {
		t.Error("expected error for nil client")
	}
}

// 10. Register 重复 ID
func TestSSEHub_RegisterDuplicateID(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	c1 := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	c2 := NewSSEClient("c-1", "127.0.0.2", []string{SSETopicLLMCalls})
	if err := hub.Register(context.Background(), c1); err != nil {
		t.Fatal(err)
	}
	if err := hub.Register(context.Background(), c2); err == nil {
		t.Error("expected error for duplicate id")
	}
}

// 11. 单 IP 连接上限
func TestSSEHub_MaxConnPerIP(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	for i := 0; i < SSEMaxConnPerIP; i++ {
		c := NewSSEClient("c-"+string(rune(i)), "127.0.0.1", []string{SSETopicLLMCalls})
		if err := hub.Register(context.Background(), c); err != nil {
			t.Fatalf("register %d failed: %v", i, err)
		}
	}
	c := NewSSEClient("c-6", "127.0.0.1", []string{SSETopicLLMCalls})
	if err := hub.Register(context.Background(), c); err == nil {
		t.Error("expected error for exceeded max conn per IP")
	}
}

// 12. 不同 IP 不受限
func TestSSEHub_DifferentIPsNotLimited(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	for i := 0; i < SSEMaxConnPerIP+2; i++ {
		ip := "127.0.0." + string(rune('1'+i))
		c := NewSSEClient("c-"+string(rune(i)), ip, []string{SSETopicLLMCalls})
		if err := hub.Register(context.Background(), c); err != nil {
			t.Fatalf("register %d failed: %v", i, err)
		}
	}
}

// 13. Publish 广播到订阅者
func TestSSEHub_Publish(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	c1 := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	c2 := NewSSEClient("c-2", "127.0.0.2", []string{SSETopicLLMCalls})
	c3 := NewSSEClient("c-3", "127.0.0.3", []string{SSETopicIntentRecogn})
	_ = hub.Register(context.Background(), c1)
	_ = hub.Register(context.Background(), c2)
	_ = hub.Register(context.Background(), c3)

	hub.Publish(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "test"})

	select {
	case <-c1.Events(context.Background()):
	case <-time.After(100 * time.Millisecond):
		t.Error("c1 timeout")
	}
	select {
	case <-c2.Events(context.Background()):
	case <-time.After(100 * time.Millisecond):
		t.Error("c2 timeout")
	}
	select {
	case <-c3.Events(context.Background()):
		t.Error("c3 should NOT receive event")
	case <-time.After(100 * time.Millisecond):
	}
}

// 14. Publish 自动设置 timestamp
func TestSSEHub_PublishSetsTimestamp(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	_ = hub.Register(context.Background(), c)

	before := time.Now()
	hub.Publish(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "test"})
	select {
	case got := <-c.Events(context.Background()):
		if got.Timestamp.Before(before) {
			t.Error("timestamp should be >= publish time")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout")
	}
}

// 15. Unregister 不存在的 client
func TestSSEHub_UnregisterNonExistent(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	hub.Unregister(context.Background(), "nonexistent") 
}

// 16. Stop 关闭所有客户端
func TestSSEHub_StopAllClients(t *testing.T) {
	hub := NewSSEHub()
	c1 := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	c2 := NewSSEClient("c-2", "127.0.0.2", []string{SSETopicLLMCalls})
	_ = hub.Register(context.Background(), c1)
	_ = hub.Register(context.Background(), c2)
	hub.Stop(context.Background())
	if !c1.Closed(context.Background()) {
		t.Error("c1 should be closed")
	}
	if !c2.Closed(context.Background()) {
		t.Error("c2 should be closed")
	}
	if hub.GetClientCount(context.Background()) != 0 {
		t.Errorf("expected 0 clients after stop, got %d", hub.GetClientCount(context.Background()))
	}
}

// 17. Stop 后 Publish 不 panic
func TestSSEHub_PublishAfterStop(t *testing.T) {
	hub := NewSSEHub()
	hub.Stop(context.Background())
	hub.Publish(context.Background(), SSEEvent{Topic: SSETopicLLMCalls}) 
}

// 18. Stop 幂等
func TestSSEHub_StopIdempotent(t *testing.T) {
	hub := NewSSEHub()
	hub.Stop(context.Background())
	hub.Stop(context.Background())
	hub.Stop(context.Background())
}

// 19. GetIPCount
func TestSSEHub_GetIPCount(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	_ = hub.Register(context.Background(), NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls}))
	_ = hub.Register(context.Background(), NewSSEClient("c-2", "127.0.0.1", []string{SSETopicLLMCalls}))
	_ = hub.Register(context.Background(), NewSSEClient("c-3", "127.0.0.2", []string{SSETopicLLMCalls}))
	if hub.GetIPCount(context.Background(), "127.0.0.1") != 2 {
		t.Errorf("expected 2 for 127.0.0.1, got %d", hub.GetIPCount(context.Background(), "127.0.0.1"))
	}
	if hub.GetIPCount(context.Background(), "127.0.0.2") != 1 {
		t.Errorf("expected 1 for 127.0.0.2, got %d", hub.GetIPCount(context.Background(), "127.0.0.2"))
	}
	if hub.GetIPCount(context.Background(), "127.0.0.3") != 0 {
		t.Errorf("expected 0 for 127.0.0.3, got %d", hub.GetIPCount(context.Background(), "127.0.0.3"))
	}
}

// 20. GetClient
func TestSSEHub_GetClient(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	_ = hub.Register(context.Background(), c)
	if hub.GetClient(context.Background(), "c-1") == nil {
		t.Error("expected non-nil client")
	}
	if hub.GetClient(context.Background(), "nonexistent") != nil {
		t.Error("expected nil for nonexistent")
	}
}

// 21. ListClients
func TestSSEHub_ListClients(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	_ = hub.Register(context.Background(), NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls}))
	_ = hub.Register(context.Background(), NewSSEClient("c-2", "127.0.0.2", []string{SSETopicIntentRecogn}))
	clients := hub.ListClients(context.Background())
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
}


// 22. ParseTopics 正常解析
func TestParseTopics_Normal(t *testing.T) {
	topics := ParseTopics("llm_calls,intent_recognition")
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}
	if topics[0] != SSETopicLLMCalls {
		t.Errorf("expected llm_calls, got %s", topics[0])
	}
	if topics[1] != SSETopicIntentRecogn {
		t.Errorf("expected intent_recognition, got %s", topics[1])
	}
}

// 23. ParseTopics 空字符串
func TestParseTopics_Empty(t *testing.T) {
	if topics := ParseTopics(""); len(topics) != 0 {
		t.Errorf("expected 0 topics, got %d", len(topics))
	}
}

// 24. ParseTopics 过滤非法 topic
func TestParseTopics_FilterInvalid(t *testing.T) {
	topics := ParseTopics("llm_calls,invalid_topic,intent_recognition")
	if len(topics) != 2 {
		t.Fatalf("expected 2 valid topics, got %d", len(topics))
	}
}

// 25. ParseTopics 含空格
func TestParseTopics_WithSpaces(t *testing.T) {
	topics := ParseTopics("llm_calls, intent_recognition, rag_queries")
	if len(topics) != 3 {
		t.Fatalf("expected 3 topics, got %d", len(topics))
	}
}

// 26. IsValidSSETopic
func TestIsValidSSETopic(t *testing.T) {
	valid := []string{
		SSETopicLLMCalls, SSETopicIntentRecogn, SSETopicRAGQueries,
		SSETopicAgentActions, SSETopicHumanizeScores, SSETopicSystemAlerts,
	}
	for _, topic := range valid {
		if !IsValidSSETopic(topic) {
			t.Errorf("expected %s to be valid", topic)
		}
	}
	if IsValidSSETopic("invalid") {
		t.Error("expected invalid topic to be invalid")
	}
}


// 27. InitGlobalSSEHub 单例
func TestInitGlobalSSEHub_Singleton(t *testing.T) {
	hub := InitGlobalSSEHub()
	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	hub2 := GetGlobalSSEHub()
	if hub2 != hub {
		t.Error("expected same hub instance")
	}
}

// 28. PublishSSEEvent 不 panic（全局 Hub）
func TestPublishSSEEvent_NoPanic(t *testing.T) {
	PublishSSEEvent(SSETopicLLMCalls, "test", "data", "trace-1")
}


// 29. 并发 Register / Unregister / Publish
func TestSSEHub_Concurrent(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := NewSSEClient("c-"+string(rune(idx)), "127.0.0.1", []string{SSETopicLLMCalls})
			_ = hub.Register(context.Background(), c)
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Publish(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "concurrent"})
		}()
	}
	wg.Wait()
	if hub.Stopped(context.Background()) {
		t.Error("hub should not be stopped")
	}
}

// 30. 并发 Publish 到同一 client
func TestSSEClient_ConcurrentSend(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Send(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "concurrent"})
		}()
	}
	wg.Wait()
}


// 31. 客户端缓冲区大小校验
func TestSSEClient_BufferSize(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	if cap(c.eventCh) != SSEClientBufferSize {
		t.Errorf("expected buffer %d, got %d", SSEClientBufferSize, cap(c.eventCh))
	}
}

// 32. 空 topics 列表创建客户端
func TestSSEClient_EmptyTopics(t *testing.T) {
	c := NewSSEClient("c-1", "127.0.0.1", []string{})
	if len(c.Topics(context.Background())) != 0 {
		t.Errorf("expected 0 topics, got %d", len(c.Topics(context.Background())))
	}
	if c.IsSubscribed(context.Background(), SSETopicLLMCalls) {
		t.Error("expected NOT subscribed to any topic")
	}
}

// 33. Publish 到无订阅者的 topic
func TestSSEHub_PublishNoSubscribers(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	hub.Publish(context.Background(), SSEEvent{Topic: SSETopicLLMCalls, EventType: "no-listeners"})
}

// 34. Unregister 后 IP 计数减少
func TestSSEHub_UnregisterDecrementsIPCount(t *testing.T) {
	hub := NewSSEHub()
	defer hub.Stop(context.Background())
	_ = hub.Register(context.Background(), NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls}))
	_ = hub.Register(context.Background(), NewSSEClient("c-2", "127.0.0.1", []string{SSETopicLLMCalls}))
	if hub.GetIPCount(context.Background(), "127.0.0.1") != 2 {
		t.Fatalf("expected 2, got %d", hub.GetIPCount(context.Background(), "127.0.0.1"))
	}
	hub.Unregister(context.Background(), "c-1")
	if hub.GetIPCount(context.Background(), "127.0.0.1") != 1 {
		t.Errorf("expected 1 after unregister, got %d", hub.GetIPCount(context.Background(), "127.0.0.1"))
	}
	hub.Unregister(context.Background(), "c-2")
	if hub.GetIPCount(context.Background(), "127.0.0.1") != 0 {
		t.Errorf("expected 0 after all unregistered, got %d", hub.GetIPCount(context.Background(), "127.0.0.1"))
	}
}

// 35. 客户端 createdAt 时间记录
func TestSSEClient_CreatedAt(t *testing.T) {
	before := time.Now()
	c := NewSSEClient("c-1", "127.0.0.1", []string{SSETopicLLMCalls})
	after := time.Now()
	if c.createdAt.Before(before) || c.createdAt.After(after) {
		t.Errorf("createdAt %v not in [%v, %v]", c.createdAt, before, after)
	}
}

