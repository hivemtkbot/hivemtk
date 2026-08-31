package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDispatcherDefaultMaxTokensBaseline 守护 reasoning 模型 max_tokens 基线铁律：
// 当 Dispatch 请求未显式指定 MaxTokens(<=0) 时，callProvider 必须默认 2048(≥基线)，
// 否则推理模型 reasoning 阶段会被截断到空回复，进而触发"LLM 返回空"缺陷。
// 用 httptest 本地 mock 端点，断言落到 LLM 请求体的 max_tokens==2048（无需真实网络）。
func TestDispatcherDefaultMaxTokensBaseline(t *testing.T) {
	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.Unmarshal(body, &req)
		gotMaxTokens = req.MaxTokens
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	llmService := NewLLMService()
	d := newDispatcherBase(llmService)
	provider := &ProviderConfig{
		Name:    "test",
		BaseURL: srv.URL,
		APIType: "openai",
		Model:   "test-model",
		Enabled: true,
		APIKey:  "x",
	}
	d.providers[provider.Name] = provider

	route := &ScenarioRoute{Scenario: ScenarioSOPReply, Provider: "test", MaxLatency: 10000}
	result, err := d.CallProviderForTest(context.Background(), provider, DispatchRequest{
		Scenario: ScenarioSOPReply,
		Prompt:   "hello",
	}, route)
	if err != nil {
		t.Fatalf("CallProviderForTest failed: %v", err)
	}
	if result == nil || result.Content != "hi" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if gotMaxTokens != 2048 {
		t.Fatalf("reasoning 基线缺失：默认 max_tokens 应为 2048，实际 %d", gotMaxTokens)
	}
}

// TestValidateConfigDefaultMaxTokens 守护 ValidateConfig 默认 max_tokens 基线 2048。
func TestValidateConfigDefaultMaxTokens(t *testing.T) {
	s := NewLLMService()
	cfg := &LLMConfig{Model: "test-model"}
	if err := s.ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig failed: %v", err)
	}
	if cfg.MaxTokens != 2048 {
		t.Fatalf("expected default MaxTokens 2048, got %d", cfg.MaxTokens)
	}
	def := s.GetDefaultConfig()
	if def.MaxTokens != 2048 {
		t.Fatalf("expected GetDefaultConfig MaxTokens 2048, got %d", def.MaxTokens)
	}
}
