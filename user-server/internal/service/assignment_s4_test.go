package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
)

// S-4 专属坐席定向路由单测（2026-08-26）
// 通过 AssignWithOwner 注入 owner，无需 DB；owner 解析（resolveOwnerAgentID）为
// DB 查询薄封装，回退语义由 ownerID<=0 分支覆盖。

func s4Agents() []AgentInfo {
	return []AgentInfo{
		{AgentID: 1, AgentName: "老销售", Online: true, ActiveSess: 4, Capacity: 5},
		{AgentID: 2, AgentName: "新坐席", Online: true, ActiveSess: 0, Capacity: 5},
	}
}

// Platform 取无技能映射的渠道，避免触发既有 skill_match 标签干扰断言
func s4Session() *model.CustomerSession {
	return &model.CustomerSession{Platform: "email", OneID: "one_1"}
}

// TestAssignWithOwner_OwnerOnlineRouted owner 在线且有容量 → 直接定向，忽略 least_busy
func TestAssignWithOwner_OwnerOnlineRouted(t *testing.T) {
	svc := NewAssignmentService(nil)
	d, err := svc.AssignWithOwner(context.Background(), s4Session(), s4Agents(), 1)
	if err != nil {
		t.Fatalf("AssignWithOwner: %v", err)
	}
	if d.AgentID != 1 {
		t.Errorf("owner 在线应被定向选中 agent 1, got %d", d.AgentID)
	}
	if d.Strategy != string(StrategyOwnerRoute) {
		t.Errorf("strategy 应为 owner_route, got %q", d.Strategy)
	}
}

// TestAssignWithOwner_OfflineOwnerFallsBack owner 离线/满载 → 回退现有算法（least_busy 选 2）
func TestAssignWithOwner_OfflineOwnerFallsBack(t *testing.T) {
	svc := NewAssignmentService(nil)

	// owner 离线
	candidates := []AgentInfo{
		{AgentID: 1, AgentName: "离线老销售", Online: false, ActiveSess: 0, Capacity: 5},
		{AgentID: 2, AgentName: "在线坐席", Online: true, ActiveSess: 0, Capacity: 5},
	}
	d, err := svc.AssignWithOwner(context.Background(), s4Session(), candidates, 1)
	if err != nil {
		t.Fatalf("owner 离线应回退而非报错: %v", err)
	}
	if d.AgentID != 2 || d.Strategy != string(StrategyLeastBusy) {
		t.Errorf("应回退 least_busy 选 agent 2, got agent=%d strategy=%s", d.AgentID, d.Strategy)
	}

	// owner 满载
	full := []AgentInfo{
		{AgentID: 1, AgentName: "满载老销售", Online: true, ActiveSess: 5, Capacity: 5},
		{AgentID: 2, AgentName: "空闲坐席", Online: true, ActiveSess: 0, Capacity: 5},
	}
	d, err = svc.AssignWithOwner(context.Background(), s4Session(), full, 1)
	if err != nil {
		t.Fatalf("owner 满载应回退而非报错: %v", err)
	}
	if d.AgentID != 2 {
		t.Errorf("owner 满载应选空闲坐席 2, got %d", d.AgentID)
	}
}

// TestAssignWithOwner_NoOwnerUnchanged ownerID=0 → 行为与原算法完全一致（向后兼容）
func TestAssignWithOwner_NoOwnerUnchanged(t *testing.T) {
	svc := NewAssignmentService(nil)
	d, err := svc.AssignWithOwner(context.Background(), s4Session(), s4Agents(), 0)
	if err != nil {
		t.Fatalf("AssignWithOwner: %v", err)
	}
	if d.Strategy != string(StrategyLeastBusy) {
		t.Errorf("无 owner 应回退 least_busy, got %q", d.Strategy)
	}
	if d.AgentID != 2 { // ActiveSess 少者优先
		t.Errorf("least_busy 应选 agent 2, got %d", d.AgentID)
	}
}

// TestAssignWithOwner_AllOfflineStillError 全员离线维持原错误语义
func TestAssignWithOwner_AllOfflineStillError(t *testing.T) {
	svc := NewAssignmentService(nil)
	candidates := []AgentInfo{{AgentID: 1, Online: false}}
	if _, err := svc.AssignWithOwner(context.Background(), s4Session(), candidates, 1); err == nil {
		t.Error("全员离线应返回错误")
	}
}
