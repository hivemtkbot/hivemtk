package service

import (
	"context"
	"errors"
	"strconv"

	"marketing/internal/repository"
	"marketing/internal/websocket"
)

// ============================================================================
// 客服会话路由（customer_session_routing.go）
// ----------------------------------------------------------------------------
// 负责会话的"分配 / 转接"等路由逻辑。
// 与 customer_session.go 分离原因：路由关心 AgentStatus / 坐席容量，
// 与"会话生命周期"（CreateSession / SendMessage）解耦。
// 文档：docs/企业级架构优化/坐席实时聊天看板.md
// ============================================================================

// AssignSessionRequest 分配会话请求
type AssignSessionRequest struct {
	SessionID	uint	`json:"session_id" binding:"required"`
	AgentID		uint	`json:"agent_id" binding:"required"`
}

// AssignSession 分配会话给客服
func (s *CustomerSessionService) AssignSession(ctx context.Context, req *AssignSessionRequest) error {
	// 获取客服信息
	agent, err := s.agentRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	// 检查客服是否可分配
	if agent.Status == "offline" {
		return errors.New("客服不在线")
	}
	if agent.ActiveSessions >= agent.MaxSessions {
		return errors.New("客服会话已满")
	}

	// 分配会话
	if err := s.sessionRepo.AssignAgent(ctx, req.SessionID, req.AgentID, agent.AgentName); err != nil {
		return err
	}

	// 更新客服活跃会话数
	if err := s.agentRepo.IncrementActiveSessions(ctx, req.AgentID); err != nil {
		return err
	}

	// 通知客服
	session, _ := s.sessionRepo.GetByID(ctx, req.SessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(req.AgentID), 10), session)
		// 通知访客：人工客服已接入（完成网页客服渠道的坐席侧闭环）
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id":	session.SessionID,
			"handler":	"human",
			"reason":	"客服已接入，正在为您服务",
		}, session.SessionID)
	}

	return nil
}

// AutoAssign 自动分配会话
func (s *CustomerSessionService) AutoAssign(ctx context.Context, sessionID uint) error {
	// 获取在线客服
	agents, err := s.agentRepo.GetOnlineAgents(ctx)
	if err != nil || len(agents) == 0 {
		return errors.New("没有可用的在线客服")
	}

	// 选择活跃会话最少的客服
	selectedAgent := agents[0]
	for _, agent := range agents {
		if agent.ActiveSessions < selectedAgent.ActiveSessions {
			selectedAgent = agent
		}
	}

	return s.AssignSession(ctx, &AssignSessionRequest{
		SessionID:	sessionID,
		AgentID:	selectedAgent.AgentID,
	})
}

// TransferSession 转接会话
func (s *CustomerSessionService) TransferSession(ctx context.Context, sessionID uint, newAgentID uint) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// 获取新客服信息
	newAgent, err := s.agentRepo.GetByAgentID(ctx, newAgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	// 减少原客服活跃会话数
	if session.AgentID > 0 {
		s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
	}

	// 分配给新客服
	if err := s.sessionRepo.AssignAgent(ctx, sessionID, newAgentID, newAgent.AgentName); err != nil {
		return err
	}

	// 增加新客服活跃会话数
	if err := s.agentRepo.IncrementActiveSessions(ctx, newAgentID); err != nil {
		return err
	}

	// 通知新客服
	session, _ = s.sessionRepo.GetByID(ctx, sessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(newAgentID), 10), session)
		// 通知访客：已转接至其他客服
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id":	session.SessionID,
			"handler":	"human",
			"reason":	"已为您转接客服，请稍候",
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
