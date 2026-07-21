package ragretrieval

// hyde_generator_test.go HyDE 生成器单元测试

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHyDEGenerator_NilClientAutoDisabled(t *testing.T) {
	g := NewHyDEGenerator(nil, nil)
	if g.IsEnabled() {
		t.Error("nil chatClient should auto-disable")
	}
}

func TestHyDEGenerator_DisabledByConfig(t *testing.T) {
	m := &mockLLMChatClient{resp: "doc"}
	g := NewHyDEGenerator(m, &HyDEGeneratorConfig{Enabled: false})
	if g.IsEnabled() {
		t.Error("Enabled=false should disable")
	}
}

func TestHyDEGenerator_EnabledByDefault(t *testing.T) {
	m := &mockLLMChatClient{resp: "doc"}
	g := NewHyDEGenerator(m, nil)
	if !g.IsEnabled() {
		t.Error("default config should enable when client non-nil")
	}
}

func TestHyDEGenerator_DisabledReturnsEmptyNoError(t *testing.T) {
	g := NewHyDEGenerator(nil, nil)
	out, err := g.Generate(context.Background(), "如何退货")
	if err != nil {
		t.Errorf("disabled Generate should not error: %v", err)
	}
	if out != "" {
		t.Errorf("disabled Generate should return empty, got=%q", out)
	}
}

func TestHyDEGenerator_Success(t *testing.T) {
	hydeDoc := "用户在购买商品后，如需退货，应在收货之日起 7 天内提交退货申请。退货商品需保持原包装完好，配件齐全。审核通过后将原路退款至支付账户。"
	m := &mockLLMChatClient{resp: hydeDoc}
	g := NewHyDEGenerator(m, nil)
	out, err := g.Generate(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if out != hydeDoc {
		t.Errorf("output mismatch: got=%q want=%q", out, hydeDoc)
	}
	// 验证 prompt 包含查询
	if !strings.Contains(m.lastPrompt, "如何退货") {
		t.Errorf("prompt should contain query: %s", m.lastPrompt)
	}
	// 验证 temperature=0.3
	if m.lastOpts.Temperature != 0.3 {
		t.Errorf("Temperature=%.2f want=0.3", m.lastOpts.Temperature)
	}
}

func TestHyDEGenerator_LLMError(t *testing.T) {
	m := &mockLLMChatClient{err: errors.New("LLM down")}
	g := NewHyDEGenerator(m, nil)
	_, err := g.Generate(context.Background(), "如何退货")
	if err == nil {
		t.Fatal("should propagate LLM error")
	}
	if !strings.Contains(err.Error(), "HyDE LLM 调用失败") {
		t.Errorf("error should wrap HyDE prefix, got=%v", err)
	}
}

func TestHyDEGenerator_DocTooShort(t *testing.T) {
	// 默认 minDocLength=20，输出短于 20 字符应报错
	m := &mockLLMChatClient{resp: "短文档"} // 3 字符
	g := NewHyDEGenerator(m, nil)
	_, err := g.Generate(context.Background(), "如何退货")
	if err == nil {
		t.Fatal("short doc should error")
	}
	if !strings.Contains(err.Error(), "过短") {
		t.Errorf("error should mention 过短, got=%v", err)
	}
}

func TestHyDEGenerator_CustomMinLength(t *testing.T) {
	// 自定义 minDocLength=2，"短文档"（3 字符）应通过
	m := &mockLLMChatClient{resp: "短文档"}
	g := NewHyDEGenerator(m, &HyDEGeneratorConfig{Enabled: true, MinDocLength: 2})
	out, err := g.Generate(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("should pass with custom min length: %v", err)
	}
	if out != "短文档" {
		t.Errorf("output=%q want=短文档", out)
	}
}

func TestHyDEGenerator_MaxTokensFromConfig(t *testing.T) {
	m := &mockLLMChatClient{resp: "假设文档内容"}
	g := NewHyDEGenerator(m, &HyDEGeneratorConfig{Enabled: true, MaxDocTokens: 300})
	_, _ = g.Generate(context.Background(), "如何退货")
	// MaxTokens 应为 MaxDocTokens * 2 = 600
	if m.lastOpts.MaxTokens != 600 {
		t.Errorf("MaxTokens=%d want=600", m.lastOpts.MaxTokens)
	}
}
