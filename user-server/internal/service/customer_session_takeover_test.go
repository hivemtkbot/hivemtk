package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
)

// ============================================================================
// 方向10：坐席实时聊天看板 - 接管/释放/切换 Service 单元测试
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §三
// ============================================================================

// TestTakeoverByAgent_Success 正常接管：AI 会话切到人工
func TestTakeoverByAgent_Success(t *testing.T) {
	svc := setupCustomerSessionService(t)

	// 准备：1 个 AI 状态会话
	sess, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_1",
		UserName:  "访客A",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 准备：1 个在线客服
	agent := &model.AgentStatus{AgentID: 101, AgentName: "客服甲", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	if err := svc.agentRepo.Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}

	// 接管
	err = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{
		SessionID: sess.ID,
		AgentID:   101,
		Reason:    "AI 答非所问",
	})
	if err != nil {
		t.Fatalf("TakeoverByAgent: %v", err)
	}

	// 验证
	got, _ := svc.GetSessionByID(context.Background(), sess.ID)
	if got.HandlerType != model.HandlerTypeHuman {
		t.Errorf("handler_type = %s, want human", got.HandlerType)
	}
	if got.Status != model.SessionStatusHumanHandling {
		t.Errorf("status = %s, want human_handling", got.Status)
	}
	if got.AgentID != 101 {
		t.Errorf("agent_id = %d, want 101", got.AgentID)
	}
	// 坐席活跃数 +1
	a, _ := svc.agentRepo.GetByAgentID(context.Background(), 101)
	if a.ActiveSessions != 1 {
		t.Errorf("agent active = %d, want 1", a.ActiveSessions)
	}
}

// TestTakeoverByAgent_OfflineAgent 离线坐席不允许接管
func TestTakeoverByAgent_OfflineAgent(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformWeb,
		AccountID: "acc_1",
		UserID:    "u_1",
	})
	offline := &model.AgentStatus{AgentID: 200, AgentName: "客服乙", Status: "offline", MaxSessions: 5}
	_ = svc.agentRepo.Create(context.Background(), offline)

	err := svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 200})
	if err == nil {
		t.Error("expected error for offline agent")
	}
}

// TestTakeoverByAgent_AgentFull 坐席已满不允许接管
func TestTakeoverByAgent_AgentFull(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	agent := &model.AgentStatus{AgentID: 300, AgentName: "客服丙", Status: "online", MaxSessions: 1, ActiveSessions: 1}
	_ = svc.agentRepo.Create(context.Background(), agent)

	err := svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 300})
	if err == nil {
		t.Error("expected error for full agent")
	}
}

// TestTakeoverByAgent_Idempotent 同坐席重复接管：幂等
func TestTakeoverByAgent_Idempotent(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	agent := &model.AgentStatus{AgentID: 400, AgentName: "客服丁", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), agent)

	// 第一次接管
	_ = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 400})
	a, _ := svc.agentRepo.GetByAgentID(context.Background(), 400)
	if a.ActiveSessions != 1 {
		t.Fatalf("after 1st takeover active = %d, want 1", a.ActiveSessions)
	}

	// 第二次接管：幂等，活跃数不应再 +1
	_ = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 400})
	a, _ = svc.agentRepo.GetByAgentID(context.Background(), 400)
	if a.ActiveSessions != 1 {
		t.Errorf("after 2nd takeover active = %d, want still 1 (idempotent)", a.ActiveSessions)
	}
}

// TestTakeoverByAgent_TakeoverFromAnother 从别的坐席接管：原坐席活跃数 -1
func TestTakeoverByAgent_TakeoverFromAnother(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	a1 := &model.AgentStatus{AgentID: 500, AgentName: "甲", Status: "online", MaxSessions: 5, ActiveSessions: 1}
	a2 := &model.AgentStatus{AgentID: 501, AgentName: "乙", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), a1)
	_ = svc.agentRepo.Create(context.Background(), a2)
	// 让会话先归属 a1
	_ = svc.sessionRepo.AssignAgent(context.Background(), sess.ID, 500, "甲")
	a1Got, _ := svc.agentRepo.GetByAgentID(context.Background(), 500)
	a1Got.ActiveSessions = 1
	_ = svc.agentRepo.Update(context.Background(), a1Got)

	// a2 接管
	if err := svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 501}); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	// a1 活跃 -1
	old, _ := svc.agentRepo.GetByAgentID(context.Background(), 500)
	if old.ActiveSessions != 0 {
		t.Errorf("a1 active = %d, want 0", old.ActiveSessions)
	}
	// a2 活跃 +1
	newA, _ := svc.agentRepo.GetByAgentID(context.Background(), 501)
	if newA.ActiveSessions != 1 {
		t.Errorf("a2 active = %d, want 1", newA.ActiveSessions)
	}
}

// TestReleaseToAI 释放回 AI
func TestReleaseToAI(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	agent := &model.AgentStatus{AgentID: 600, AgentName: "客服", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), agent)
	// 接管一次
	_ = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 600})

	// 释放
	if err := svc.ReleaseToAI(context.Background(), &ReleaseToAIRequest{SessionID: sess.ID, AgentID: 600}); err != nil {
		t.Fatalf("ReleaseToAI: %v", err)
	}
	got, _ := svc.GetSessionByID(context.Background(), sess.ID)
	if got.HandlerType != model.HandlerTypeAI {
		t.Errorf("handler = %s, want ai", got.HandlerType)
	}
	if got.Status != model.SessionStatusWaiting {
		t.Errorf("status = %s, want waiting", got.Status)
	}
	if got.AgentID != 0 {
		t.Errorf("agent_id = %d, want 0", got.AgentID)
	}
	a, _ := svc.agentRepo.GetByAgentID(context.Background(), 600)
	if a.ActiveSessions != 0 {
		t.Errorf("active = %d, want 0", a.ActiveSessions)
	}
}

// TestReleaseToAI_NotOwner 无权释放别人的会话
func TestReleaseToAI_NotOwner(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	a1 := &model.AgentStatus{AgentID: 700, AgentName: "甲", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	a2 := &model.AgentStatus{AgentID: 701, AgentName: "乙", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), a1)
	_ = svc.agentRepo.Create(context.Background(), a2)
	_ = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 700})

	// a2 想释放 → 拒绝
	err := svc.ReleaseToAI(context.Background(), &ReleaseToAIRequest{SessionID: sess.ID, AgentID: 701})
	if err == nil {
		t.Error("expected permission denied")
	}
}

// TestSwitchHandler_Human 切到人工
func TestSwitchHandler_Human(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	agent := &model.AgentStatus{AgentID: 800, AgentName: "客服", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), agent)

	err := svc.SwitchHandler(context.Background(), &SwitchHandlerRequest{
		SessionID:   sess.ID,
		AgentID:     800,
		HandlerType: model.HandlerTypeHuman,
		Reason:      "客户要求转人工",
	})
	if err != nil {
		t.Fatalf("SwitchHandler: %v", err)
	}
	got, _ := svc.GetSessionByID(context.Background(), sess.ID)
	if got.HandlerType != model.HandlerTypeHuman {
		t.Errorf("handler = %s, want human", got.HandlerType)
	}
}

// TestSwitchHandler_AI 切回 AI（agent_id 自动从会话读取）
func TestSwitchHandler_AI(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	agent := &model.AgentStatus{AgentID: 900, AgentName: "客服", Status: "online", MaxSessions: 5, ActiveSessions: 0}
	_ = svc.agentRepo.Create(context.Background(), agent)
	_ = svc.TakeoverByAgent(context.Background(), &TakeoverRequest{SessionID: sess.ID, AgentID: 900})

	// AgentID 留空，让 service 从会话读
	err := svc.SwitchHandler(context.Background(), &SwitchHandlerRequest{
		SessionID:   sess.ID,
		AgentID:     0,
		HandlerType: model.HandlerTypeAI,
	})
	if err != nil {
		t.Fatalf("SwitchHandler: %v", err)
	}
	got, _ := svc.GetSessionByID(context.Background(), sess.ID)
	if got.HandlerType != model.HandlerTypeAI {
		t.Errorf("handler = %s, want ai", got.HandlerType)
	}
}

// TestSwitchHandler_AI_NoAgent 会话尚未分配坐席就切 AI → 报错
func TestSwitchHandler_AI_NoAgent(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	// 未接管直接切 AI
	err := svc.SwitchHandler(context.Background(), &SwitchHandlerRequest{
		SessionID:   sess.ID,
		HandlerType: model.HandlerTypeAI,
	})
	if err == nil {
		t.Error("expected error: no agent assigned")
	}
}

// TestSwitchHandler_InvalidType 非法 handler_type
func TestSwitchHandler_InvalidType(t *testing.T) {
	svc := setupCustomerSessionService(t)

	sess, _ := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformWeb, AccountID: "acc_1", UserID: "u_1",
	})
	err := svc.SwitchHandler(context.Background(), &SwitchHandlerRequest{
		SessionID:   sess.ID,
		HandlerType: "robot", // 非法
	})
	if err == nil {
		t.Error("expected error for invalid handler type")
	}
}
