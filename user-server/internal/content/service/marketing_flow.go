package service

import (
	"context"

	"encoding/json"

	"errors"

	"fmt"

	"hivemtk-user/internal/content/model"

	"hivemtk-user/internal/content/repository"

	reachmodel "hivemtk-user/internal/model"

	"hivemtk-user/internal/platform"

	cdprepo "hivemtk-user/internal/repository"

	"strconv"

	"strings"

	"time"

	"gorm.io/gorm"
)

var smsSenderFunc func(phone, content string) error

func SetSmsSender(fn func(phone, content string) error) {
	smsSenderFunc = fn
}

var workflowAssetResolverFunc func(ctx context.Context) (json.RawMessage, bool)

func SetWorkflowAssetResolver(fn func(ctx context.Context) (json.RawMessage, bool)) {
	workflowAssetResolverFunc = fn
}

type MarketingWorkflow struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	Industry string                   `json:"industry"`
	Trigger  map[string]interface{}   `json:"trigger"`
	Steps    []map[string]interface{} `json:"steps"`
	KPI      map[string]interface{}   `json:"kpi"`
}

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

type CreateFlowRequest struct {
	Name          string                `json:"name" binding:"required"`
	Description   string                `json:"description"`
	TriggerType   model.TriggerType     `json:"trigger_type" binding:"required"`
	TriggerConfig map[string]any        `json:"trigger_config"`
	FlowData      *model.FlowDefinition `json:"flow_data"`
}

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

func (s *MarketingFlowService) GetFlowList(page, pageSize int) ([]*model.MarketingFlow, int64, error) {
	return s.flowRepo.GetAll(page, pageSize)
}

func (s *MarketingFlowService) GetFlowByID(id uint) (*model.MarketingFlow, error) {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

type UpdateFlowRequest struct {
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	TriggerType   model.TriggerType     `json:"trigger_type"`
	TriggerConfig map[string]any        `json:"trigger_config"`
	FlowData      *model.FlowDefinition `json:"flow_data"`
}

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

func (s *MarketingFlowService) DeleteFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.Delete(id)
}

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

func (s *MarketingFlowService) PauseFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.UpdateStatus(id, model.FlowStatusPaused)
}

func (s *MarketingFlowService) StopFlow(id uint) error {
	flow, err := s.flowRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = flow

	return s.flowRepo.UpdateStatus(id, model.FlowStatusInactive)
}

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

func (s *MarketingFlowService) GetExecutionList(flowID uint, page, pageSize int) ([]*model.FlowExecution, int64, error) {
	return s.executionRepo.GetByFlowID(flowID, page, pageSize)
}

func (s *MarketingFlowService) GetExecutionStats(flowID uint) (map[string]int64, error) {
	return s.executionRepo.GetStats(flowID)
}

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
