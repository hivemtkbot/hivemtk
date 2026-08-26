package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

// ============================================================================
// M5：assignment 策略显式化回归测试
// round_robin 原先被静默忽略（走默认 least_busy），manual/非法策略名同样静默。
// 修复后：round_robin 按 LastAssign 轮转；manual 返回明确错误；非法策略名报错。
// ============================================================================

func m5Session() *model.CustomerSession {
	return &model.CustomerSession{Platform: "douyin", Priority: 1}
}

func m5Agents() []AgentInfo {
	t0 := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	return []AgentInfo{
		{AgentID: 1, AgentName: "A1", Online: true, ActiveSess: 3, Capacity: 10, LastAssign: t0.Add(2 * time.Hour)},
		{AgentID: 2, AgentName: "A2", Online: true, ActiveSess: 9, Capacity: 10, LastAssign: t0},
		{AgentID: 3, AgentName: "A3", Online: true, ActiveSess: 1, Capacity: 10, LastAssign: t0.Add(1 * time.Hour)},
	}
}

// TestAssignWithStrategy_RoundRobinRotation round_robin 按 LastAssign 最早优先轮转
// （注意 A2 虽最忙，但 LastAssign 最早——round_robin 不看负载）
func TestAssignWithStrategy_RoundRobinRotation(t *testing.T) {
	svc := NewAssignmentService(nil)
	d, err := svc.AssignWithStrategy(context.Background(), m5Session(), m5Agents(), "round_robin")
	if err != nil {
		t.Fatalf("AssignWithStrategy(round_robin): %v", err)
	}
	if d.Strategy != string(StrategyRoundRobin) {
		t.Errorf("Strategy = %q, want round_robin", d.Strategy)
	}
	if d.AgentID != 2 {
		t.Errorf("AgentID = %d, want 2（LastAssign 最早的坐席）", d.AgentID)
	}
}

// TestAssignWithStrategy_RoundRobinSkipsFull 轮转跳过满载/离线坐席
func TestAssignWithStrategy_RoundRobinSkipsFull(t *testing.T) {
	svc := NewAssignmentService(nil)
	candidates := []AgentInfo{
		{AgentID: 1, Online: true, ActiveSess: 10, Capacity: 10, LastAssign: time.Time{}}, // 满载且最早
		{AgentID: 2, Online: true, ActiveSess: 0, Capacity: 5, LastAssign: time.Now()},
	}
	d, err := svc.AssignWithStrategy(context.Background(), m5Session(), candidates, "round_robin")
	if err != nil {
		t.Fatalf("AssignWithStrategy: %v", err)
	}
	if d.AgentID != 2 {
		t.Errorf("AgentID = %d, want 2（满载坐席应被跳过）", d.AgentID)
	}
}

// TestResolveAutoAssignMode_ManualExplicitError manual → 明确错误而非静默自动分配
func TestResolveAutoAssignMode_ManualExplicitError(t *testing.T) {
	if _, err := ResolveAutoAssignMode("manual"); !errors.Is(err, ErrManualAssignNotAllowed) {
		t.Errorf("manual 应返回 ErrManualAssignNotAllowed, got %v", err)
	}
}

// TestResolveAutoAssignMode_UnknownStrategyRejected 非法策略名 → 报错而非静默回退
func TestResolveAutoAssignMode_UnknownStrategyRejected(t *testing.T) {
	for _, bad := range []string{"random", "least-busy", "ROUND_ROBIN ", "auto_assign"} {
		if _, err := ResolveAutoAssignMode(bad); !errors.Is(err, ErrUnknownAssignStrategy) {
			t.Errorf("mode=%q 应返回 ErrUnknownAssignStrategy, got %v", bad, err)
		}
	}
}

// TestResolveAutoAssignMode_ValidModes 合法模式归一化：空串/auto 系列→默认算法，round_robin 原样
func TestResolveAutoAssignMode_ValidModes(t *testing.T) {
	cases := map[string]AssignmentStrategy{
		"":            StrategyLeastBusy,
		"least_busy":  StrategyLeastBusy,
		"skill_match": StrategyLeastBusy,
		"owner_route": StrategyLeastBusy,
		"round_robin": StrategyRoundRobin,
	}
	for in, want := range cases {
		got, err := ResolveAutoAssignMode(in)
		if err != nil {
			t.Errorf("mode=%q unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("mode=%q resolved=%q want=%q", in, got, want)
		}
	}
}

// TestAssignWithStrategy_DefaultUnchanged 默认策略行为不变（douyin 平台触发 social_media
// 技能标签 → 策略标记为 skill_match；无坐席具备技能时仍按最闲优先选活跃会话最少者）
func TestAssignWithStrategy_DefaultUnchanged(t *testing.T) {
	svc := NewAssignmentService(nil)
	d, err := svc.AssignWithStrategy(context.Background(), m5Session(), m5Agents(), "")
	if err != nil {
		t.Fatalf("AssignWithStrategy(\"\"): %v", err)
	}
	if d.Strategy != string(StrategySkillMatch) || d.AgentID != 3 {
		t.Errorf("got strategy=%s agent=%d, want skill_match/3", d.Strategy, d.AgentID)
	}
}
