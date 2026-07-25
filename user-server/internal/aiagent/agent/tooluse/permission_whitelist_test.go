package tooluse

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// ============================================================================
// WhitelistPermissionChecker 单元测试（P2-8）
// ============================================================================
// 覆盖场景：
//   1. 默认放行策略（defaultAllow=true/false）
//   2. 全局白名单
//   3. Agent 维度白名单
//   4. 超级权限 "*"
//   5. 运行时更新（SetAgentWhitelist / AddAgentTools / RemoveAgentWhitelist）
//   6. nil 检查器
//   7. 并发安全
// ============================================================================

func TestWhitelistPermissionChecker_DefaultAllow(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	// 默认 defaultAllow=true，未配置的 Agent 放行
	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("defaultAllow=true 应放行，实际 %v", err)
	}

	// 切换为严格模式
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
	c.SetDefaultAllow(false) // 严格模式
	c.AddGlobalWhitelist([]string{"global.tool1", "global.tool2"})
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	// 全局白名单内的工具放行
	if err := c.Check(ctx, "global.tool1", tc); err != nil {
		t.Fatalf("全局白名单内应放行：%v", err)
	}
	if err := c.Check(ctx, "global.tool2", tc); err != nil {
		t.Fatalf("全局白名单内应放行：%v", err)
	}

	// 全局白名单外的工具拒绝
	if err := c.Check(ctx, "other.tool", tc); err == nil {
		t.Fatalf("全局白名单外应拒绝")
	}
}

func TestWhitelistPermissionChecker_AgentWhitelist(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false) // 严格模式
	c.SetAgentWhitelist("agent-1", []string{"customer.search", "customer.get"})
	ctx := context.Background()
	tc1 := &ToolContext{AgentID: "agent-1"}
	tc2 := &ToolContext{AgentID: "agent-2"}

	// agent-1 白名单内放行
	if err := c.Check(ctx, "customer.search", tc1); err != nil {
		t.Fatalf("agent-1 白名单内应放行：%v", err)
	}
	// agent-1 白名单外拒绝
	if err := c.Check(ctx, "customer.create", tc1); err == nil {
		t.Fatalf("agent-1 白名单外应拒绝")
	}
	// agent-2 未配置，严格模式拒绝
	if err := c.Check(ctx, "customer.search", tc2); err == nil {
		t.Fatalf("agent-2 未配置，严格模式应拒绝")
	}
}

func TestWhitelistPermissionChecker_SuperPermission(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false) // 严格模式
	ctx := context.Background()
	tc := &ToolContext{
		AgentID:     "admin-agent",
		Permissions: []string{"read", "write", "*"}, // 超级权限
	}

	// 超级权限放行所有工具
	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("超级权限 * 应放行所有工具：%v", err)
	}
}

func TestWhitelistPermissionChecker_AddAgentTools(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	// 初始无白名单，拒绝
	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("未配置应拒绝")
	}

	// 增量追加
	c.AddAgentTools("agent-1", []string{"tool.a"})
	if err := c.Check(ctx, "tool.a", tc); err != nil {
		t.Fatalf("追加后应放行：%v", err)
	}

	// 再追加
	c.AddAgentTools("agent-1", []string{"tool.b"})
	if err := c.Check(ctx, "tool.b", tc); err != nil {
		t.Fatalf("二次追加后应放行：%v", err)
	}
	// tool.a 仍然有效
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

	// 移除前放行
	if err := c.Check(ctx, "tool.a", tc); err != nil {
		t.Fatalf("移除前应放行：%v", err)
	}

	// 移除白名单
	c.RemoveAgentWhitelist("agent-1")
	// 移除后走 defaultAllow=false，拒绝
	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("移除后应走 defaultAllow 拒绝")
	}
}

func TestWhitelistPermissionChecker_SetAgentWhitelistOverride(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	// 初始设置
	c.SetAgentWhitelist("agent-1", []string{"tool.a", "tool.b"})
	// 覆盖式更新（只保留 tool.c）
	c.SetAgentWhitelist("agent-1", []string{"tool.c"})

	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("覆盖后 tool.a 应被拒绝")
	}
	if err := c.Check(ctx, "tool.c", tc); err != nil {
		t.Fatalf("覆盖后 tool.c 应放行：%v", err)
	}
}

func TestWhitelistPermissionChecker_NilChecker(t *testing.T) {
	var c *WhitelistPermissionChecker // nil
	ctx := context.Background()
	tc := &ToolContext{AgentID: "agent-1"}

	// nil 检查器放行（与 PermissionDecorator(nil) 行为一致）
	if err := c.Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("nil 检查器应放行：%v", err)
	}
}

func TestWhitelistPermissionChecker_NilToolContext(t *testing.T) {
	c := NewWhitelistPermissionChecker()
	c.SetDefaultAllow(false)
	c.AddGlobalWhitelist([]string{"global.tool"})
	ctx := context.Background()

	// tc=nil 时，走全局白名单 + defaultAllow
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
	// 并发写
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.AddAgentTools("agent-1", []string{"tool"})
			c.Check(ctx, "tool", tc)
			c.ListAgentWhitelist("agent-1")
		}(i)
	}
	// 并发读
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Check(ctx, "tool", tc)
			c.ListConfiguredAgents()
			c.ListGlobalWhitelist()
		}()
	}
	wg.Wait() // 不 panic 即并发安全
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
	tc := &ToolContext{AgentID: ""} // 空 AgentID

	// 空 AgentID 走全局白名单 + defaultAllow
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

	// 传入空切片等价于 RemoveAgentWhitelist
	c.SetAgentWhitelist("agent-1", []string{})
	if err := c.Check(ctx, "tool.a", tc); err == nil {
		t.Fatalf("空切片设置后应走 defaultAllow 拒绝")
	}
}
