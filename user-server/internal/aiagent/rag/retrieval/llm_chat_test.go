package ragretrieval


import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockLLMChatClient mock LLMChatClient
type mockLLMChatClient struct {
	resp       string
	err        error
	lastPrompt string
	lastOpts   LLMChatOptions
}

func (m *mockLLMChatClient) Chat(_ context.Context, prompt string, opts LLMChatOptions) (string, error) {
	m.lastPrompt = prompt
	m.lastOpts = opts
	if m.err != nil {
		return "", m.err
	}
	return m.resp, nil
}

func TestNoopLLMChatClient_AlwaysErrors(t *testing.T) {
	c := NoopLLMChatClient{}
	_, err := c.Chat(context.Background(), "hello", LLMChatOptions{})
	if err == nil {
		t.Error("NoopLLMChatClient should always return error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error should mention disabled, got=%v", err)
	}
}

func TestMockLLMChatClient(t *testing.T) {
	m := &mockLLMChatClient{resp: "test response"}
	out, err := m.Chat(context.Background(), "hello", LLMChatOptions{Temperature: 0.5, MaxTokens: 100})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != "test response" {
		t.Errorf("got=%q want=test response", out)
	}
	if m.lastPrompt != "hello" {
		t.Errorf("lastPrompt=%q want=hello", m.lastPrompt)
	}
	if m.lastOpts.Temperature != 0.5 {
		t.Errorf("Temperature=%.2f want=0.5", m.lastOpts.Temperature)
	}
	if m.lastOpts.MaxTokens != 100 {
		t.Errorf("MaxTokens=%d want=100", m.lastOpts.MaxTokens)
	}
}

func TestMockLLMChatClient_WithError(t *testing.T) {
	expectedErr := errors.New("LLM unavailable")
	m := &mockLLMChatClient{err: expectedErr}
	_, err := m.Chat(context.Background(), "hello", LLMChatOptions{})
	if !errors.Is(err, expectedErr) {
		t.Errorf("err=%v want=%v", err, expectedErr)
	}
}

// 接口编译时断言（确保 mock 实现接口）
var _ LLMChatClient = (*mockLLMChatClient)(nil)

