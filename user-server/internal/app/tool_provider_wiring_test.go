package app

import (
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

func TestApplyDisabledTools(t *testing.T) {
	reg := tooluse.NewToolRegistry()
	reg.MustRegister(&mockTool{name: "reach.sms.send", category: tooluse.CategoryReach})
	reg.MustRegister(&mockTool{name: "rag.search", category: tooluse.CategoryKnowledge})
	reg.MustRegister(&mockTool{name: "customer.search", category: tooluse.CategoryCustomer})

	removed := applyDisabledTools(reg, []string{"reach.sms.send", "not.registered", ""})
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if reg.Has("reach.sms.send") {
		t.Error("disabled tool should be unregistered")
	}
	if !reg.Has("rag.search") || !reg.Has("customer.search") {
		t.Error("other tools must remain")
	}

	if again := applyDisabledTools(reg, []string{"reach.sms.send"}); again != 0 {
		t.Errorf("re-disable should be a no-op, got %d", again)
	}
}

// 向后兼容：空列表/nil → 全量保留
func TestApplyDisabledTools_EmptyKeepsAll(t *testing.T) {
	reg := tooluse.NewToolRegistry()
	reg.MustRegister(newMockEchoTool("echo.a"))
	reg.MustRegister(newMockEchoTool("echo.b"))

	if n := applyDisabledTools(reg, nil); n != 0 {
		t.Errorf("nil list should remove nothing, got %d", n)
	}
	if n := applyDisabledTools(reg, []string{}); n != 0 {
		t.Errorf("empty list should remove nothing, got %d", n)
	}
	if n := applyDisabledTools(nil, []string{"x"}); n != 0 {
		t.Errorf("nil registry should be safe, got %d", n)
	}
	if reg.Count() != 2 {
		t.Errorf("all tools should remain, count = %d", reg.Count())
	}
}
