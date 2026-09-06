package tooluse

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestWhitelistPermissionChecker_DefaultAllow(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("defaultAllow=true 应放行，实际 %v", err)
	}

	c.SetDefaultAllow(false)
	err := c.Check(ctx, "any.tool", tc)
	if err == nil {
		t.Fatalf("defaultAllow=false 应拒绝")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("应返回 ErrPermissionDenied，实际 %v", err)
	}
}

func TestWhitelistPermissionChecker_GlobalWhitelist(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.AddGlobalWhitelist([]string{"global.tool1", "global.tool2"})
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	if err := c.Check(ctx, "global.tool1", tc); err != nil {
		t.Fatalf("全局白名单内应放行：%v", err)
	}
	if err := c.Check(ctx, "global.tool2", tc); err != nil {
		t.Fatalf("全局白名单内应放行：%v", err)
	}

	if err := c.Check(ctx, "other.tool", tc); err == nil {
		t.Fatalf("全局白名单外应拒绝")
	}
}

func TestWhitelistPermissionChecker_AgentWhitelist(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.SetAgentWhitelist("agent-1", []string{"customer.search", "customer.get"})
	ctx := context.Background()
	tc1 := &ToolContext{AgentID: "agent-1"}
	tc2 := &ToolContext{AgentID: "agent-2"}

	if err := c.Check(ctx, "customer.search", tc1); err != nil {
		t.Fatalf("agent-1 白名单内应放行：%v", err)
	}
	if err := c.Check(ctx, "customer.create", tc1); err == nil {
		t.Fatalf("agent-1 白名单外应拒绝")
	}
	if err := c.Check(ctx, "customer.search", tc2); err == nil {
		t.Fatalf("agent-2 未配置，严格模式应拒绝")
	}
}

func TestWhitelistPermissionChecker_SuperPermission(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{
		AgentID:     "admin-agent",
		Permissions: []string{"read", "write", "*"},
	}

	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("超级权限 * 应放行所有工具：%v", err)
	}
}

func TestWhitelistPermissionChecker_AddAgentTools(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("未配置应拒绝")
	}

	c.AddAgentTools("agent-1", []string{"tool.a"})
	if err := c.Check(ctx, "tool.a", tc); err != nil {
		t.Fatalf("追加后应放行：%v", err)
	}

	c.AddAgentTools("agent-1", []string{"tool.b"})
	if err := c.Check(ctx, "tool.b", tc); err != nil {
		t.Fatalf("二次追加后应放行：%v", err)
	}
	if err := c.Check(ctx, "tool.a", tc); err != nil {
		t.Fatalf("原工具应仍放行：%v", err)
	}
}

func TestWhitelistPermissionChecker_RemoveAgentWhitelist(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.SetAgentWhitelist("agent-1", []string{"tool.a"})
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	if err := c.Check(ctx, "tool.a", tc); err != nil {
		t.Fatalf("移除前应放行：%v", err)
	}

	c.RemoveAgentWhitelist("agent-1")
	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("移除后应走 defaultAllow 拒绝")
	}
}

func TestWhitelistPermissionChecker_SetAgentWhitelistOverride(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	c.SetAgentWhitelist("agent-1", []string{"tool.a", "tool.b"})
	c.SetAgentWhitelist("agent-1", []string{"tool.c"})

	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("覆盖后 tool.a 应被拒绝")
	}
	if err := c.Check(ctx, "tool.c", tc); err != nil {
		t.Fatalf("覆盖后 tool.c 应放行：%v", err)
	}
}

func TestWhitelistPermissionChecker_NilChecker(t *testing.T) {
	var c *WhitelistPermissionChecker
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("nil 检查器应放行：%v", err)
	}
}

func TestWhitelistPermissionChecker_NilToolContext(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.AddGlobalWhitelist([]string{"global.tool"})
	ctx := context.Background()

	if err := c.Check(ctx, "global.tool", nil); err != nil {
		t.Fatalf("tc=nil 全局白名单应放行：%v", err)
	}
	if err := c.Check(ctx, "other.tool", nil); err == nil {
		t.Fatalf("tc=nil 严格模式应拒绝")
	}
}

func TestWhitelistPermissionChecker_ConcurrentAccess(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.AddAgentTools("agent-1", []string{"tool"})
			c.Check(ctx, "tool", tc)
			c.ListAgentWhitelist("agent-1")
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Check(ctx, "tool", tc)
			c.ListConfiguredAgents()
			c.ListGlobalWhitelist()
		}()
	}
	wg.Wait()
}

func TestWhitelistPermissionChecker_ListOperations(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetAgentWhitelist("agent-1", []string{"tool.a", "tool.b"})
	c.SetAgentWhitelist("agent-2", []string{"tool.c"})
	c.AddGlobalWhitelist([]string{"global.tool"})

	agents := c.ListConfiguredAgents()
	if len(agents) != 2 {
		t.Fatalf("期望 2 个已配置 Agent，实际 %d", len(agents))
	}

	tools1 := c.ListAgentWhitelist("agent-1")
	if len(tools1) != 2 {
		t.Fatalf("期望 agent-1 有 2 个工具，实际 %d", len(tools1))
	}

	globalTools := c.ListGlobalWhitelist()
	if len(globalTools) != 1 {
		t.Fatalf("期望 1 个全局工具，实际 %d", len(globalTools))
	}
}

func TestWhitelistPermissionChecker_GetDefaultAllow(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	if !c.GetDefaultAllow() {
		t.Fatalf("默认应为 true")
	}
	c.SetDefaultAllow(false)
	if c.GetDefaultAllow() {
		t.Fatalf("设置后应为 false")
	}
}

func TestWhitelistPermissionChecker_EmptyAgentID(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{AgentID: ""}

	c.AddGlobalWhitelist([]string{"global.tool"})
	if err := c.Check(ctx, "global.tool", tc); err != nil {
		t.Fatalf("空 AgentID 全局白名单应放行：%v", err)
	}
	if err := c.Check(ctx, "other.tool", tc); err == nil {
		t.Fatalf("空 AgentID 严格模式应拒绝")
	}
}

func TestWhitelistPermissionChecker_SetAgentWhitelistEmpty(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.SetAgentWhitelist("agent-1", []string{"tool.a"})

	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	c.SetAgentWhitelist("agent-1", []string{})
	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("空切片设置后应走 defaultAllow 拒绝")
	}
}
