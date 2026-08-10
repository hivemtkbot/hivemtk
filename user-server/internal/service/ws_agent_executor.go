package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/websocket"
)

// ============================================================================
// 坐席 WebSocket 会话操作执行器适配器（ws_agent_executor.go）
// ----------------------------------------------------------------------------
// 实现 websocket.AgentSessionExecutor 接口，把 WebSocket 上行的坐席操作
// 路由到 CustomerSessionService 既有的领域方法。
//
// 设计要点：
//  1. 五层架构：web 层定义接口（websocket.AgentSessionExecutor），service 层
//     提供实现，避免 websocket 包反向 import service（导致循环依赖）。
//  2. sessionID 字符串兼容：WebSocket 协议使用业务字段 session_id（sess_xxx），
//     而 service 多数方法接受数字主键。本适配器负责转换，调用方无需感知。
//  3. 越权防护：所有方法的 agentID 均来自 JWT 主体（已由 WSHandler 校验），
//     本适配器对转接/关闭等高危操作仍做二次校验，防止跨坐席误操作。
// ============================================================================

// WSAgentExecutor WebSocket 坐席会话操作执行器
//
// 通过 CustomerSessionService 实现 websocket.AgentSessionExecutor 接口。
type WSAgentExecutor struct {
	svc *CustomerSessionService
}

// NewWSAgentExecutor 创建执行器实例
func NewWSAgentExecutor(svc *CustomerSessionService) *WSAgentExecutor {
	return &WSAgentExecutor{svc: svc}
}

// 编译期接口实现断言
var _ websocket.AgentSessionExecutor = (*WSAgentExecutor)(nil)

// MarkSessionRead 标记会话内消息已读
func (e *WSAgentExecutor) MarkSessionRead(ctx context.Context, agentID uint, sessionID string) error {
	if sessionID == "" {
		return errors.New("session_id is empty")
	}
	if e.svc == nil || e.svc.messageRepo == nil {
		return errors.New("message repository unavailable")
	}
	return e.svc.messageRepo.MarkAsRead(ctx, sessionID, time.Now())
}

// TakeoverSession 坐席接管会话
//
// sessionID 可能是业务字符串（sess_xxx）或数字主键；适配器统一转换。
func (e *WSAgentExecutor) TakeoverSession(ctx context.Context, agentID uint, sessionID string, reason string) error {
	if sessionID == "" {
		return errors.New("session_id is empty")
	}
	id, err := e.resolveSessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	return e.svc.TakeoverByAgent(ctx, &TakeoverRequest{
		SessionID: id,
		AgentID:   agentID,
		Reason:    reason,
	})
}

// TransferSession 转接会话给目标坐席
func (e *WSAgentExecutor) TransferSession(ctx context.Context, fromAgentID uint, sessionID string, toAgentID uint) error {
	if sessionID == "" {
		return errors.New("session_id is empty")
	}
	if toAgentID == 0 {
		return errors.New("target_agent_id required")
	}
	id, err := e.resolveSessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	// 越权校验：仅当前归属坐席可发起转接
	sess, err := e.svc.GetSessionByID(ctx, id)
	if err != nil {
		return errors.New("会话不存在")
	}
	if sess.AgentID != fromAgentID {
		return errors.New("无权操作：会话不属于该坐席")
	}
	return e.svc.TransferSession(ctx, id, toAgentID)
}

// CloseSession 关闭会话
func (e *WSAgentExecutor) CloseSession(ctx context.Context, agentID uint, sessionID string) error {
	if sessionID == "" {
		return errors.New("session_id is empty")
	}
	id, err := e.resolveSessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	// 越权校验：会话已分配坐席时仅归属坐席可关闭；未分配则放行（管理员路径）
	sess, err := e.svc.GetSessionByID(ctx, id)
	if err != nil {
		return errors.New("会话不存在")
	}
	if sess.AgentID != 0 && sess.AgentID != agentID {
		return errors.New("无权操作：会话不属于该坐席")
	}
	return e.svc.UpdateSessionStatus(ctx, id, model.SessionStatusClosed)
}

// resolveSessionID 把 WebSocket 协议中的 session_id 字符串解析为数据库主键 ID
//
// 兼容两种格式：
//  1. 数字主键 ID（如 "123"）→ 直接返回
//  2. 业务字符串 sessionID（如 "sess_xxx"）→ 查表转换为数字 ID
func (e *WSAgentExecutor) resolveSessionID(ctx context.Context, raw string) (uint, error) {
	if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
		return uint(n), nil
	}
	sess, err := e.svc.sessionRepo.GetBySessionID(ctx, raw)
	if err != nil || sess == nil {
		return 0, errors.New("会话不存在")
	}
	return sess.ID, nil
}
