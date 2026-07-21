package agent_runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"marketing/internal/aiagent/rag/incremental"
	"marketing/internal/event"
)

// ============================================================================
// E2E 全链路集成测试
// ----------------------------------------------------------------------------
// 测试场景:
//   1. webhook 收到消息 → publish event → AgentRuntime.HandleCustomerMessage
//   2. 知识库文档变更 → publish event → IncrementalIndexer.Handle
//   3. 异步并行:customer + knowledge 事件无干扰
//
// 不依赖真实 DB / LLM,使用 mock bridge 和 mock indexer
// ============================================================================

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
	// 1. 准备 mock 桥接器
	salesBridge := &mockSalesBridge{fixedReply: "您好,我是 智能体"}
	indexer := rag.NewIncrementalIndexer(nil, nil, nil)

	// 2. 准备运行时
	rt := NewAgentRuntime(nil, salesBridge, nil).(*defaultAgentRuntime)
	customerHandler := NewEventSubscriber(rt)
	knowledgeHandler := indexer.Handle

	// 3. 注册到 event bus(同时设置 global bus 让 PublishCustomerMessage 能用)
	bus := event.New(2, 100)
	defer bus.Stop()
	prevGlobal := event.GetGlobalBus()
	event.SetGlobalBus(bus)
	defer event.SetGlobalBus(prevGlobal)

	bus.Subscribe(event.TopicCustomerMessageReceived, customerHandler)
	bus.Subscribe(event.TopicKnowledgeDocumentChanged, knowledgeHandler)

	// 4. 模拟 webhook 接收消息
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

	// 5. 等待异步处理
	time.Sleep(100 * time.Millisecond)

	// 6. 验证
	if salesBridge.callCount() != 1 {
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

	// 模拟 create
	PublishKnowledgeDocumentCreate(1, uint(docID), "测试内容", 1)
	time.Sleep(100 * time.Millisecond)

	if indexer.ChunkCount(uintToStr(uint(docID))) == 0 {
		t.Error("expected chunks after create event")
	}

	// 模拟 update
	PublishKnowledgeDocumentUpdate(1, uint(docID), "更新内容", 1)
	time.Sleep(100 * time.Millisecond)

	if indexer.ChunkCount(uintToStr(uint(docID))) == 0 {
		t.Error("expected chunks after update event")
	}

	// 模拟 delete
	PublishKnowledgeDocumentDelete(1, uint(docID), 1)
	time.Sleep(100 * time.Millisecond)

	if indexer.ChunkCount(uintToStr(uint(docID))) != 0 {
		t.Errorf("expected 0 chunks after delete, got %d", indexer.ChunkCount(uintToStr(uint(docID))))
	}
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

	// 混合发布 10 条 customer + 5 条 knowledge
	for i := 0; i < 10; i++ {
		PublishCustomerMessage("telegram", "tg_001", "cust_001", "msg", "")
	}
	for i, docID := range docIDs {
		PublishKnowledgeDocumentCreate(1, uint(docID), "content", uint(i+1))
	}

	// 等待
	time.Sleep(200 * time.Millisecond)

	// 验证
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

	// 不应 panic
	PublishCustomerMessage("telegram", "tg_001", "cust_001", "msg", "")

	time.Sleep(100 * time.Millisecond)

	// 没有 panic 即通过
	// nil bridge 走 fallbackResponse,不调真实引擎
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
