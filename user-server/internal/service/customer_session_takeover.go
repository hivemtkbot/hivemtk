package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/websocket"
)


// TakeoverRequest 坐席接管请求
type TakeoverRequest struct {
	SessionID uint   `json:"session_id" binding:"required"`
	AgentID   uint   `json:"agent_id" binding:"required"`
	Reason    string `json:"reason"` 
}

// TakeoverByAgent 坐席接管 AI 会话
//
// 行为：
//  1. 把会话 handler_type 切到 human、status 切到 human_handling
//  2. 记录接管人 AgentID（若尚未分配则 AssignAgent 一次）
//  3. 给该会话加 Redis 人工接管锁（InboxIngressService 收到新消息时绕过 AI）
//  4. 通过 WebSocket 通知前端会话更新
//
// 幂等：同一坐席重复接管直接返回成功（不重复扣活跃数）
func (s *CustomerSessionService) TakeoverByAgent(ctx context.Context, req *TakeoverRequest) error {
	if req.SessionID == 0 || req.AgentID == 0 {
		return errors.New("session_id and agent_id required")
	}
	session, err := s.sessionRepo.GetByID(ctx, req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}

	agent, err := s.agentRepo.GetByAgentID(ctx, req.AgentID)
	if err != nil || agent == nil {
		return errors.New("坐席不存在")
	}
	if agent.Status == "offline" {
		return errors.New("坐席已离线，无法接管")
	}
	if agent.ActiveSessions >= agent.MaxSessions {
		return errors.New("坐席会话已满")
	}

	if session.AgentID == req.AgentID {
		session.HandlerType = model.HandlerTypeHuman
		session.Status = model.SessionStatusHumanHandling
		now := time.Now()
		session.LastMessageAt = &now
		if err := s.sessionRepo.Update(ctx, session); err != nil {
			return err
		}
		_ = s.lockHumanSession(ctx, session.SessionID, req.Reason)
		_ = s.notifySessionUpdate(ctx, session, "handler_changed", "human")
		return nil
	}

	if session.AgentID > 0 {
		_ = s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
	}
	if err := s.sessionRepo.AssignAgent(ctx, req.SessionID, req.AgentID, agent.AgentName); err != nil {
		return err
	}
	_ = s.agentRepo.IncrementActiveSessions(ctx, req.AgentID)

	updated, err := s.sessionRepo.GetByID(ctx, req.SessionID)
	if err != nil {
		return err
	}
	updated.HandlerType = model.HandlerTypeHuman
	updated.Status = model.SessionStatusHumanHandling
	now := time.Now()
	updated.LastMessageAt = &now
	if err := s.sessionRepo.Update(ctx, updated); err != nil {
		return err
	}
	_ = s.lockHumanSession(ctx, updated.SessionID, req.Reason)
	_ = s.notifySessionUpdate(ctx, updated, "handler_changed", "human")
	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id": updated.SessionID,
		"handler":    "human",
		"reason":     "客服已接管，正在为您服务",
	}, updated.SessionID)
	return nil
}

// ReleaseToAIRequest 释放回 AI 请求
type ReleaseToAIRequest struct {
	SessionID uint `json:"session_id" binding:"required"`
	AgentID   uint `json:"agent_id" binding:"required"`
}

// ReleaseToAI 坐席释放会话回 AI 托管
//
// 行为：
//  1. 把 handler_type 切回 ai、status 切回 waiting
//  2. 解 Redis 人工锁（InboxIngressService 后续消息会重新走 AI 路由）
//  3. 坐席活跃数 -1
//  4. 推 WebSocket 给坐席与访客
//
// 仅当会话原本属于该坐席才允许释放（防止误操作别人会话）
func (s *CustomerSessionService) ReleaseToAI(ctx context.Context, req *ReleaseToAIRequest) error {
	if req.SessionID == 0 || req.AgentID == 0 {
		return errors.New("session_id and agent_id required")
	}
	session, err := s.sessionRepo.GetByID(ctx, req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}
	if session.AgentID != req.AgentID {
		return errors.New("无权操作：会话不属于该坐席")
	}

	session.HandlerType = model.HandlerTypeAI
	session.Status = model.SessionStatusWaiting
	session.AgentID = 0
	session.AgentName = ""
	now := time.Now()
	session.LastMessageAt = &now
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return err
	}
	_ = s.agentRepo.DecrementActiveSessions(ctx, req.AgentID)
	_ = s.unlockHumanSession(ctx, session.SessionID)
	_ = s.notifySessionUpdate(ctx, session, "handler_changed", "ai")
	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id": session.SessionID,
		"handler":    "ai",
		"reason":     "已切回 AI 托管，请稍候",
	}, session.SessionID)
	return nil
}

// SwitchHandlerRequest AI/人工切换请求（统一接口）
type SwitchHandlerRequest struct {
	SessionID   uint              `json:"session_id" binding:"required"`
	AgentID     uint              `json:"agent_id"`
	HandlerType model.HandlerType `json:"handler_type" binding:"required"` 
	Reason      string            `json:"reason"`
}

// SwitchHandler 通用 AI/人工切换（前端按钮只调一个接口）
//
// 委派给 TakeoverByAgent / ReleaseToAI，避免上层维护两条调用路径。
func (s *CustomerSessionService) SwitchHandler(ctx context.Context, req *SwitchHandlerRequest) error {
	if req.SessionID == 0 {
		return errors.New("session_id required")
	}
	switch req.HandlerType {
	case model.HandlerTypeHuman:
		if req.AgentID == 0 {
			return errors.New("切人工时 agent_id required")
		}
		return s.TakeoverByAgent(ctx, &TakeoverRequest{
			SessionID: req.SessionID,
			AgentID:   req.AgentID,
			Reason:    req.Reason,
		})
	case model.HandlerTypeAI:
		if req.AgentID == 0 {
			sess, err := s.sessionRepo.GetByID(ctx, req.SessionID)
			if err != nil {
				return errors.New("会话不存在")
			}
			req.AgentID = sess.AgentID
			if req.AgentID == 0 {
				return errors.New("会话尚未分配坐席，无需切回 AI")
			}
		}
		return s.ReleaseToAI(ctx, &ReleaseToAIRequest{
			SessionID: req.SessionID,
			AgentID:   req.AgentID,
		})
	default:
		return fmt.Errorf("invalid handler_type: %s", req.HandlerType)
	}
}

// LockHumanSession 锁定会话为人工接管（暴露给 controller，转接/投诉升级时调用）
func (s *CustomerSessionService) LockHumanSession(ctx context.Context, sessionID, reason string) error {
	return s.lockHumanSession(ctx, sessionID, reason)
}

// UnlockHumanSession 解锁人工接管锁
func (s *CustomerSessionService) UnlockHumanSession(ctx context.Context, sessionID string) error {
	return s.unlockHumanSession(ctx, sessionID)
}

// lockHumanSession 内部：写 Redis 人工接管锁
func (s *CustomerSessionService) lockHumanSession(ctx context.Context, sessionID, reason string) error {
	svc := NewInboxIngressServiceWithDB(nil, nil)
	return svc.LockSessionForHuman(ctx, sessionID, reason)
}

// unlockHumanSession 内部：解 Redis 人工接管锁
func (s *CustomerSessionService) unlockHumanSession(ctx context.Context, sessionID string) error {
	svc := NewInboxIngressServiceWithDB(nil, nil)
	return svc.UnlockSessionForHuman(ctx, sessionID)
}

// notifySessionUpdate 内部：推送会话状态变更给前端
func (s *CustomerSessionService) notifySessionUpdate(ctx context.Context, session *model.CustomerSession, event, handler string) error {
	agentID := strconv.FormatUint(uint64(session.AgentID), 10)
	return websocket.NotifySessionUpdate(agentID, map[string]any{
		"session_id":   session.SessionID,
		"handler_type": handler,
		"event":        event,
		"status":       session.Status,
		"updated_at":   time.Now().Unix(),
	})
}

