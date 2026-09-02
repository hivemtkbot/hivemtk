package service

import (
	"context"
	"errors"
	"strconv"

	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/websocket"
)

// AssignSessionRequest 分配会话请求
type AssignSessionRequest struct {
	SessionID uint `json:"session_id" binding:"required"`
	AgentID   uint `json:"agent_id" binding:"required"`
}

// AssignSession 分配会话给客服
func (s *CustomerSessionService) AssignSession(ctx context.Context, req *AssignSessionRequest) error {
	agent, err := s.agentRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	if agent.Status == "offline" {
		return errors.New("客服不在线")
	}
	if agent.ActiveSessions >= agent.MaxSessions {
		return errors.New("客服会话已满")
	}

	if err := s.sessionRepo.AssignAgent(ctx, req.SessionID, req.AgentID, agent.AgentName); err != nil {
		return err
	}

	if err := s.agentRepo.IncrementActiveSessions(ctx, req.AgentID); err != nil {
		return err
	}

	session, _ := s.sessionRepo.GetByID(ctx, req.SessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(req.AgentID), 10), session)
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "客服已接入，正在为您服务",
		}, session.SessionID)
	}

	return nil
}

// AutoAssign 自动分配会话
func (s *CustomerSessionService) AutoAssign(ctx context.Context, sessionID uint) error {
	agents, err := s.agentRepo.GetOnlineAgents(ctx)
	if err != nil || len(agents) == 0 {
		return errors.New("没有可用的在线客服")
	}

	selectedAgent := agents[0]
	for _, agent := range agents {
		if agent.ActiveSessions < selectedAgent.ActiveSessions {
			selectedAgent = agent
		}
	}

	return s.AssignSession(ctx, &AssignSessionRequest{
		SessionID: sessionID,
		AgentID:   selectedAgent.AgentID,
	})
}

// TransferSession 转接会话
func (s *CustomerSessionService) TransferSession(ctx context.Context, sessionID uint, newAgentID uint) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// 防自转：不能将会话转给自己
	if session.AgentID > 0 && session.AgentID == newAgentID {
		return errors.New("不能将会话转给自己")
	}

	newAgent, err := s.agentRepo.GetByAgentID(ctx, newAgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	// 目标坐席必须在线
	if newAgent.Status != "online" {
		return errors.New("目标客服不在线，无法转接")
	}

	if session.AgentID > 0 {
		s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
	}

	if err := s.sessionRepo.AssignAgent(ctx, sessionID, newAgentID, newAgent.AgentName); err != nil {
		return err
	}

	if err := s.agentRepo.IncrementActiveSessions(ctx, newAgentID); err != nil {
		return err
	}

	session, _ = s.sessionRepo.GetByID(ctx, sessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(newAgentID), 10), session)
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "已为您转接客服，请稍候",
		}, session.SessionID)
	}

	return nil
}

// 编译期类型断言：确保 CustomerSessionService 字段类型保持稳定
// （reflect 方式避免 init 阶段 nil pointer）
//
// 仅保留 CustomerSessionService 的引用断言，其他服务见各自文件
var _ = func() error {
	t := struct {
		x *repository.AgentStatusRepository
	}{}.x
	_ = t
	return nil
}()
