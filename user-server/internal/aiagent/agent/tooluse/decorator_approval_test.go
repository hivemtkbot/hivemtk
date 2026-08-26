package tooluse

import (
	"context"
	"errors"
	"testing"
)

func TestIsColdOutreachTool(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		// 正例
		{"reach.telegram.dm", true},
		{"reach.dm.send", true},
		{"reach.proactive.email", true},
		{"reach.batch_send.sms", true},
		{"proactive.outreach", true},
		{"batch_send", true},
		// 反例
		{"", false},
		{"reach.telegram.send", false}, // 既有热触达工具名，行为必须不变
		{"reach.email.send", false},
		{"reach.wecom.send", false},
		{"customer.search", false},
		{"knowledge.list_kb", false},
		{"order.lookup", false},
	}
	for _, c := range cases {
		if got := IsColdOutreachTool(c.name); got != c.want {
			t.Errorf("IsColdOutreachTool(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

type fakeApprovalChecker struct {
	approved bool
	gotTool  string
	gotOwner string
}

func (f *fakeApprovalChecker) IsApproved(ctx context.Context, toolName, accountIDorOwnerKey string) bool {
	f.gotTool = toolName
	f.gotOwner = accountIDorOwnerKey
	return f.approved
}

type stubColdTool struct {
	BaseTool
	executed bool
}

func newStubColdTool() *stubColdTool {
	return &stubColdTool{BaseTool: BaseTool{
		NameVal:     "reach.telegram.dm",
		CategoryVal: CategoryReach,
	}}
}

func (s *stubColdTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	s.executed = true
	return SuccessResult(s.Name(), "ok"), nil
}

func TestWithApprovalDenied(t *testing.T) {
	tool := newStubColdTool()
	wrapped := WithApprovalChecker(tool, &fakeApprovalChecker{approved: false})
	ctx := WithToolContext(context.Background(), &ToolContext{CallerID: "u1"})

	result, err := wrapped.Execute(ctx, nil)
	if !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("err = %v, want ErrApprovalDenied", err)
	}
	if result.Success {
		t.Fatal("result.Success should be false on denial")
	}
	if tool.executed {
		t.Fatal("inner tool must not be executed when denied")
	}
}

func TestWithApprovalApproved(t *testing.T) {
	tool := newStubColdTool()
	fc := &fakeApprovalChecker{approved: true}
	wrapped := WithApprovalChecker(tool, fc)
	ctx := WithToolContext(context.Background(), &ToolContext{AgentID: "a9"})

	result, err := wrapped.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Success || !tool.executed {
		t.Fatal("inner tool should execute when approved")
	}
	if fc.gotTool != "reach.telegram.dm" || fc.gotOwner != "a9" {
		t.Fatalf("checker args = (%q, %q), want owner fallback to AgentID", fc.gotTool, fc.gotOwner)
	}
}

func TestWithApprovalNoWiringBackwardCompat(t *testing.T) {
	prev := globalApprovalChecker.Load()
	defer globalApprovalChecker.Store(prev)

	globalApprovalChecker.Store(nil) // 模拟未接线

	tool := newStubColdTool()
	wrapped := WithApproval(tool)
	result, err := wrapped.Execute(context.Background(), nil)
	if err != nil || !result.Success {
		t.Fatalf("unwired checker must allow execution, got err=%v", err)
	}
}

func TestSetGlobalApprovalChecker(t *testing.T) {
	prev := globalApprovalChecker.Load()
	defer globalApprovalChecker.Store(prev)

	fc := &fakeApprovalChecker{approved: false}
	SetGlobalApprovalChecker(fc)
	loaded := globalApprovalChecker.Load()
	if loaded == nil || (*loaded).IsApproved(context.Background(), "x", "") != false {
		t.Fatal("global checker not set")
	}

	SetGlobalApprovalChecker(nil) // 恢复默认放行
	if globalApprovalChecker.Load() != nil {
		t.Fatal("nil should reset to default-allow")
	}
}

func TestWarmToolBypassesApproval(t *testing.T) {
	denied := &fakeApprovalChecker{approved: false}
	warm := &stubColdTool{BaseTool: BaseTool{NameVal: "reach.weixin.send", CategoryVal: CategoryReach}}
	wrapped := WithApprovalChecker(warm, denied)

	if _, err := wrapped.Execute(context.Background(), nil); err != nil {
		t.Fatalf("warm tool must bypass approval gate, got err=%v", err)
	}
	if warm.executed == false {
		t.Fatal("warm tool should execute")
	}
}
