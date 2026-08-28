package agent_runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	rag "hivemtk-user/internal/aiagent/rag/incremental"
	"hivemtk-user/internal/event"
)


// mockSalesBridge 模拟销售引擎桥接器
type mockSalesBridge struct {
	mu         sync.Mutex
	calls      []*mockSalesCall
	fixedReply string
}

type mockSalesCall struct {
	Channel    string
	AccountID  string
	CustomerID string
	Content    string
	TraceID    string
}

func (b *mockSalesBridge) HandleWithAgent(ctx context.Context, agentCtx *AgentContext, req *SalesRequest) (*SalesResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	reply := b.fixedReply
	if reply == "" {
		reply = "AI 收到您的消息,正在处理..."
	}

	b.calls = append(b.calls, &mockSalesCall{
		Channel:    req.Channel,
		AccountID:  req.AccountID,
		CustomerID: req.CustomerID,
		Content:    req.Content,
		TraceID:    req.TraceID,
	})

	return &SalesResponse{
		ReplyContent:   reply,
		ReplyType:      "text",
		Confidence:     0.95,
		AgentID:        agentCtx.AgentID,
		AgentCode:      agentCtx.AgentCode,
		Channel:        req.Channel,
		CustomerID:     req.CustomerID,
		TraceID:        req.TraceID,
		ToolsCalled:    []string{"rag", "sop"},
		LLMModel:       "mock-model",
		TokensUsed:     100,
		HandoffToHuman: false,
		StopReason:     "completed",
		Duration:       10 * time.Millisecond,
	}, nil
}

func (b *mockSalesBridge) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.calls)
}

// TestE2E_CustomerMessageFlow 测试客户消息全链路
func TestE2E_CustomerMessageFlow(t *testing.T) {
	salesBridge := &mockSalesBridge{fixedReply: "您好,我是 智能体"}
	indexer := rag.NewIncrementalIndexer(nil, nil, nil)

	rt := NewAgentRuntime(nil, salesBridge, nil).(*defaultAgentRuntime)
	customerHandler := NewEventSubscriber(rt)
	knowledgeHandler := indexer.Handle

	bus := event.New(2, 100)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	bus.Subscribe(event.TopicCustomerMessageReceived, customerHandler)
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, knowledgeHandler)

	customerPayload := event.CustomerMessagePayload{
		ChannelType: "telegram",
		AccountID:   "tg_e2e_001",
		CustomerID:  "cust_e2e_001",
		Content:     "请问你们的价格是多少?",
		MessageType: "text",
		Timestamp:   time.Now(),
		TraceID:     "e2e_trace_001",
	}

	event.Publish(event.TopicCustomerMessageReceived, customerPayload)

	if !waitUntil(func() bool { return salesBridge.callCount() == 1 }, 5*time.Second) {
		t.Errorf("expected 1 sales call, got %d", salesBridge.callCount())
	}

	if len(salesBridge.calls) == 0 {
		t.Fatal("no sales calls recorded")
	}

	call := salesBridge.calls[0]
	if call.Channel != "telegram" {
		t.Errorf("Channel = %s, want telegram", call.Channel)
	}
	if call.Content != "请问你们的价格是多少?" {
		t.Errorf("Content = %s, want original message", call.Content)
	}
	if call.TraceID != "e2e_trace_001" {
		t.Errorf("TraceID = %s, want e2e_trace_001", call.TraceID)
	}
}

// TestE2E_KnowledgeIndexFlow 测试知识库增量索引全链路
func TestE2E_KnowledgeIndexFlow(t *testing.T) {
	indexer, docID := setupE2EKnowledgeTestEnv(t, "e2e_knowledge_doc_1000")
	bus := event.New(2, 100)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	bus.Subscribe(event.TopicKnowledgeDocumentChanged, indexer.Handle)

	PublishKnowledgeDocumentCreate("1", uint(docID), "测试内容", 1)
	if !waitUntil(func() bool { return indexer.ChunkCount(uintToStr(uint(docID))) > 0 }, 5*time.Second) {
		t.Error("expected chunks after create event")
	}

	PublishKnowledgeDocumentUpdate("1", uint(docID), "更新内容", 1)
	if !waitUntil(func() bool { return indexer.ChunkCount(uintToStr(uint(docID))) > 0 }, 5*time.Second) {
		t.Error("expected chunks after update event")
	}

	PublishKnowledgeDocumentDelete("1", uint(docID), 1)
	if !waitUntil(func() bool { return indexer.ChunkCount(uintToStr(uint(docID))) == 0 }, 5*time.Second) {
		t.Errorf("expected 0 chunks after delete, got %d", indexer.ChunkCount(uintToStr(uint(docID))))
	}
}

// waitUntil 每隔 20ms 轮询 cond 直到为真或超时（异步事件消费的确定性等待，替代固定 sleep）
func waitUntil(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// TestE2E_CustomerAndKnowledgeInParallel 测试两路事件并行无干扰
func TestE2E_CustomerAndKnowledgeInParallel(t *testing.T) {
	salesBridge := &mockSalesBridge{}
	indexer, docIDs := setupE2EKnowledgeTestEnvMulti(t, 5)
	rt := NewAgentRuntime(nil, salesBridge, nil).(*defaultAgentRuntime)

	bus := event.New(4, 200)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	bus.Subscribe(event.TopicCustomerMessageReceived, NewEventSubscriber(rt))
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, indexer.Handle)

	for i := 0; i < 10; i++ {
		PublishCustomerMessage("telegram", "tg_001", "cust_001", "", "msg", "")
	}
	for i, docID := range docIDs {
		PublishKnowledgeDocumentCreate("1", uint(docID), "content", uint(i+1))
	}

	// 异步消费耗时不确定（索引构建约 400ms/条），固定 sleep 有竞态，改轮询等待
	deadline := time.Now().Add(5 * time.Second)
	for salesBridge.callCount() != 10 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	for time.Now().Before(deadline) {
		allIndexed := true
		for _, docID := range docIDs {
			if indexer.ChunkCount(uintToStr(uint(docID))) == 0 {
				allIndexed = false
				break
			}
		}
		if allIndexed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if salesBridge.callCount() != 10 {
		t.Errorf("expected 10 customer calls, got %d", salesBridge.callCount())
	}
	for i, docID := range docIDs {
		if indexer.ChunkCount(uintToStr(uint(docID))) == 0 {
			t.Errorf("expected chunks for doc %d (index %d)", docID, i)
		}
	}
}

// TestE2E_NilBridgeFallback 测试 nil 桥接器降级
func TestE2E_NilBridgeFallback(t *testing.T) {
	rt := NewAgentRuntime(nil, nil, nil).(*defaultAgentRuntime)
	customerHandler := NewEventSubscriber(rt)

	bus := event.New(2, 100)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	bus.Subscribe(event.TopicCustomerMessageReceived, customerHandler)

	PublishCustomerMessage("telegram", "tg_001", "cust_001", "", "msg", "")

	time.Sleep(100 * time.Millisecond)

}

// uintToStr uint → string(辅助)
func uintToStr(v uint) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

