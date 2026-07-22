package service

import (
	"errors"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// 客服状态服务（agent_status.go）
// ----------------------------------------------------------------------------
// 从 customer_session.go 拆分（2026-07-22 方向C）。
// 职责：客服在线/离线、活跃会话数、容量配置。
// 文档：docs/企业级架构优化/坐席实时聊天看板.md
// ============================================================================

// AgentStatusService 客服状态服务
type AgentStatusService struct {
	agentRepo *repository.AgentStatusRepository
}

// NewAgentStatusService 创建客服状态服务实例
func NewAgentStatusService() *AgentStatusService {
	return &AgentStatusService{
		agentRepo: repository.NewAgentStatusRepository(),
	}
}

// CreateAgentRequest 创建客服请求
type CreateAgentRequest struct {
	AgentID     uint   `json:"agent_id" binding:"required"`
	AgentName   string `json:"agent_name" binding:"required"`
	MaxSessions int    `json:"max_sessions"`
}

// CreateAgent 创建客服状态记录
func (s *AgentStatusService) CreateAgent(req *CreateAgentRequest) (*model.AgentStatus, error) {
	agent := &model.AgentStatus{
		AgentID:     req.AgentID,
		AgentName:   req.AgentName,
		Status:      "offline",
		MaxSessions: req.MaxSessions,
	}
	if agent.MaxSessions == 0 {
		agent.MaxSessions = 5
	}

	if err := s.agentRepo.Create(agent); err != nil {
		return nil, err
	}

	return agent, nil
}

// GetAgentStatus 获取客服状态
func (s *AgentStatusService) GetAgentStatus(agentID uint) (*model.AgentStatus, error) {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

// GetOnlineAgents 获取在线客服列表
func (s *AgentStatusService) GetOnlineAgents() ([]*model.AgentStatus, error) {
	return s.agentRepo.GetOnlineAgents()
}

// ListAllAgents 列出全部客服（不分在线/离线），用于客服监管控制台
func (s *AgentStatusService) ListAllAgents() ([]*model.AgentStatus, error) {
	return s.agentRepo.ListAllAgents()
}

// UpdateAgentStatus 更新客服状态
func (s *AgentStatusService) UpdateAgentStatus(agentID uint, status string) error {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return err
	}
	_ = agent

	// 检查状态变更合法性
	if agent.Status == "offline" && status != "online" {
		return errors.New("客服离线时只能切换到在线状态")
	}

	return s.agentRepo.UpdateStatus(agentID, status)
}

// GoOnline 客服上线
func (s *AgentStatusService) GoOnline(agentID uint) error {
	return s.UpdateAgentStatus(agentID, "online")
}

// GoOffline 客服下线
func (s *AgentStatusService) GoOffline(agentID uint) error {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return err
	}
	_ = agent

	// 检查是否有未完成的会话
	if agent.ActiveSessions > 0 {
		return errors.New("还有未完成的会话，请先处理或转接")
	}

	return s.agentRepo.UpdateStatus(agentID, "offline")
}

// GetAgentSessions 获取客服的活跃会话
func (s *AgentStatusService) GetAgentSessions(agentID uint) ([]*model.CustomerSession, error) {
	sessionRepo := repository.NewCustomerSessionRepository()
	return sessionRepo.GetAgentSessions(agentID)
}
