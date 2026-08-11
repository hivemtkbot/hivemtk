package service

import (
	"context"

	"encoding/json"

	"errors"

	"fmt"

	"sync"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/llm"

	"hivemtk-user/internal/dto"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"
)

type SOPService struct {
	agentRepo  *repository.SopAgentRepository
	execRepo   *repository.SopExecutionRepository
	dispatcher *llm.Dispatcher
}

const (
	SOPStatusPending = "pending"

	SOPStatusRunning = "running"

	SOPStatusSuccess = "success"

	SOPStatusFailed = "failed"

	SOPStatusPaused = "paused"

	SOPStatusCanceled = "canceled"

	SOPNodeTypeStart = "start"

	SOPNodeTypeMessage = "message"

	SOPNodeTypeBranch = "branch"

	SOPNodeTypeWait = "wait"

	SOPNodeTypeAction = "action"

	SOPNodeTypeEnd = "end"

	SOPNodeTypeAIDecide = "ai_decide"

	SOPNodeTypeSendOffer = "send_offer"

	SOPNodeTypeGreeting = "greeting" // 问候

	SOPNodeTypeInquire = "inquire" // 询问需求

	SOPNodeTypeIntroduce = "introduce" // 介绍产品

	SOPNodeTypeHandle = "handle" // 异议处理

	SOPNodeTypeClose = "close" // 促单

	SOPNodeTypeInvite = "invite" // 邀约

	SOPNodeTypeFollowUp = "follow_up" // 跟进

	SOPNodeTypeActivate = "activate" // 激活沉默客户

	SOPNodeTypeNurture = "nurture" // 培育线索

	SOPNodeTypeCondition = "condition" // 条件分支（替代旧 branch）

	SOPNodeTypeLLM = "llm" // LLM 决策节点

	SOPTriggerManual = "manual"

	SOPTriggerAuto = "auto"

	SOPTriggerIntent = "intent"

	SOPTriggerSchedule = "schedule"
)

var SOPNodeSupportedTypes = map[string]bool{
	SOPNodeTypeStart: true, SOPNodeTypeMessage: true, SOPNodeTypeBranch: true,
	SOPNodeTypeWait: true, SOPNodeTypeAction: true, SOPNodeTypeEnd: true,
	SOPNodeTypeAIDecide: true, SOPNodeTypeSendOffer: true,
	SOPNodeTypeGreeting: true, SOPNodeTypeInquire: true, SOPNodeTypeIntroduce: true,
	SOPNodeTypeHandle: true, SOPNodeTypeClose: true, SOPNodeTypeInvite: true,
	SOPNodeTypeFollowUp: true, SOPNodeTypeActivate: true, SOPNodeTypeNurture: true,
	SOPNodeTypeCondition: true, SOPNodeTypeLLM: true,
}

var (
	ErrSOPNotFound = errors.New("sop not found")

	ErrSOPInvalidGraph = errors.New("invalid sop graph")

	ErrSOPNoStart = errors.New("sop graph has no start node")

	ErrSOPExecNotFound = errors.New("execution not found")

	ErrSOPExecNotRunning = errors.New("execution is not running")
)

type SOPNode = dto.SOPNode

type SOPConditionBranch = dto.SOPConditionBranch

type SOPPosition = dto.SOPPosition

type SOPGraph = dto.SOPGraph

type SOPEdge = dto.SOPEdge

func NewSOPService(db *gorm.DB, dispatcher *llm.Dispatcher) *SOPService {
	return &SOPService{
		agentRepo:  repository.NewSopAgentRepository(db),
		execRepo:   repository.NewSopExecutionRepository(db),
		dispatcher: dispatcher,
	}
}

type CreateRequest = dto.CreateRequest

func (s *SOPService) TemplateFromActiveAsset(ctx context.Context, scenario string) (*CreateRequest, bool) {
	if r := GetAssetResolver(); r != nil {
		if sop, ok := r.GetActiveSOP(ctx); ok && sop != nil {
			return sop.ToCreateRequest(context.Background(), scenario), true
		}
	}
	return nil, false
}

func (s *SOPService) Create(ctx context.Context, req *CreateRequest) (*model.SOPAgent, error) {
	// M2 运行时覆盖默认：请求未携带 SOP 图时，回退到「生效中」的已购 industry_sop 资产作为默认模板。
	if len(req.SOPGraph.Nodes) == 0 {
		if tpl, ok := s.TemplateFromActiveAsset(ctx, req.Scenario); ok && tpl != nil {
			req = tpl
		}
	}
	if err := s.validateGraph(ctx, &req.SOPGraph); err != nil {
		return nil, err
	}
	// M2 运行时覆盖默认：请求未配置 A/B 测试时，回退到「生效中」的已购 ab_test_plan 资产
	// 作为默认方案（仅当方案校验通过，避免引入非法配置）。
	if !req.ABTestConfig.Enabled && len(req.ABTestConfig.Variants) == 0 {
		if r := GetAssetResolver(); r != nil {
			if plan, ok := r.GetActiveABPlan(ctx); ok && plan != nil {
				if cfg := plan.ToSOPABTestConfig(context.Background()); ValidateSOPABTestConfig(cfg) == nil {
					req.ABTestConfig = cfg
				}
			}
		}
	}
	// 校验 A/B 测试配置（如启用）
	if err := ValidateSOPABTestConfig(req.ABTestConfig); err != nil {
		return nil, fmt.Errorf("A/B 测试配置非法：%w", err)
	}
	graphData, _ := json.Marshal(req.SOPGraph)
	if req.TriggerType == "" {
		req.TriggerType = SOPTriggerAuto
	}
	triggerMap := toJSONMap(req.TriggerConfig)
	abMap := model.JSONMap{}
	if req.ABTestConfig.Enabled {
		abData, _ := json.Marshal(req.ABTestConfig)
		_ = json.Unmarshal(abData, &abMap)
	}
	agent := &model.SOPAgent{

		Name:          req.Name,
		Scenario:      req.Scenario,
		Description:   req.Description,
		TriggerType:   req.TriggerType,
		TriggerConfig: triggerMap,
		SOPGraph:      toJSONMapBytes(graphData),
		Priority:      req.Priority,
		ABTestConfig:  abMap,
		CreatedBy:     req.CreatedBy,
		IsActive:      true,
	}
	if agent.TriggerConfig == nil {
		agent.TriggerConfig = model.JSONMap{}
	}
	if err := s.agentRepo.Create(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *SOPService) Update(ctx context.Context, id uint, req *CreateRequest) (*model.SOPAgent, error) {
	if err := s.validateGraph(ctx, &req.SOPGraph); err != nil {
		return nil, err
	}
	// 校验 A/B 测试配置（如启用）
	if err := ValidateSOPABTestConfig(req.ABTestConfig); err != nil {
		return nil, fmt.Errorf("A/B 测试配置非法：%w", err)
	}
	agent, err := s.agentRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSOPNotFound
		}
		return nil, err
	}
	graphData, _ := json.Marshal(req.SOPGraph)
	abMap := model.JSONMap{}
	if req.ABTestConfig.Enabled {
		abData, _ := json.Marshal(req.ABTestConfig)
		_ = json.Unmarshal(abData, &abMap)
	}
	agent.Name = req.Name
	agent.Scenario = req.Scenario
	agent.Description = req.Description
	agent.TriggerType = req.TriggerType
	agent.TriggerConfig = toJSONMap(req.TriggerConfig)
	agent.SOPGraph = toJSONMapBytes(graphData)
	agent.Priority = req.Priority
	agent.ABTestConfig = abMap
	agent.Version++
	if err := s.agentRepo.Save(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

func (s *SOPService) Get(ctx context.Context, id uint) (*model.SOPAgent, error) {
	agent, err := s.agentRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSOPNotFound
		}
		return nil, err
	}
	return agent, nil
}

func (s *SOPService) List(ctx context.Context, scenario string, page, pageSize int) ([]model.SOPAgent, int64, error) {
	return s.agentRepo.List(ctx, scenario, page, pageSize)
}

func (s *SOPService) Delete(ctx context.Context, id uint) error {
	rowsAffected, err := s.agentRepo.DeleteByID(ctx, id)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrSOPNotFound
	}
	return nil
}

func (s *SOPService) Activate(ctx context.Context, id uint) error {
	rowsAffected, err := s.agentRepo.UpdateActive(ctx, id, true)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrSOPNotFound
	}
	return nil
}

func (s *SOPService) Deactivate(ctx context.Context, id uint) error {
	rowsAffected, err := s.agentRepo.UpdateActive(ctx, id, false)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrSOPNotFound
	}
	return nil
}

func (s *SOPService) Execute(ctx context.Context, req *dto.ExecuteRequest) (*model.SOPExecution, error) {
	agent, err := s.Get(ctx, req.SOPID)
	if err != nil {
		return nil, err
	}
	if !agent.IsActive {
		return nil, errors.New("sop is not active")
	}

	// 解析 A/B 测试 variant（未启用时返回空，使用主图）
	variantName, variantGraphID, err := s.resolveABTestVariant(context.Background(), agent, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("A/B 测试分流失败：%w", err)
	}

	// 生成 trace_id（用于全链路追踪）
	traceID := logger.TraceIDFromContext(ctx)
	if traceID == "" {
		traceID = logger.GenerateTraceID()
	}

	exec := &model.SOPExecution{
		SOPID:          req.SOPID,
		CustomerID:     req.CustomerID,
		SessionID:      req.SessionID,
		Status:         SOPStatusRunning,
		CurrentNodeIdx: 0,
		StartedAt:      time.Now(),
		ExecutionData:  model.JSONMap(req.Input),
		Variant:        variantName,
		TraceID:        traceID,
	}
	if exec.ExecutionData == nil {
		exec.ExecutionData = model.JSONMap{}
	}
	if err := s.execRepo.Create(ctx, exec); err != nil {
		return nil, err
	}
	// 根据 variant 加载对应 SOP 图
	graph, err := s.loadSOPGraph(context.Background(), agent, variantGraphID)
	if err != nil {
		return nil, err
	}
	startNode := findStartNode(&graph)
	if startNode == nil {
		exec.Status = SOPStatusFailed
		exec.ErrorMessage = ErrSOPNoStart.Error()
		_ = s.execRepo.Save(ctx, exec)
		return exec, ErrSOPNoStart
	}
	exec.CurrentNode = startNode.ID
	if err := s.execRepo.Save(ctx, exec); err != nil {
		return nil, err
	}
	// 累加 execution_count
	_ = s.agentRepo.IncrementExecutionCount(ctx, agent.ID)

	// 派发 start 节点到 SOPExecutionDispatcher
	// 若调度器未初始化（如测试场景），仅返回 Execution，节点流转由调用方 Step 推进（向后兼容）
	if dispatcher := GetSOPExecutionDispatcher(); dispatcher != nil {
		dispatcher.DispatchOrLog(&dispatchTask{
			ExecutionID: exec.ID,
			NodeID:      startNode.ID,
			Attempt:     0,
			TraceID:     traceID,
		})
	}
	return exec, nil
}

func (s *SOPService) Step(ctx context.Context, req *dto.StepRequest) (*model.SOPExecution, error) {
	exec, err := s.execRepo.GetByID(ctx, req.ExecutionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSOPExecNotFound
		}
		return nil, err
	}
	if exec.Status != SOPStatusRunning {
		return nil, ErrSOPExecNotRunning
	}

	// 输出合并到执行数据
	if exec.ExecutionData == nil {
		exec.ExecutionData = model.JSONMap{}
	}
	for k, v := range req.Output {
		exec.ExecutionData[k] = v
	}

	// 清除 wait_event（客户回复唤醒）
	if exec.WaitEvent != "" {
		exec.WaitEvent = ""
	}

	// 持久化合并后的 ExecutionData
	if err := s.execRepo.Save(ctx, exec); err != nil {
		return nil, err
	}

	// 派发当前节点任务给调度器（客户回复唤醒场景）
	// 调度器会重新执行当前 wait 节点，由 WaitExecutor 检测到 timer 已 fired 后推进下一节点
	if dispatcher := GetSOPExecutionDispatcher(); dispatcher != nil {
		traceID := exec.TraceID
		if traceID == "" {
			traceID = logger.GenerateTraceID()
		}
		dispatcher.DispatchOrLog(&dispatchTask{
			ExecutionID: exec.ID,
			NodeID:      exec.CurrentNode,
			Attempt:     0,
			TraceID:     traceID,
		})
		return exec, nil
	}

	// 调度器未初始化：保持原有同步推进逻辑（向后兼容）
	agent, err := s.Get(ctx, exec.SOPID)
	if err != nil {
		return nil, err
	}

	// 根据 exec.Variant 加载对应的 SOP 图
	// variant=="" 表示未启用 A/B 测试，使用主图
	var variantGraphID uint
	if exec.Variant != "" {
		cfg := ParseSOPABTestConfig(agent.ABTestConfig)
		if cfg.Enabled {
			for _, v := range cfg.Variants {
				if v.Name == exec.Variant {
					variantGraphID = v.SOPGraphID
					break
				}
			}
		}
	}
	graph, err := s.loadSOPGraph(context.Background(), agent, variantGraphID)
	if err != nil {
		return nil, err
	}

	// 找到当前节点
	current := findNodeByID(&graph, exec.CurrentNode)
	if current == nil {
		exec.Status = SOPStatusFailed
		exec.ErrorMessage = "current node not found"
		_ = s.execRepo.Save(ctx, exec)
		return exec, nil
	}
	// 决定下一个节点
	next := nextNode(&graph, current, exec.ExecutionData)
	if next == nil {
		exec.Status = SOPStatusSuccess
		now := time.Now()
		exec.CompletedAt = &now
		if err := s.execRepo.Save(ctx, exec); err != nil {
			return nil, err
		}
		_ = s.agentRepo.IncrementSuccessCount(ctx, exec.SOPID)
		return exec, nil
	}
	exec.CurrentNode = next.ID
	for i, n := range graph.Nodes {
		if n.ID == next.ID {
			exec.CurrentNodeIdx = i
			break
		}
	}
	if err := s.execRepo.Save(ctx, exec); err != nil {
		return nil, err
	}
	return exec, nil
}

func (s *SOPService) Pause(ctx context.Context, execID uint) error {
	return s.execRepo.UpdateStatus(ctx, execID, SOPStatusPaused)
}

func (s *SOPService) Resume(ctx context.Context, execID uint) error {
	return s.execRepo.UpdateStatus(ctx, execID, SOPStatusRunning)
}

func (s *SOPService) Cancel(ctx context.Context, execID uint) error {
	now := time.Now()
	return s.execRepo.UpdateFields(ctx, execID, map[string]any{
		"status":       SOPStatusCanceled,
		"completed_at": now,
	})
}

func (s *SOPService) GetExecution(ctx context.Context, execID uint) (*model.SOPExecution, error) {
	exec, err := s.execRepo.GetByID(ctx, execID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSOPExecNotFound
		}
		return nil, err
	}
	return exec, nil
}

func (s *SOPService) ListExecutions(ctx context.Context, customerID string, status string, page, pageSize int) ([]model.SOPExecution, int64, error) {
	return s.execRepo.List(ctx, customerID, status, page, pageSize)
}

func (s *SOPService) MatchByIntent(ctx context.Context, intentType string) ([]model.SOPAgent, error) {
	list, err := s.agentRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	// 简单的匹配规则：trigger_config 中包含 intent 字段
	matched := []model.SOPAgent{}
	for _, a := range list {
		if a.TriggerType != SOPTriggerIntent {
			continue
		}
		if intents, ok := a.TriggerConfig["intents"].([]any); ok {
			for _, i := range intents {
				if s, ok := i.(string); ok && s == intentType {
					matched = append(matched, a)
					break
				}
			}
		}
	}
	return matched, nil
}

func (s *SOPService) Stats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"total":   0,
		"active":  0,
		"running": 0,
		"success": 0,
		"failed":  0,
	}
	// 统计 SOP 智能体总数与活跃数
	totalAgents, err := s.agentRepo.CountAll(ctx)
	if err != nil {
		return stats, err
	}
	activeAgents, err := s.agentRepo.CountActive(ctx)
	if err != nil {
		return stats, err
	}
	// 统计执行记录：运行中、已完成、失败
	runningExecs, err := s.execRepo.CountByStatus(ctx, SOPStatusRunning)
	if err != nil {
		return stats, err
	}
	successExecs, err := s.execRepo.CountByStatus(ctx, SOPStatusSuccess)
	if err != nil {
		return stats, err
	}
	failedExecs, err := s.execRepo.CountByStatus(ctx, SOPStatusFailed)
	if err != nil {
		return stats, err
	}

	stats["total"] = totalAgents
	stats["active"] = activeAgents
	stats["running"] = runningExecs
	stats["success"] = successExecs
	stats["failed"] = failedExecs
	return stats, nil
}

func (s *SOPService) validateGraph(ctx context.Context, graph *SOPGraph) error {
	if graph == nil {
		return ErrSOPInvalidGraph
	}
	if len(graph.Nodes) == 0 {
		return ErrSOPInvalidGraph
	}
	hasStart := false
	ids := map[string]bool{}
	for _, n := range graph.Nodes {
		if n.ID == "" {
			return fmt.Errorf("node has empty id")
		}
		if ids[n.ID] {
			return fmt.Errorf("duplicate node id: %s", n.ID)
		}
		ids[n.ID] = true
		// 节点类型校验（支持 start/greeting/inquire/introduce/handle/close/invite/follow_up/activate/nurture/condition/llm/旧版类型）
		if !SOPNodeSupportedTypes[n.Type] {
			return fmt.Errorf("node %s has unsupported type: %s", n.ID, n.Type)
		}
		if n.Type == SOPNodeTypeStart {
			hasStart = true
		}
		// condition 节点：校验 Conditions 分支的 Next 引用
		if n.Type == SOPNodeTypeCondition {
			for _, br := range n.Conditions {
				if br.Next == "" {
					return fmt.Errorf("condition node %s has a branch with empty next", n.ID)
				}
			}
		}
	}
	if !hasStart {
		return ErrSOPNoStart
	}
	for _, n := range graph.Nodes {
		for _, nextID := range n.Next {
			if !ids[nextID] {
				return fmt.Errorf("node %s references missing node %s", n.ID, nextID)
			}
		}
		// condition 节点的分支 Next 引用校验
		if n.Type == SOPNodeTypeCondition {
			for _, br := range n.Conditions {
				if !ids[br.Next] {
					return fmt.Errorf("condition node %s branch [%s] references missing node %s", n.ID, br.Label, br.Next)
				}
			}
		}
	}
	// 边引用校验
	for _, e := range graph.Edges {
		if !ids[e.From] {
			return fmt.Errorf("edge from missing node %s", e.From)
		}
		if !ids[e.To] {
			return fmt.Errorf("edge to missing node %s", e.To)
		}
	}
	return nil
}

func findStartNode(graph *SOPGraph) *SOPNode {
	for i, n := range graph.Nodes {
		if n.Type == SOPNodeTypeStart {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func findNodeByID(graph *SOPGraph, id string) *SOPNode {
	for i, n := range graph.Nodes {
		if n.ID == id {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func nextNode(graph *SOPGraph, current *SOPNode, data model.JSONMap) *SOPNode {
	if current == nil {
		return nil
	}
	if current.Type == SOPNodeTypeEnd {
		return nil
	}

	// condition 节点：优先级路由
	if current.Type == SOPNodeTypeCondition {
		// 优先用 Conditions 字段
		if len(current.Conditions) > 0 {
			br, err := SOPEvaluateConditionBranches(current.Conditions, data)
			if err == nil && br.Matched && br.NextNode != "" {
				if n := findNodeByID(graph, br.NextNode); n != nil {
					return n
				}
			}
			// 不匹配或失败时，退回到 Next[0]（兜底）
			if len(current.Next) > 0 {
				return findNodeByID(graph, current.Next[0])
			}
			return nil
		}

		// fallback：使用旧 Condition 字段（兼容旧版数据）
		condStr := current.Condition
		if condStr == "" {
			if c, ok := current.Config["condition"].(string); ok {
				condStr = c
			}
		}
		if condStr != "" {
			result, err := SOPEvaluateNodeCondition(current, data)
			if err == nil {
				if nextID, ok := result["_next_node"].(string); ok && nextID != "" {
					if n := findNodeByID(graph, nextID); n != nil {
						return n
					}
				}
			}
		}
		// 兜底
		if len(current.Next) > 0 {
			return findNodeByID(graph, current.Next[0])
		}
		return nil
	}

	// llm 节点：LLM 决策结果
	if current.Type == SOPNodeTypeLLM {
		// 优先从执行数据读取 LLM 决策结果
		if decision, ok := data["_llm_decision"].(string); ok && decision != "" {
			if n := findNodeByID(graph, decision); n != nil {
				return n
			}
		}
		// 退化：从 Config.next 读取
		if nextID, ok := current.Config["next"].(string); ok && nextID != "" {
			if n := findNodeByID(graph, nextID); n != nil {
				return n
			}
		}
		// 再退回到 Next[0]
		if len(current.Next) > 0 {
			return findNodeByID(graph, current.Next[0])
		}
		return nil
	}

	// 旧版 branch 节点：Edges + when 字段
	if current.Type == SOPNodeTypeBranch {
		// 优先看是否有 Conditions（兼容升级）
		if len(current.Conditions) > 0 {
			br, err := SOPEvaluateConditionBranches(current.Conditions, data)
			if err == nil && br.Matched && br.NextNode != "" {
				if n := findNodeByID(graph, br.NextNode); n != nil {
					return n
				}
			}
		}
		// 旧版 Edges 评估
		for _, e := range graph.Edges {
			if e.From == current.ID {
				if v, ok := data["_branch_result"].(string); ok {
					if v == e.When || (v == "true" && e.When == "true") || (v == "false" && e.When == "false") {
						return findNodeByID(graph, e.To)
					}
				}
			}
		}
		// 退回到 Next
		if len(current.Next) > 0 {
			return findNodeByID(graph, current.Next[0])
		}
		return nil
	}

	// 旧版 ai_decide 节点：等价 llm 节点（向后兼容）
	if current.Type == SOPNodeTypeAIDecide {
		if decision, ok := data["_ai_decision"].(string); ok && decision != "" {
			if n := findNodeByID(graph, decision); n != nil {
				return n
			}
		}
		if decision, ok := data["_llm_decision"].(string); ok && decision != "" {
			if n := findNodeByID(graph, decision); n != nil {
				return n
			}
		}
		if len(current.Next) > 0 {
			return findNodeByID(graph, current.Next[0])
		}
		return nil
	}

	// 通用节点：按 Next[0] 顺序流转
	if len(current.Next) > 0 {
		return findNodeByID(graph, current.Next[0])
	}
	return nil
}

var (
	sopOnce sync.Once

	sopInstance *SOPService
)

func GetSOPService() *SOPService {
	return sopInstance
}

func InitSOPService(db *gorm.DB, dispatcher *llm.Dispatcher) *SOPService {
	sopOnce.Do(func() {
		sopInstance = NewSOPService(db, dispatcher)
	})
	return sopInstance
}

// ===== merged from sop_part2.go (was a mechanical _partN split) =====
func NewWelcomeSOP() *CreateRequest {
	return &CreateRequest{

		Name:        "客户欢迎 SOP",
		Scenario:    "welcome",
		Description: "新客户接入时的标准欢迎流程（14 节点类型示范）",
		TriggerType: SOPTriggerAuto,
		SOPGraph: SOPGraph{
			Name:     "welcome_graph",
			Scenario: "welcome",
			Version:  "2.0",
			Entry:    "start",
			Exits:    []string{"end"},
			Nodes: []SOPNode{
				{ID: "start", Type: SOPNodeTypeStart, Name: "开始", Next: []string{"greeting"}},
				{
					ID:          "greeting",
					Type:        SOPNodeTypeGreeting,
					Name:        "问候",
					Description: "标准化客户问候",
					Prompt:      "您好，欢迎咨询，我是您的专属顾问",
					Next:        []string{"inquire"},
				},
				{
					ID:          "inquire",
					Type:        SOPNodeTypeInquire,
					Name:        "询问需求",
					Description: "了解客户核心诉求",
					Prompt:      "请问您想了解什么产品或服务？",
					Next:        []string{"end"},
				},
				{ID: "end", Type: SOPNodeTypeEnd, Name: "结束"},
			},
		},
	}
}
