package ragretrieval


import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMultiQueryGenerator_NilClientAutoDisabled(t *testing.T) {
	g := NewMultiQueryGenerator(nil, nil)
	if g.IsEnabled() {
		t.Error("nil chatClient should auto-disable")
	}
}

func TestMultiQueryGenerator_DisabledReturnsNilNoError(t *testing.T) {
	g := NewMultiQueryGenerator(nil, nil)
	out, err := g.Generate(context.Background(), "如何退货")
	if err != nil {
		t.Errorf("disabled should not error: %v", err)
	}
	if out != nil {
		t.Errorf("disabled should return nil, got=%v", out)
	}
}

func TestMultiQueryGenerator_Success(t *testing.T) {
	rawResp := `["退货流程是怎样的","如何申请退款","商品退换货政策"]`
	m := &mockLLMChatClient{resp: rawResp}
	g := NewMultiQueryGenerator(m, nil)
	out, err := g.Generate(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len=%d want=3", len(out))
	}
	expected := []string{"退货流程是怎样的", "如何申请退款", "商品退换货政策"}
	for i, v := range expected {
		if out[i] != v {
			t.Errorf("out[%d]=%q want=%q", i, out[i], v)
		}
	}
	if m.lastOpts.Temperature != 0.5 {
		t.Errorf("Temperature=%.2f want=0.5", m.lastOpts.Temperature)
	}
}

func TestMultiQueryGenerator_MarkdownWrapped(t *testing.T) {
	rawResp := "```json\n[\"变体1\",\"变体2\"]\n```"
	m := &mockLLMChatClient{resp: rawResp}
	g := NewMultiQueryGenerator(m, nil)
	out, err := g.Generate(context.Background(), "查询")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d want=2", len(out))
	}
}

func TestMultiQueryGenerator_LLMError(t *testing.T) {
	m := &mockLLMChatClient{err: errors.New("LLM unavailable")}
	g := NewMultiQueryGenerator(m, nil)
	_, err := g.Generate(context.Background(), "查询")
	if err == nil {
		t.Fatal("should propagate LLM error")
	}
	if !strings.Contains(err.Error(), "Multi-Query LLM 调用失败") {
		t.Errorf("error should wrap prefix, got=%v", err)
	}
}

func TestMultiQueryGenerator_InvalidJSON(t *testing.T) {
	m := &mockLLMChatClient{resp: "not a json at all"}
	g := NewMultiQueryGenerator(m, nil)
	_, err := g.Generate(context.Background(), "查询")
	if err == nil {
		t.Fatal("invalid JSON should error")
	}
}

func TestMultiQueryGenerator_EmptyArray(t *testing.T) {
	m := &mockLLMChatClient{resp: "[]"}
	g := NewMultiQueryGenerator(m, nil)
	_, err := g.Generate(context.Background(), "查询")
	if err == nil {
		t.Fatal("empty array should error")
	}
}

func TestMultiQueryGenerator_Deduplication(t *testing.T) {
	rawResp := `["退货流程","退货流程","如何退款"]`
	m := &mockLLMChatClient{resp: rawResp}
	g := NewMultiQueryGenerator(m, nil)
	out, err := g.Generate(context.Background(), "查询")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("after dedup len=%d want=2", len(out))
	}
}

func TestMultiQueryGenerator_EmptyStringsFiltered(t *testing.T) {
	rawResp := `["退货流程","", "  ", "如何退款"]`
	m := &mockLLMChatClient{resp: rawResp}
	g := NewMultiQueryGenerator(m, nil)
	out, err := g.Generate(context.Background(), "查询")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("after filtering empties len=%d want=2", len(out))
	}
}

func TestMultiQueryGenerator_CustomVariantN(t *testing.T) {
	m := &mockLLMChatClient{resp: `["v1","v2","v3","v4","v5"]`}
	g := NewMultiQueryGenerator(m, &MultiQueryGeneratorConfig{Enabled: true, VariantN: 5})
	out, err := g.Generate(context.Background(), "查询")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(m.lastPrompt, "5 个查询变体") {
		t.Errorf("prompt should mention 5 variants: %s", m.lastPrompt)
	}
	if len(out) != 5 {
		t.Errorf("len=%d want=5", len(out))
	}
}

func TestExtractJSONArray_Plain(t *testing.T) {
	out := extractJSONArray(`["a","b"]`)
	if out != `["a","b"]` {
		t.Errorf("got=%q", out)
	}
}

func TestExtractJSONArray_MarkdownWrapped(t *testing.T) {
	out := extractJSONArray("```json\n[\"a\"]\n```")
	if out != `["a"]` {
		t.Errorf("got=%q", out)
	}
}

func TestExtractJSONArray_NoArray(t *testing.T) {
	out := extractJSONArray("no json here")
	if out != "" {
		t.Errorf("should return empty, got=%q", out)
	}
}

func TestExtractJSONArray_ArrayWithPrefix(t *testing.T) {
	out := extractJSONArray(`好的，这是结果：["v1","v2"]`)
	if out != `["v1","v2"]` {
		t.Errorf("got=%q", out)
	}
}

func TestExtractJSONArray_UnclosedArray(t *testing.T) {
	out := extractJSONArray(`["v1","v2"`)
	if out != "" {
		t.Errorf("unclosed array should return empty, got=%q", out)
	}
}

