package tooluse

import (
	"context"
	"errors"
	"testing"
)

type approvalMockTool struct {
	name     string
	category ToolCategory
}

func (m *approvalMockTool) Name() string               { return m.name }
func (m *approvalMockTool) Category() ToolCategory     { return m.category }
func (m *approvalMockTool) Description() string        { return "test" }
func (m *approvalMockTool) Parameters() ToolParameters { return ToolParameters{} }
func (m *approvalMockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return SuccessResult(m.name, nil), nil
}

func TestIsColdOutreachTool(t *testing.T) {
	cases := []struct {
		name     string
		category ToolCategory
		want     bool
	}{

		{"reach.telegram.dm", CategoryReach, true},
		{"reach.dm.send", CategoryReach, true},
		{"reach.proactive.email", CategoryReach, true},
		{"reach.batch_send.sms", CategoryReach, true},
		{"reach.outbound.send", CategoryReach, true},
		{"reach.schedule.wecom", CategoryReach, true},
		{"reach.lead.dig", CategoryReach, true},

		{"reach.telegram.send", CategoryReach, false},
		{"reach.email.send", CategoryReach, false},
		{"reach.wecom.send", CategoryReach, false},
		{"reach.recall", CategoryReach, false},
		{"reach.health", CategoryReach, false},
		{"reach.account.list", CategoryReach, false},

		{"proactive.outreach", CategoryCustomer, false},
		{"batch_send", CategoryKnowledge, false},
		{"outbound.send", CategoryBusiness, false},

		{"", CategoryReach, false},
		{"", CategoryCustomer, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tb := &approvalMockTool{name: c.name, category: c.category}
			got := IsColdOutreachTool(tb)
			if got != c.want {
				t.Errorf("IsColdOutreachTool({name=%q, cat=%s}) = %v, want %v",
					c.name, c.category, got, c.want)
			}
		})
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

	globalApprovalChecker.Store(nil)

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

	SetGlobalApprovalChecker(nil)
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

func TestNonReachCategoryBypassesApproval(t *testing.T) {
	denied := &fakeApprovalChecker{approved: false}
	nonReach := &stubColdTool{BaseTool: BaseTool{NameVal: "batch_send.email", CategoryVal: CategoryKnowledge}}
	wrapped := WithApprovalChecker(nonReach, denied)

	if _, err := wrapped.Execute(context.Background(), nil); err != nil {
		t.Fatalf("non-reach category must bypass approval gate, got err=%v", err)
	}
	if !nonReach.executed {
		t.Fatal("non-reach tool should execute")
	}
}
