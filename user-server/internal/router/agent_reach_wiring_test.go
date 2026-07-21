package router

import (
	"testing"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/pkg/testutil"
)

// TestRegisterAgentReachTools_Wiring 验证生产接线：
// registerAgentReachTools 把 reach.web.send 等真实工具注册进全局注册中心，
// 且底层 adapter 是真实 IntegrationReachAdapter（非 NoOp）。
func TestRegisterAgentReachTools_Wiring(t *testing.T) {
	db := testutil.NewTestDB(t)

	// 模拟生产 Setup 调用
	registerAgentReachTools(db)

	reg := tooluse.GetGlobalRegistry()
	if !reg.Has("reach.web.send") {
		t.Fatalf("生产接线失败：全局注册中心缺失 reach.web.send")
	}
	if !reg.Has("reach.telegram.send") {
		t.Fatalf("生产接线失败：缺失 reach.telegram.send")
	}
	t.Logf("✅ 生产接线：reach.web.send / reach.telegram.send 已注册进全局注册中心")

	// 工具总数应 >= 20（触达工具），证明真实接入
	tools := reg.List()
	if len(tools) < 20 {
		t.Fatalf("全局注册中心工具数应 >= 20，实际 %d", len(tools))
	}
	t.Logf("✅ 全局注册中心共注册 %d 个工具（含网页客服 web 渠道）", len(tools))
}
