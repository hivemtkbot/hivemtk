package agent_runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/event"
)

type mockRuntime struct {
	mu               sync.Mutex
	receivedPayloads []CustomerMessagePayload
	handleError      error
}

func (m *mockRuntime) HandleCustomerMessage(ctx context.Context, payload CustomerMessagePayload) (*SalesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedPayloads = append(m.receivedPayloads, payload)
	if m.handleError != nil {
		return nil, m.handleError
	}
	return &SalesResponse{
		ReplyContent: "mock reply",
		ReplyType:    "text",
		Confidence:   0.9,
		AgentID:      1,
		AgentCode:    "mock_agent",
		TraceID:      payload.TraceID,
		Duration:     1 * time.Millisecond,
	}, nil
}

func (m *mockRuntime) LoadAgentContext(ctx context.Context, channel, account string) (*AgentContext, error) {
	return &AgentContext{
		AgentID:   1,
		AgentCode: "mock_agent",
		AgentType: "sales",
		Channel:   channel,
		AccountID: account,
		LoadedAt:  time.Now(),
	}, nil
}

func (m *mockRuntime) RefreshCache(ctx context.Context, agentID uint) error {
	return nil
}

func (m *mockRuntime) Stop(ctx context.Context) error {
	return nil
}

func (m *mockRuntime) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.receivedPayloads)
}

// TestEventSubscriber_Handle_ValidPayload 验证正常事件被处理
func TestEventSubscriber_Handle_ValidPayload(t *testing.T) {
	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)

	payload := event.CustomerMessagePayload{
		ChannelType: "telegram",
		AccountID:   "tg_001",
		CustomerID:  "cust_001",
		Content:     "你好",
		MessageType: "text",
		Timestamp:   time.Now(),
		TraceID:     "test_trace_001",
	}

	err := handler(event.Event{
		Topic:     event.TopicCustomerMessageReceived,
		Payload:   payload,
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}
	if mock.callCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount())
	}

	got := mock.receivedPayloads[0]
	if got.ChannelType != "telegram" {
		t.Errorf("ChannelType = %s, want telegram", got.ChannelType)
	}
	if got.AccountID != "tg_001" {
		t.Errorf("AccountID = %s, want tg_001", got.AccountID)
	}
	if got.TraceID != "test_trace_001" {
		t.Errorf("TraceID = %s, want test_trace_001", got.TraceID)
	}
}

// TestEventSubscriber_Handle_InvalidPayload 验证非法载荷返回 error
func TestEventSubscriber_Handle_InvalidPayload(t *testing.T) {
	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)

	err := handler(event.Event{
		Topic:   event.TopicCustomerMessageReceived,
		Payload: "this is a string, not CustomerMessagePayload",
	})

	if err == nil {
		t.Error("expected error for invalid payload, got nil")
	}
	if mock.callCount() != 0 {
		t.Errorf("runtime should not be called, got %d calls", mock.callCount())
	}
}

// TestEventSubscriber_Handle_NilPayload 验证 nil payload 不 panic
func TestEventSubscriber_Handle_NilPayload(t *testing.T) {
	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("handler panicked: %v", r)
		}
	}()

	err := handler(event.Event{
		Topic:   event.TopicCustomerMessageReceived,
		Payload: (*event.CustomerMessagePayload)(nil),
	})

	if err == nil {
		t.Error("expected error for nil payload, got nil")
	}
}

// TestEventSubscriber_Handle_RuntimeError 验证 runtime 抛错时 handler 正常返回 error
func TestEventSubscriber_Handle_RuntimeError(t *testing.T) {
	mock := &mockRuntime{handleError: errors.New("mock runtime error")}
	handler := NewEventSubscriber(mock)

	payload := event.CustomerMessagePayload{
		ChannelType: "wecom",
		AccountID:   "wc_001",
		Content:     "test",
		TraceID:     "trace_002",
	}

	err := handler(event.Event{
		Topic:   event.TopicCustomerMessageReceived,
		Payload: payload,
	})

	if err == nil {
		t.Error("expected error from runtime, got nil")
	}
	if mock.callCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount())
	}
}

// TestEventSubscriber_Handle_AutoTraceID 验证空 TraceID 时自动生成
func TestEventSubscriber_Handle_AutoTraceID(t *testing.T) {
	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)

	payload := event.CustomerMessagePayload{
		ChannelType: "telegram",
		AccountID:   "tg_002",
		Content:     "test",
		TraceID:     "",
	}

	err := handler(event.Event{
		Topic:   event.TopicCustomerMessageReceived,
		Payload: payload,
	})

	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}
	if mock.callCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount())
	}

	got := mock.receivedPayloads[0]
	if got.TraceID == "" {
		t.Error("TraceID should be auto-generated when empty")
	}
	if !contains(got.TraceID, "agent_runtime_") {
		t.Errorf("TraceID = %s, should contain 'agent_runtime_'", got.TraceID)
	}
}

// TestEventSubscriber_Handle_PointerPayload 验证指针类型 payload
func TestEventSubscriber_Handle_PointerPayload(t *testing.T) {
	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)

	payload := &event.CustomerMessagePayload{
		ChannelType: "feishu",
		AccountID:   "fs_001",
		Content:     "feishu test",
		TraceID:     "trace_pointer_001",
	}

	err := handler(event.Event{
		Topic:   event.TopicCustomerMessageReceived,
		Payload: payload,
	})

	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}
	if mock.callCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount())
	}
	if mock.receivedPayloads[0].ChannelType != "feishu" {
		t.Errorf("ChannelType = %s, want feishu", mock.receivedPayloads[0].ChannelType)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
