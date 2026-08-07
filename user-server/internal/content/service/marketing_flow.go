package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"marketing/internal/content/model"
	"marketing/internal/content/repository"
	reachmodel "marketing/internal/model"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/platform"
	cdprepo "marketing/internal/repository"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// smsSenderFunc 短信发送函数注入点。
// 因 internal/service 包已反向依赖 internal/content/service（sop_condition / sales_engine_adapters），
// 本包无法直接 import internal/service，故采用函数注入打破循环依赖。
// 由 internal/service 包在 init() 阶段调用 SetSmsSender 注入真实实现（基于 SmsService.SendSms）。
var smsSenderFunc func(phone, content string) error

// SetSmsSender 注入短信发送实现（由 internal/service 包 init 调用）。
func SetSmsSender(fn func(phone, content string) error) {
	smsSenderFunc = fn
}

// ============================================================================
// M2 运行时覆盖默认：marketing_workflow 资产读取注入点。
// 与 SetSmsSender 同理，本包无法反向依赖 internal/service，故由 internal/service
// 注入「读取生效中 marketing_workflow 资产」的函数，打破循环依赖。
// ============================================================================
var workflowAssetResolverFunc func(ctx context.Context) (json.RawMessage, bool)

// SetWorkflowAssetResolver 注入「读取生效中 marketing_workflow 资产」函数（由 internal/service 调用）。
func SetWorkflowAssetResolver(fn func(ctx context.Context) (json.RawMessage, bool)) {
	workflowAssetResolverFunc = fn
}

// MarketingWorkflow 资产结构（与 user-server 的 service.MarketingWorkflow 字段兼容的子集，
// 此处独立声明以避免反向依赖 internal/service）。
type MarketingWorkflow struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	Industry string                   `json:"industry"`
	Trigger  map[string]interface{}   `json:"trigger"`
	Steps    []map[string]interface{} `json:"steps"`
	KPI      map[string]interface{}   `json:"kpi"`
}

// ResolveActiveWorkflow 返回「生效中」的已购 marketing_workflow 资产（若存在）。
func ResolveActiveWorkflow(ctx context.Context) (*MarketingWorkflow, bool) {
	if workflowAssetResolverFunc == nil {
		return nil, false
	}
	raw, ok := workflowAssetResolverFunc(ctx)
	if !ok || len(raw) == 0 {
		return nil, false
	}
	var w MarketingWorkflow
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, false
	}
	return &w, true
}

// MarketingFlowService 营销流程服务
type MarketingFlowService struct {
	flowRepo         *repository.MarketingFlowRepository
	executionRepo    *repository.FlowExecutionRepository
	userTagRepo      cdprepo.UserTagRepository
	orderRepo        cdprepo.OrderRepository
	clueRepo         cdprepo.ClueRepository
	sessionRepo      *cdprepo.CustomerSessionRepository
	agentRepo        *cdprepo.AgentStatusRepository
	operationLogRepo cdprepo.OperationLogRepository
}

// NewMarketingFlowService 创建营销流程服务实例
func NewMarketingFlowService() *MarketingFlowService {
	return &MarketingFlowService{
		flowRepo:         repository.NewMarketingFlowRepository(),
		executionRepo:    repository.NewFlowExecutionRepository(),
		userTagRepo:      cdprepo.NewUserTagRepository(),
		orderRepo:        cdprepo.NewOrderRepository(),
		clueRepo:         cdprepo.NewClueRepository(),
		sessionRepo:      cdprepo.NewCustomerSessionRepository(),
		agentRepo:        cdprepo.NewAgentStatusRepository(),
		operationLogRepo: cdprepo.NewOperationLogRepository(),
	}
}

// NewMarketingFlowServiceWithDB 创建营销流程服务实例（带数据库连接，用于测试）
func NewMarketingFlowServiceWithDB(db *gorm.DB) *MarketingFlowService {
	return &MarketingFlowService{
		flowRepo:         repository.NewMarketingFlowRepositoryWithDB(db),
		executionRepo:    repository.NewFlowExecutionRepositoryWithDB(db),
		userTagRepo:      cdprepo.NewUserTagRepositoryWithDB(db),
		orderRepo:        cdprepo.NewOrderRepositoryWithDB(db),
		clueRepo:         cdprepo.NewClueRepositoryWithDB(db),
		sessionRepo:      cdprepo.NewCustomerSessionRepositoryWithDB(db),
		agentRepo:        cdprepo.NewAgentStatusRepositoryWithDB(db),
		operationLogRepo: cdprepo.NewOperationLogRepositoryWithDB(db),
	}
}

// CreateFlowRequest 创建流程请求
type CreateFlowRequest struct {
	Name          string                `json:"name" binding:"required"`
	Description   string                `json:"description"`
	TriggerType   model.TriggerType     `json:"trigger_type" binding:"required"`
	TriggerConfig map[string]any        `json:"trigger_config"`
	FlowData      *model.FlowDefinition `json:"flow_data"`
}

// CreateFlow 创建流程
func (s *MarketingFlowService) CreateFlow(createdBy uint, req *CreateFlowRequest) (*model.MarketingFlow, error) {
	flowData, err := json.Marshal(req.FlowData)
	if err != nil {
		return nil, fmt.Errorf("流程定义 JSON 序列化失败：%w", err)
	}

	// 验证流程定义是否有效
	if err := s.validateFlowDefinition(string(flowData)); err != nil {
		return nil, fmt.Errorf("流程定义无效：%w", err)
	}

	triggerConfig, _ := json.Marshal(req.TriggerConfig)

	flow := &model.MarketingFlow{
		Name:          req.Name,
		Description:   req.Description,
		Status:        model.FlowStatusDraft,
		TriggerType:   req.TriggerType,
		TriggerConfig: string(triggerConfig),
		FlowData:      string(flowData),
		Version:       1,
		CreatedBy:     createdBy,
	}

	if err := s.flowRepo.Create(flow); err != nil {
		return nil, err
	}

	return flow, nil
}

// GetFlowList 获取流程列表
func (s *MarketingFlowService) GetFlowList(page, pageSize int) ([]*model.MarketingFlow, int64, error) {
	return s.flowRepo.GetAll(page, pageSize)
}

// GetFlowByID 获取流程详情
func (s *MarketingFlowService) GetFlowByID(id uint) (*model.MarketingFlow, error) {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

// UpdateFlowRequest 更新流程请求
type UpdateFlowRequest struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	TriggerType   model.TriggerType     `json:"trigger_type"`
	TriggerConfig map[string]any        `json:"trigger_config"`
	FlowData      *model.FlowDefinition `json:"flow_data"`
}

// UpdateFlow 更新流程
func (s *MarketingFlowService) UpdateFlow(id uint, req *UpdateFlowRequest) (*model.MarketingFlow, error) {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		flow.Name = req.Name
	}
	if req.Description != "" {
		flow.Description = req.Description
	}
	if req.TriggerType != "" {
		flow.TriggerType = req.TriggerType
	}
	if req.TriggerConfig != nil {
		triggerConfig, _ := json.Marshal(req.TriggerConfig)
		flow.TriggerConfig = string(triggerConfig)
	}
	if req.FlowData != nil {
		flowData, _ := json.Marshal(req.FlowData)
		flow.FlowData = string(flowData)
	}

	flow.Version++
	if err := s.flowRepo.Update(flow); err != nil {
		return nil, err
	}

	return flow, nil
}

// DeleteFlow 删除流程
func (s *MarketingFlowService) DeleteFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.Delete(id)
}

// ActivateFlow 激活流程
func (s *MarketingFlowService) ActivateFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}

	// 验证流程定义是否有效
	if err := s.validateFlowDefinition(flow.FlowData); err != nil {
		return fmt.Errorf("流程定义无效：%w", err)
	}

	return s.flowRepo.UpdateStatus(id, model.FlowStatusActive)
}

// PauseFlow 暂停流程
func (s *MarketingFlowService) PauseFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.UpdateStatus(id, model.FlowStatusPaused)
}

// StopFlow 停止流程
func (s *MarketingFlowService) StopFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.UpdateStatus(id, model.FlowStatusInactive)
}

// validateFlowDefinition 验证流程定义
func (s *MarketingFlowService) validateFlowDefinition(flowData string) error {
	if flowData == "" {
		return errors.New("流程定义为空")
	}

	var def model.FlowDefinition
	if err := json.Unmarshal([]byte(flowData), &def); err != nil {
		return fmt.Errorf("流程定义 JSON 解析失败：%w", err)
	}

	if len(def.Nodes) == 0 {
		return errors.New("流程至少需要一个节点")
	}

	// 验证必须有触发器节点
	hasTrigger := false
	for _, node := range def.Nodes {
		if node.Type == "trigger" {
			hasTrigger = true
			break
		}
	}
	if !hasTrigger {
		return errors.New("流程必须有触发器节点")
	}

	return nil
}

// GetExecutionList 获取执行记录列表
func (s *MarketingFlowService) GetExecutionList(flowID uint, page, pageSize int) ([]*model.FlowExecution, int64, error) {
	return s.executionRepo.GetByFlowID(flowID, page, pageSize)
}

// GetExecutionStats 获取执行统计
func (s *MarketingFlowService) GetExecutionStats(flowID uint) (map[string]int64, error) {
	return s.executionRepo.GetStats(flowID)
}

// TriggerFlow 触发流程
func (s *MarketingFlowService) TriggerFlow(ctx context.Context, flow *model.MarketingFlow, triggerID, userID string, data map[string]any) error {
	// 检查流程状态
	if flow.Status != model.FlowStatusActive {
		return errors.New("流程未激活")
	}

	// 创建执行记录
	execution := &model.FlowExecution{
		FlowID:        flow.ID,
		TriggerID:     triggerID,
		UserID:        userID,
		Status:        "running",
		ExecutionData: "",
		StartedAt:     time.Now(),
	}

	if err := s.executionRepo.Create(execution); err != nil {
		return err
	}

	// 异步执行流程,使用 background ctx 避免上游取消中断长时间运行的流程
	go s.executeFlow(context.Background(), execution, flow, data)

	return nil
}

// executeFlow 执行流程
func (s *MarketingFlowService) executeFlow(ctx context.Context, execution *model.FlowExecution, flow *model.MarketingFlow, data map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			// 处理 panic
			execution.Status = "failed"
			execution.ErrorMessage = fmt.Sprintf("流程执行异常：%v", r)
			s.executionRepo.Update(execution)
		}
	}()

	// 解析流程定义
	var flowDef model.FlowDefinition
	if err := json.Unmarshal([]byte(flow.FlowData), &flowDef); err != nil {
		execution.Status = "failed"
		execution.ErrorMessage = fmt.Sprintf("流程定义解析失败：%v", err)
		s.executionRepo.Update(execution)
		return
	}

	// 执行流程节点
	executionData := data
	for _, node := range flowDef.Nodes {
		execution.CurrentNode = node.ID

		// 执行节点
		result, err := s.executeNode(ctx, node, execution.UserID, executionData)
		if err != nil {
			execution.Status = "failed"
			execution.ErrorMessage = fmt.Sprintf("节点 %s 执行失败：%v", node.Name, err)
			s.executionRepo.Update(execution)
			return
		}

		// 合并执行数据
		if result != nil {
			for k, v := range result {
				executionData[k] = v
			}
		}

		// 检查是否需要继续
		if len(node.NextNodes) == 0 {
			break
		}
	}

	// 完成执行
	execution.Status = "completed"
	execution.CompletedAt = func() *time.Time { t := time.Now(); return &t }()
	executionDataBytes, _ := json.Marshal(executionData)
	execution.ExecutionData = string(executionDataBytes)
	s.executionRepo.Update(execution)
}

// executeNode 执行单个节点
func (s *MarketingFlowService) executeNode(ctx context.Context, node model.FlowNode, userID string, data map[string]any) (map[string]any, error) {
	switch node.Type {
	case "trigger":
		// 触发器节点，无需执行
		return data, nil

	case "action":
		// 执行动作
		return s.executeAction(ctx, node, userID, data)

	case "condition":
		// 条件判断
		return s.evaluateCondition(node, data)

	case "delay":
		// 延迟节点
		return s.handleDelay(ctx, node)

	default:
		return nil, fmt.Errorf("未知的节点类型：%s", node.Type)
	}
}

// executeAction 执行动作
func (s *MarketingFlowService) executeAction(ctx context.Context, node model.FlowNode, userID string, data map[string]any) (map[string]any, error) {
	actionType, ok := node.Config["action_type"].(string)
	if !ok {
		return nil, errors.New("动作类型未指定")
	}

	switch model.ActionType(actionType) {
	case model.ActionTypeSendMessage:
		return s.sendActionSendMessage(ctx, node.Config, userID, data)
	case model.ActionTypeAddTag:
		return s.sendActionAddTag(ctx, node.Config, userID, data)
	case model.ActionTypeRemoveTag:
		return s.sendActionRemoveTag(ctx, node.Config, userID, data)
	case model.ActionTypeAssignAgent:
		return s.sendActionAssignAgent(ctx, node.Config, userID, data)
	case model.ActionTypeCreateTask:
		return s.sendActionCreateTask(ctx, node.Config, userID, data)
	case model.ActionTypeWebhook:
		return s.sendActionWebhook(ctx, node.Config, userID, data)
	case model.ActionTypeSendEmail:
		return s.sendActionSendEmail(ctx, node.Config, userID, data)
	case model.ActionTypeSendSms:
		return s.sendActionSendSms(ctx, node.Config, userID, data)
	case model.ActionTypeUpdateLead:
		return s.sendActionUpdateLead(ctx, node.Config, userID, data)
	case model.ActionTypeCreateOrder:
		return s.sendActionCreateOrder(ctx, node.Config, userID, data)
	default:
		return nil, fmt.Errorf("未知的动作类型：%s", actionType)
	}
}

// sendActionSendMessage 发送消息动作
func (s *MarketingFlowService) sendActionSendMessage(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	// Extract required config parameters
	platformName, _ := config["platform"].(string)
	accountID, _ := config["account_id"].(string)
	chatID, _ := config["chat_id"].(string)
	content, _ := config["content"].(string)

	// Validate required parameters
	if platformName == "" {
		return nil, errors.New("platform 未指定")
	}
	if accountID == "" {
		return nil, errors.New("account_id 未指定")
	}
	if chatID == "" {
		return nil, errors.New("chat_id 未指定")
	}
	if content == "" {
		return nil, errors.New("content 未指定")
	}

	// Get platform adapter from registry
	registry := platform.GetAdapterRegistry()
	adapter, err := registry.Get(reachmodel.Platform(platformName))
	if err != nil {
		return nil, fmt.Errorf("获取平台适配器失败：%w", err)
	}

	// Send message via adapter
	reply, err := adapter.SendMessage(accountID, chatID, content, nil)
	if err != nil {
		return nil, fmt.Errorf("发送消息失败：%w", err)
	}

	return map[string]any{
		"success":    true,
		"message_id": reply.MessageID,
		"platform":   platformName,
	}, nil
}

// sendActionAddTag 添加标签动作
func (s *MarketingFlowService) sendActionAddTag(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	// 从配置中提取标签列表
	tagsRaw, ok := config["tags"].([]any)
	if !ok {
		return nil, errors.New("tags 配置格式错误")
	}

	// 将 []interface{} 转换为 []string，并去重
	tagSet := make(map[string]bool)
	for _, tag := range tagsRaw {
		if tagName, ok := tag.(string); ok && tagName != "" {
			tagSet[tagName] = true
		}
	}

	if len(tagSet) == 0 {
		return nil, errors.New("没有有效的标签")
	}

	// 将 map 转换为 slice
	var tagNames []string
	for tagName := range tagSet {
		tagNames = append(tagNames, tagName)
	}

	// 调用仓库批量添加标签
	if err := s.userTagRepo.AddTags(ctx, userID, tagNames); err != nil {
		return nil, fmt.Errorf("添加标签失败：%w", err)
	}

	return map[string]any{
		"success":    true,
		"added_tags": tagNames,
		"user_id":    userID,
	}, nil
}

// sendActionRemoveTag 移除标签动作
// 配置参数：
//   - tags: []string 需要移除的标签名列表
func (s *MarketingFlowService) sendActionRemoveTag(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.userTagRepo == nil {
		return nil, errors.New("用户标签仓库未初始化")
	}
	if userID == "" {
		return nil, errors.New("user_id 未指定")
	}

	// 从配置中提取标签列表（兼容 []interface{} 与 []string 两种形式）
	var tagNames []string
	switch tagsRaw := config["tags"].(type) {
	case []any:
		seen := make(map[string]bool)
		for _, tag := range tagsRaw {
			if tagName, ok := tag.(string); ok && tagName != "" && !seen[tagName] {
				seen[tagName] = true
				tagNames = append(tagNames, tagName)
			}
		}
	case []string:
		seen := make(map[string]bool)
		for _, tagName := range tagsRaw {
			if tagName != "" && !seen[tagName] {
				seen[tagName] = true
				tagNames = append(tagNames, tagName)
			}
		}
	default:
		return nil, errors.New("tags 配置格式错误，应为字符串数组")
	}

	if len(tagNames) == 0 {
		return nil, errors.New("没有有效的待移除标签")
	}

	// 调用仓库批量移除标签
	if err := s.userTagRepo.RemoveTags(ctx, userID, tagNames); err != nil {
		return nil, fmt.Errorf("移除标签失败：%w", err)
	}

	return map[string]any{
		"success":      true,
		"removed_tags": tagNames,
		"user_id":      userID,
	}, nil
}

// sendActionAssignAgent 分配客服动作
// 配置参数：
//   - agent_id: float64 指定客服 ID（可选；若未指定则自动选择空闲客服）
//   - session_id: string 指定会话 ID（可选；若未指定则按 user_id 查找活跃会话）
//
// 优先级：显式 agent_id > 自动选择空闲客服
//
//	显式 session_id > 按 user_id 查找活跃会话
func (s *MarketingFlowService) sendActionAssignAgent(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.sessionRepo == nil {
		return nil, errors.New("会话仓库未初始化")
	}
	if s.agentRepo == nil {
		return nil, errors.New("客服状态仓库未初始化")
	}

	// 1. 解析目标会话
	var sessionID uint
	var sessionUserID string

	// 1.1 优先使用显式 session_id
	if sid, ok := config["session_id"].(string); ok && sid != "" {
		id, err := strconv.ParseUint(sid, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("session_id 格式无效：%w", err)
		}
		sessionID = uint(id)
	} else {
		// 1.2 其次使用 data 中的 session_id
		if sidRaw, ok := data["session_id"]; ok {
			switch v := sidRaw.(type) {
			case string:
				if id, err := strconv.ParseUint(v, 10, 64); err == nil {
					sessionID = uint(id)
				}
			case float64:
				sessionID = uint(v)
			}
		}
		// 1.3 最后按 user_id 查找活跃会话
		if sessionID == 0 {
			if userID == "" {
				return nil, errors.New("未指定 session_id 且 user_id 为空，无法定位会话")
			}
			session, err := s.sessionRepo.GetActiveByUserID(ctx, userID)
			if err != nil {
				return nil, fmt.Errorf("未找到用户的活跃会话：%w", err)
			}
			sessionID = session.ID
			sessionUserID = session.UserID
		}
	}

	// 2. 解析目标客服
	var agentID uint
	var agentName string

	// 2.1 优先使用显式 agent_id
	if aidRaw, ok := config["agent_id"]; ok {
		switch v := aidRaw.(type) {
		case float64:
			agentID = uint(v)
		case string:
			if v != "" {
				id, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("agent_id 格式无效：%w", err)
				}
				agentID = uint(id)
			}
		}
	}

	// 2.2 自动选择空闲客服
	if agentID == 0 {
		agents, err := s.agentRepo.GetOnlineAgents(ctx)
		if err != nil {
			return nil, fmt.Errorf("获取在线客服失败：%w", err)
		}
		if len(agents) == 0 {
			return nil, errors.New("当前没有可用的在线客服")
		}
		// GetOnlineAgents 已按 active_sessions ASC 排序，取第一个即负载最低
		agentID = agents[0].AgentID
		agentName = agents[0].AgentName
	} else {
		// 显式指定的客服，查询其名称
		status, err := s.agentRepo.GetByAgentID(ctx, agentID)
		if err != nil {
			return nil, fmt.Errorf("查询客服状态失败：%w", err)
		}
		agentName = status.AgentName
		if status.Status != "online" && status.Status != "busy" {
			return nil, fmt.Errorf("客服 %d 当前状态为 %s，无法分配", agentID, status.Status)
		}
		if status.ActiveSessions >= status.MaxSessions {
			return nil, fmt.Errorf("客服 %d 当前活跃会话数已达上限 %d", agentID, status.MaxSessions)
		}
	}

	// 3. 执行分配
	if err := s.sessionRepo.AssignAgent(ctx, sessionID, agentID, agentName); err != nil {
		return nil, fmt.Errorf("分配客服失败：%w", err)
	}

	// 4. 增加客服活跃会话计数
	_ = s.agentRepo.IncrementActiveSessions(ctx, agentID)

	return map[string]any{
		"success":    true,
		"session_id": sessionID,
		"agent_id":   agentID,
		"agent_name": agentName,
		"user_id":    firstNonEmpty(sessionUserID, userID),
	}, nil
}

// sendActionCreateTask 创建任务动作
// 通过 OperationLog 持久化任务记录，便于后续审计与跟进。
// 配置参数：
//   - title: string 任务标题（必填）
//   - description: string 任务描述
//   - module: string 模块名（默认 marketing_flow）
//   - resource_id: string 关联资源 ID
//   - assignee_id: float64 责任人 ID
func (s *MarketingFlowService) sendActionCreateTask(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.operationLogRepo == nil {
		return nil, errors.New("操作日志仓库未初始化")
	}

	// 提取任务标题
	title, _ := config["title"].(string)
	if title == "" {
		return nil, errors.New("task title 未指定")
	}

	// 提取可选字段
	description, _ := config["description"].(string)
	module, _ := config["module"].(string)
	if module == "" {
		module = "marketing_flow"
	}
	resourceID, _ := config["resource_id"].(string)

	// 解析责任人 ID（兼容 float64 / string / int）
	var assigneeID uint
	if assigneeRaw, ok := config["assignee_id"]; ok {
		switch v := assigneeRaw.(type) {
		case float64:
			assigneeID = uint(v)
		case string:
			if v != "" {
				if id, err := strconv.ParseUint(v, 10, 64); err == nil {
					assigneeID = uint(id)
				}
			}
		}
	}
	// 未指定责任人时，使用当前流程触发用户
	if assigneeID == 0 {
		if uid, err := strconv.ParseUint(userID, 10, 64); err == nil {
			assigneeID = uint(uid)
		}
	}
	if assigneeID == 0 {
		return nil, errors.New("无法确定任务责任人：assignee_id 与 user_id 均无效")
	}

	// 构建任务详情（JSON）
	detailMap := map[string]any{
		"title":       title,
		"description": description,
		"user_id":     userID,
		"source":      "marketing_flow",
	}
	for k, v := range data {
		// 排除内部字段，避免污染
		if !strings.HasPrefix(k, "_") {
			detailMap[k] = v
		}
	}
	detailJSON, err := json.Marshal(detailMap)
	if err != nil {
		return nil, fmt.Errorf("任务详情序列化失败：%w", err)
	}

	// 通过 OperationLog 持久化任务记录
	logEntry := &reachmodel.OperationLog{
		UserID:     assigneeID,
		Action:     "create",
		Module:     module,
		Resource:   "task",
		ResourceID: resourceID,
		Detail:     string(detailJSON),
		NewValue:   title,
	}

	if err := s.operationLogRepo.Create(ctx, logEntry); err != nil {
		return nil, fmt.Errorf("创建任务失败：%w", err)
	}

	return map[string]any{
		"success":     true,
		"task_id":     logEntry.ID,
		"title":       title,
		"assignee_id": assigneeID,
		"resource_id": resourceID,
	}, nil
}

// sendActionSendSms 发送短信动作
// 配置参数：
//   - phone: string 手机号（必填，若为空则从 data 中取 phone/user_phone）
//   - content: string 短信内容（必填）
//
// 通过函数注入的方式调用 SmsService.SendSms，避免与 internal/service 包形成循环依赖。
func (s *MarketingFlowService) sendActionSendSms(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if smsSenderFunc == nil {
		return nil, errors.New("短信发送器未注册，请确认 internal/service 包已调用 SetSmsSender")
	}

	// 提取手机号
	phone, _ := config["phone"].(string)
	if phone == "" {
		// 从 data 中回退取值
		if p, ok := data["phone"].(string); ok {
			phone = p
		} else if p, ok := data["user_phone"].(string); ok {
			phone = p
		}
	}
	if phone == "" {
		return nil, errors.New("phone 未指定")
	}

	// 提取短信内容
	content, _ := config["content"].(string)
	if content == "" {
		return nil, errors.New("content 未指定")
	}

	// 调用注入的 SMS 发送实现
	if err := smsSenderFunc(phone, content); err != nil {
		return nil, fmt.Errorf("发送短信失败：%w", err)
	}

	return map[string]any{
		"success": true,
		"phone":   phone,
		"content": content,
		"user_id": userID,
	}, nil
}

// sendActionUpdateLead 更新线索动作
// 配置参数：
//   - clue_id: string 线索 ID（必填，若为空则尝试从 data 中取 clue_id）
//   - fields: map[string]interface{} 需要更新的字段及其值
//
// 支持更新的字段：name / city / address / desc / is_verify / type / source_id / account
func (s *MarketingFlowService) sendActionUpdateLead(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.clueRepo == nil {
		return nil, errors.New("线索仓库未初始化")
	}

	// 提取线索 ID
	clueID, _ := config["clue_id"].(string)
	if clueID == "" {
		if id, ok := data["clue_id"].(string); ok {
			clueID = id
		} else if id, ok := data["lead_id"].(string); ok {
			clueID = id
		}
	}
	if clueID == "" {
		return nil, errors.New("clue_id 未指定")
	}

	// 提取待更新字段
	fieldsRaw, ok := config["fields"].(map[string]any)
	if !ok || len(fieldsRaw) == 0 {
		return nil, errors.New("fields 配置格式错误或为空")
	}

	// 白名单过滤，仅允许更新 Clue 模型中存在的字段
	allowedFields := map[string]bool{
		"name":      true,
		"city":      true,
		"address":   true,
		"desc":      true,
		"is_verify": true,
		"type":      true,
		"source_id": true,
		"account":   true,
	}
	updates := make(map[string]any)
	for k, v := range fieldsRaw {
		if !allowedFields[k] {
			continue
		}
		// 类型修正：is_verify 与 type 在 model 中为 int64
		if k == "is_verify" || k == "type" {
			switch vv := v.(type) {
			case float64:
				updates[k] = int64(vv)
			case int:
				updates[k] = int64(vv)
			case int64:
				updates[k] = vv
			case string:
				if n, err := strconv.ParseInt(vv, 10, 64); err == nil {
					updates[k] = n
				}
			default:
				return nil, fmt.Errorf("字段 %s 的值类型不合法", k)
			}
			continue
		}
		updates[k] = v
	}

	if len(updates) == 0 {
		return nil, errors.New("没有有效的可更新字段（请检查字段名是否在白名单内）")
	}

	// 调用仓库更新
	if err := s.clueRepo.UpdateByID(ctx, clueID, updates); err != nil {
		return nil, fmt.Errorf("更新线索失败：%w", err)
	}

	return map[string]any{
		"success": true,
		"clue_id": clueID,
		"updates": updates,
		"user_id": userID,
	}, nil
}

// sendActionCreateOrder 创建订单动作
// 配置参数：
//   - price: string 订单金额（必填）
//   - tg_id: float64 Telegram 用户 ID（必填，作为业务侧用户标识）
//   - account_id: string 账号 ID（必填）
//   - status: float64/string 初始订单状态（可选，默认 0=待支付）
func (s *MarketingFlowService) sendActionCreateOrder(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.orderRepo == nil {
		return nil, errors.New("订单仓库未初始化")
	}

	// 提取金额
	price, _ := config["price"].(string)
	if price == "" {
		// 兼容 float64 形式
		if p, ok := config["price"].(float64); ok {
			price = strconv.FormatFloat(p, 'f', -1, 64)
		}
	}
	if price == "" {
		return nil, errors.New("price 未指定")
	}

	// 提取 Telegram 用户 ID
	var tgID int64
	switch v := config["tg_id"].(type) {
	case float64:
		tgID = int64(v)
	case string:
		if v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("tg_id 格式无效：%w", err)
			}
			tgID = id
		}
	}
	if tgID == 0 {
		// 从 data 中回退取值
		if v, ok := data["tg_id"].(float64); ok {
			tgID = int64(v)
		} else if v, ok := data["tg_id"].(string); ok && v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				tgID = id
			}
		}
	}
	if tgID == 0 {
		return nil, errors.New("tg_id 未指定")
	}

	// 提取账号 ID
	accountID, _ := config["account_id"].(string)
	if accountID == "" {
		if v, ok := data["account_id"].(string); ok {
			accountID = v
		}
	}
	if accountID == "" {
		return nil, errors.New("account_id 未指定")
	}

	// 解析订单状态（默认 0=待支付）
	var statusInt int
	switch v := config["status"].(type) {
	case float64:
		statusInt = int(v)
	case string:
		if v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				statusInt = n
			}
		}
	}

	// 构建订单（ID 在 BeforeCreate 钩子中自动生成）
	order := &reachmodel.Order{
		Price:     price,
		TgID:      tgID,
		AccountID: accountID,
		Status:    _type.OrderStatusType(statusInt),
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("创建订单失败：%w", err)
	}

	return map[string]any{
		"success":    true,
		"order_id":   order.ID,
		"price":      price,
		"tg_id":      tgID,
		"account_id": accountID,
		"status":     int(order.Status),
		"user_id":    userID,
	}, nil
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// sendActionWebhook Webhook 动作
func (s *MarketingFlowService) sendActionWebhook(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return nil, errors.New("webhook URL 未指定")
	}

	// 设置默认方法为 POST
	method, _ := config["method"].(string)
	if method == "" {
		method = "POST"
	}

	// 构建请求体
	var bodyReader io.Reader
	if data != nil && len(data) > 0 {
		body, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("JSON 序列化失败：%w", err)
		}
		bodyReader = bytes.NewBuffer(body)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败：%w", err)
	}

	// 设置默认请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Marketing-Flow-Webhook/1.0")

	// 添加自定义请求头
	if headers, ok := config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if strVal, ok := v.(string); ok {
				req.Header.Set(k, strVal)
			}
		}
	}

	// 添加流程上下文信息
	req.Header.Set("X-Flow-User-ID", userID)

	// 执行请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送 Webhook 请求失败：%w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败：%w", err)
	}

	// 检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Webhook 返回错误状态码：%d, 响应：%s", resp.StatusCode, string(respBody))
	}

	// 尝试解析 JSON 响应
	var result map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			// 非 JSON 响应，返回原始内容
			return map[string]any{
				"status_code": resp.StatusCode,
				"body":        string(respBody),
			}, nil
		}
		result["status_code"] = resp.StatusCode
		return result, nil
	}

	return map[string]any{
		"status_code": resp.StatusCode,
	}, nil
}

// sendActionSendEmail 发送邮件动作
func (s *MarketingFlowService) sendActionSendEmail(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	// 获取 SMTP 配置
	smtpHost, _ := config["smtp_host"].(string)
	smtpUser, _ := config["smtp_user"].(string)
	smtpPass, _ := config["smtp_pass"].(string)

	// 验证 SMTP 配置
	if smtpHost == "" {
		return nil, errors.New("SMTP 主机未配置，请在动作配置或环境变量中设置 smtp_host")
	}
	if smtpUser == "" || smtpPass == "" {
		return nil, errors.New("SMTP 用户名或密码未配置")
	}

	// 解析 SMTP 端口（支持 int 和 float64 类型）
	var smtpPort int
	switch v := config["smtp_port"].(type) {
	case int:
		smtpPort = v
	case float64:
		smtpPort = int(v)
	default:
		smtpPort = 587
	}

	// 获取邮件参数
	to, ok := config["to"].(string)
	if !ok || to == "" {
		return nil, errors.New("收件人邮箱未指定")
	}

	subject, _ := config["subject"].(string)
	if subject == "" {
		subject = "营销邮件"
	}

	body, _ := config["body"].(string)
	if body == "" {
		return nil, errors.New("邮件正文未指定")
	}

	from, _ := config["from"].(string)
	if from == "" {
		from = smtpUser
	}

	// 构建邮件内容
	mime := "MIME-version: 1.0\r\n"
	contentType := "Content-Type: text/html; charset=\"UTF-8\"\r\n"
	if isHTML, ok := config["is_html"].(bool); ok && !isHTML {
		contentType = "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	}

	// 修复：邮件头字段 from/to/subject 去除 CR/LF，防止 CRLF 注入导致邮件头或收件人被注入
	mailHeader := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n%s%s\r\n",
		strings.NewReplacer("\r", "", "\n", "").Replace(from),
		strings.NewReplacer("\r", "", "\n", "").Replace(to),
		strings.NewReplacer("\r", "", "\n", "").Replace(subject),
		mime, contentType)
	message := mailHeader + body

	// 发送邮件
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(message))
	if err != nil {
		return nil, fmt.Errorf("发送邮件失败：%w", err)
	}

	return map[string]any{
		"sent": true,
		"to":   to,
	}, nil
}

// evaluateCondition 评估条件
func (s *MarketingFlowService) evaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range data {
		result[k] = v
	}

	// 获取条件表达式
	conditionRaw, ok := node.Config["condition"].(string)
	if !ok || conditionRaw == "" {
		// 空条件视为始终匹配，返回 true
		result["_condition_matched"] = true
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	// 解析条件表达式："field operator value"
	field, operator, value, err := parseCondition(conditionRaw)
	if err != nil {
		return nil, err
	}

	// 获取字段值
	fieldValue, exists := data[field]
	if !exists {
		// 字段不存在，返回 false，不报错
		result["_condition_matched"] = false
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	// 评估条件
	matched, err := evaluateOperator(fieldValue, operator, value)
	if err != nil {
		return nil, err
	}

	result["_condition_matched"] = matched

	// 根据条件结果选择下一个节点
	if matched {
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
	} else {
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
	}

	return result, nil
}

// parseCondition 解析条件表达式 "field operator value"
func parseCondition(condition string) (field, operator, value string, err error) {
	condition = strings.TrimSpace(condition)

	// 支持的运算符列表（按长度降序排列，优先匹配长的运算符）
	operators := []string{"contains", "gte", "lte", "eq", "ne", "gt", "lt", "in"}

	for _, op := range operators {
		// 查找运算符位置
		idx := strings.Index(condition, " "+op+" ")
		if idx != -1 {
			field = strings.TrimSpace(condition[:idx])
			rest := strings.TrimSpace(condition[idx+len(op)+2:])
			return field, op, rest, nil
		}
	}

	return "", "", "", errors.New("无效的条件表达式：未识别的运算符")
}

// ParseCondition 公开版 parseCondition(供跨包调用,如 service/sop_condition.go)
func ParseCondition(condition string) (field, operator, value string, err error) {
	return parseCondition(condition)
}

// evaluateOperator 评估运算符
func evaluateOperator(fieldValue any, operator, value string) (bool, error) {
	switch operator {
	case "eq":
		return evalEq(fieldValue, value)
	case "ne":
		return evalNe(fieldValue, value)
	case "gt":
		return evalGt(fieldValue, value)
	case "lt":
		return evalLt(fieldValue, value)
	case "gte":
		return evalGte(fieldValue, value)
	case "lte":
		return evalLte(fieldValue, value)
	case "contains":
		return evalContains(fieldValue, value)
	case "in":
		return evalIn(fieldValue, value)
	default:
		return false, fmt.Errorf("不支持的运算符：%s", operator)
	}
}

// EvaluateOperator 公开版 evaluateOperator(供跨包调用,如 service/sop_condition.go)
func EvaluateOperator(fieldValue any, operator, value string) (bool, error) {
	return evaluateOperator(fieldValue, operator, value)
}

// evalEq 等于比较
func evalEq(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case string:
		return strings.EqualFold(fv, value), nil
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv == numValue, nil
	default:
		return strings.EqualFold(fmt.Sprintf("%v", fv), value), nil
	}
}

// evalNe 不等于比较
func evalNe(fieldValue any, value string) (bool, error) {
	result, err := evalEq(fieldValue, value)
	if err != nil {
		return false, err
	}
	return !result, nil
}

// evalGt 大于比较
func evalGt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv > numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) > numValue, nil
	default:
		return false, errors.New("gt 运算符仅支持数字类型")
	}
}

// evalLt 小于比较
func evalLt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv < numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) < numValue, nil
	default:
		return false, errors.New("lt 运算符仅支持数字类型")
	}
}

// evalGte 大于等于比较
func evalGte(fieldValue any, value string) (bool, error) {
	result, err := evalGt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}
	// 检查是否等于
	return evalEq(fieldValue, value)
}

// evalLte 小于等于比较
func evalLte(fieldValue any, value string) (bool, error) {
	result, err := evalLt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}
	// 检查是否等于
	return evalEq(fieldValue, value)
}

// evalContains 包含比较（大小写不敏感）
func evalContains(fieldValue any, value string) (bool, error) {
	strValue := fmt.Sprintf("%v", fieldValue)
	return strings.Contains(strings.ToLower(strValue), strings.ToLower(value)), nil
}

// evalIn 列表成员比较
func evalIn(fieldValue any, value string) (bool, error) {
	// 解析列表 [a,b,c]
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return false, errors.New("in 运算符的列表格式错误，应该为 [a,b,c]")
	}

	// 去掉方括号
	listStr := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	items := strings.Split(listStr, ",")

	for _, item := range items {
		item = strings.TrimSpace(item)
		matched, err := evalEq(fieldValue, item)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
}

// handleDelay 处理延迟
func (s *MarketingFlowService) handleDelay(ctx context.Context, node model.FlowNode) (map[string]any, error) {
	duration, _ := node.Config["duration"].(float64)
	if duration > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(duration) * time.Second):
		}
	}
	return nil, nil
}

// GetActiveFlows 获取所有激活的流程
func (s *MarketingFlowService) GetActiveFlows() ([]*model.MarketingFlow, error) {
	return s.flowRepo.GetByStatus(model.FlowStatusActive)
}

// EvaluateCondition 公开版 evaluateCondition(供跨包测试使用)
func (s *MarketingFlowService) EvaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	return s.evaluateCondition(node, data)
}
