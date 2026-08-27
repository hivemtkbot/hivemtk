package tooluse

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type stubTool struct {
	name string
	category ToolCategory
	exec func(ctx context.Context, args map[string]any) (ToolResult, error)
}

func (s *stubTool) Name() string               { return s.name }
func (s *stubTool) Category() ToolCategory     { return s.category }
func (s *stubTool) Description() string        { return s.name }
func (s *stubTool) Parameters() ToolParameters { return ToolParameters{} }
func (s *stubTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if s.exec != nil {
		return s.exec(ctx, args)
	}
	return ToolResult{Success: true, ToolName: s.name, Data: map[string]any{"executed": true}}, nil
}

type fixedDNC struct {
	blocked atomic.Bool
}

func (f *fixedDNC) IsBlocked(ctx context.Context, oneID string) bool {
	return f.blocked.Load()
}

func TestDNC_NilResolverAllowsAll(t *testing.T) {
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuard(inner)
	r, err := g.Execute(context.Background(), map[string]any{"one_id": "u1"})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !r.Success {
		t.Fatalf("expected OK, got %+v", r)
	}
}

func TestDNC_NotColdOutreachSkips(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(true)
	inner := &stubTool{name: "knowledge.search"}
	g := WithDNCGuardResolver(inner, resolver)
	r, err := g.Execute(context.Background(), map[string]any{"one_id": "u1"})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !r.Success {
		t.Fatalf("expected OK, got %+v", r)
	}
}

func TestDNC_BlockedDenies(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(true)
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuardResolver(inner, resolver)
	_, err := g.Execute(context.Background(), map[string]any{"one_id": "u1"})
	if err == nil || !errors.Is(err, ErrDNCBlocked) {
		t.Fatalf("expected ErrDNCBlocked, got %v", err)
	}
}

func TestDNC_NotBlockedAllows(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(false)
	inner := &stubTool{name: "reach.batch_send.email"}
	g := WithDNCGuardResolver(inner, resolver)
	r, err := g.Execute(context.Background(), map[string]any{"one_id": "u2"})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !r.Success {
		t.Fatalf("expected OK, got %+v", r)
	}
}

func TestDNC_PrefersToolContextCustomerID(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(true)
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuardResolver(inner, resolver)
	ctx := WithToolContext(context.Background(), &ToolContext{CustomerID: "ctx-1"})
	r, err := g.Execute(ctx, map[string]any{"one_id": "args-2"})
	if err == nil || !errors.Is(err, ErrDNCBlocked) {
		t.Fatalf("expected ErrDNCBlocked from ctx CustomerID, got err=%v r=%+v", err, r)
	}
}

func TestDNC_PhoneFallbackNormalization(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(true)
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuardResolver(inner, resolver)
	r, err := g.Execute(context.Background(), map[string]any{"phone": "13800138000"})
	if err == nil || !errors.Is(err, ErrDNCBlocked) {
		t.Fatalf("expected ErrDNCBlocked via phone fallback, got err=%v r=%+v", err, r)
	}
}

func TestDNC_NoOneIDAllows(t *testing.T) {
	resolver := &fixedDNC{}
	resolver.blocked.Store(true)
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuardResolver(inner, resolver)
	r, err := g.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("expected nil err (无 oneID 放行), got %v", err)
	}
	if !r.Success {
		t.Fatalf("expected OK, got %+v", r)
	}
}

func TestDNC_GlobalResolver(t *testing.T) {
	r := &fixedDNC{}
	r.blocked.Store(true)
	prev := globalDNCResolver
	SetGlobalDNCResolver(r)
	defer SetGlobalDNCResolver(prev)

	inner := &stubTool{name: "reach.dm.telegram"}
	g := WithDNCGuard(inner)
	_, err := g.Execute(context.Background(), map[string]any{"one_id": "u3"})
	if err == nil || !errors.Is(err, ErrDNCBlocked) {
		t.Fatalf("expected ErrDNCBlocked via global resolver, got %v", err)
	}
}

func TestDNC_ErrorMessageContainsContext(t *testing.T) {
	r := &fixedDNC{}
	r.blocked.Store(true)
	inner := &stubTool{name: "reach.proactive.whatsapp"}
	g := WithDNCGuardResolver(inner, r)
	_, err := g.Execute(context.Background(), map[string]any{"one_id": "u9"})
	if err == nil {
		t.Fatal("expected err")
	}
	if !strings.Contains(err.Error(), "reach.proactive.whatsapp") || !strings.Contains(err.Error(), "u9") {
		t.Fatalf("error should contain tool name and one_id, got %q", err.Error())
	}
}