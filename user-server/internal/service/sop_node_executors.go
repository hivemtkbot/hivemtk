package service

// ============================================================================
// 14 种 SOP 节点执行器实现（SOP 节点执行器完善设计）
// ----------------------------------------------------------------------------
// 设计依据：docs/核心链路优化.md 第十三章 §13.2.2
// 私域独立部署：无 merchant_id 字段
// 五层架构：本文件位于 L3 业务层（Service）
//
// 节点分类（按行为）：
//   1. 空动作类：start / end —— 仅状态机推进
//   2. 消息发送类：greeting / inquire / introduce / handle / close / invite /
//                  follow_up / activate / nurture（9 种）+ 旧版 message / action / send_offer
//                  复用 MessageNodeBase 三级降级（node.Prompt > ScriptTemplate > LLM）
//   3. 条件路由类：condition / 旧版 branch
//   4. LLM 决策类：llm / 旧版 ai_decide
//   5. 等待类：wait
//
// 消息发送统一走 SessionMessageRepository.Create 持久化 + WebSocket Hub 推送，
// 与 SmartCSOrchestrator.saveOutboundMessage 保持一致（私域部署不需要平台适配器出站）。
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/websocket"

	"gorm.io/gorm"
)

// SOPNodeExecutorDeps 节点执行器共享依赖
//
// 通过统一结构体注入，避免每个执行器重复声明字段。
// 由 SOPExecutionDispatcher 在构造时填充并分发给各执行器。
type SOPNodeExecutorDeps struct {
	DB          *gorm.DB
	Dispatcher  *llm.Dispatcher
	WSHub       *websocket.Hub
	MsgRepo     *repository.SessionMessageRepository
	SessionRepo *repository.CustomerSessionRepository
	LLMSem      chan struct{} // 全局 LLM 并发信号量（默认容量 4）
}

// ============================================================================
// 1. 空动作类执行器：start / end
// ============================================================================

// StartExecutor 开始节点执行器
//
// 仅记录 trace 与时间戳，立即推进到 Next[0]。
type StartExecutor struct{}

// NodeType 返回节点类型
func (e *StartExecutor) NodeType() string { return SOPNodeTypeStart }

// Execute 记录开始事件，推进下一节点
func (e *StartExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	logger.Ctx(ctx).Info().
		Str("execution_id", fmt.Sprintf("%d", ec.Execution.ID)).
		Str("sop_id", fmt.Sprintf("%d", ec.Execution.SOPID)).
		Str("customer_id", ec.CustomerID).
		Msg("sop execution started")
	return &NodeExecResult{
		Status: NodeStatusCompleted,
		Output: model.JSONMap{
			"_started_at": ec.StartedAt.Format(time.RFC3339),
			"_trigger":    ec.Execution.ExecutionData["_trigger"],
		},
	}, nil
}

// IsAsync 同步执行
func (e *StartExecutor) IsAsync() bool { return false }

// EndExecutor 结束节点执行器
//
// 标记流程完成，调度器据将 Execution.Status 置为 success。
type EndExecutor struct{}

// NodeType 返回节点类型
func (e *EndExecutor) NodeType() string { return SOPNodeTypeEnd }

// Execute 标记流程结束
func (e *EndExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	logger.Ctx(ctx).Info().
		Str("execution_id", fmt.Sprintf("%d", ec.Execution.ID)).
		Str("sop_id", fmt.Sprintf("%d", ec.Execution.SOPID)).
		Msg("sop execution ended")
	return &NodeExecResult{
		Status: NodeStatusCompleted,
		Output: model.JSONMap{
			"_ended_at": time.Now().Format(time.RFC3339),
		},
		NextNodeID: "", // 空表示流程结束
	}, nil
}

// IsAsync 同步执行
func (e *EndExecutor) IsAsync() bool { return false }

// ============================================================================
// 2. 消息发送类执行器：MessageNodeBase + 9 种商用节点 + 3 种旧版节点
// ============================================================================

// MessageNodeBase 消息发送类节点基座
//
// 9 种商用节点（greeting/inquire/introduce/handle/close/invite/follow_up/activate/nurture）
// 与 3 种旧版节点（message/action/send_offer）行为高度同构：
//  1. 三级降级获取消息内容：node.Prompt 模板 > ScriptTemplate 关键词匹配 > LLM 即时生成
//  2. 幂等性检查：同节点重试不重复发送
//  3. 持久化到 session_messages 表（与 SmartCSOrchestrator 一致）
//  4. WebSocket 推送给前端
//
// 私域部署：消息出站走 DB+WS 双写，由前端 / visitor 端拉取，
// 不直接调用 platform.PlatformAdapter.SendMessage（避免账号 cookie 依赖）。
type MessageNodeBase struct {
	nodeType   string
	scenario   llm.DispatchScenario // LLM 降级时的调度场景
	dispatcher *llm.Dispatcher
	wsHub      *websocket.Hub
	msgRepo    *repository.SessionMessageRepository
}

// NodeType 返回节点类型
func (b *MessageNodeBase) NodeType() string { return b.nodeType }

// IsAsync 同步执行
func (b *MessageNodeBase) IsAsync() bool { return false }

// Execute 执行消息发送节点
func (b *MessageNodeBase) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	start := time.Now()
	execID := ec.Execution.ID
	nodeID := ec.Node.ID
	sideEffectKey := fmt.Sprintf("message_sent:%d:%s", execID, nodeID)

	// 1. 幂等性检查：已发送则跳过
	if hasSideEffect(ec.Execution, sideEffectKey) {
		logger.Ctx(ctx).Info().
			Str("node_id", nodeID).
			Str("side_effect", sideEffectKey).
			Msg("message already sent, skipping (idempotent)")
		return &NodeExecResult{
			Status: NodeStatusSkipped,
			Output: model.JSONMap{},
		}, nil
	}

	// 2. 三级降级获取消息内容
	content, contentSource, err := b.resolveContent(ctx, ec)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str("node_id", nodeID).
			Str("node_type", b.nodeType).
			Msg("resolve message content failed")
		return &NodeExecResult{
			Status:       NodeStatusFailed,
			ErrorMessage: fmt.Sprintf("resolve content failed: %v", err),
			Retryable:    true,
		}, err
	}
	if strings.TrimSpace(content) == "" {
		// 内容为空视为失败（避免发空消息给客户）
		return &NodeExecResult{
			Status:       NodeStatusFailed,
			ErrorMessage: "resolved content is empty",
			Retryable:    true,
		}, fmt.Errorf("resolved content is empty for node %s", nodeID)
	}

	// 3. 持久化到 session_messages 表
	if b.msgRepo != nil && ec.SessionID != "" {
		msg := &model.SessionMessage{
			SessionID:   ec.SessionID,
			Content:     content,
			ContentType: model.MessageTypeText,
			SenderType:  "ai",
			SenderID:    fmt.Sprintf("sop_executor:%d", execID),
			SenderName:  "AI 销冠",
			AISource:    contentSource,
		}
		if err := b.msgRepo.Create(ctx, msg); err != nil {
			logger.Ctx(ctx).Warn().Err(err).
				Str("session_id", ec.SessionID).
				Msg("persist session message failed (non-fatal, will still push WS)")
		}
	}

	// 4. WebSocket 推送（私域部署：BroadcastToMerchant 全广播，前端按 session_id 过滤）
	if b.wsHub != nil {
		payload := map[string]any{
			"execution_id": execID,
			"node_id":      nodeID,
			"node_type":    b.nodeType,
			"session_id":   ec.SessionID,
			"customer_id":  ec.CustomerID,
			"content":      content,
			"source":       contentSource,
			"timestamp":    time.Now().Format(time.RFC3339),
		}
		_ = b.wsHub.BroadcastToMerchant("", websocket.MsgTypeSOP, payload)
	}

	// 5. 更新会话最后消息（best-effort）
	if ec.SessionID != "" {
		b.updateSessionLastMessage(ctx, ec.SessionID, content)
	}

	latencyMs := time.Since(start).Milliseconds()
	logger.Ctx(ctx).Info().
		Str("node_id", nodeID).
		Str("node_type", b.nodeType).
		Str("content_source", contentSource).
		Int64("latency_ms", latencyMs).
		Int("content_len", len(content)).
		Msg("message node executed")

	return &NodeExecResult{
		Status: NodeStatusCompleted,
		Output: model.JSONMap{
			fmt.Sprintf("_%s_content", b.nodeType): content,
			fmt.Sprintf("_%s_source", b.nodeType):  contentSource,
		},
		SideEffects: []string{sideEffectKey},
	}, nil
}

// resolveContent 三级降级获取消息内容
//
// 优先级：
//  1. node.Prompt（话术模板，支持 {{var}} 变量替换）
// 2. node.Config.script_keyword（ScriptTemplateService 关键词匹配，TODO: 集成）
//  3. LLM 即时生成（dispatcher.Dispatch，scenario 由节点类型决定）
//
// 返回 (content, source, error)，source ∈ {"prompt", "llm", "fallback"}
func (b *MessageNodeBase) resolveContent(ctx context.Context, ec *ExecutionContext) (string, string, error) {
	// 1. 优先使用 node.Prompt
	if ec.Node.Prompt != "" {
		content := renderPromptTemplate(ec.Node.Prompt, ec.ExecutionData)
		if strings.TrimSpace(content) != "" {
			return content, "prompt", nil
		}
	}

	// 2. 检查 node.Config.content（兼容旧版字段）
	if cfgContent, ok := ec.Node.Config["content"].(string); ok && strings.TrimSpace(cfgContent) != "" {
		return renderPromptTemplate(cfgContent, ec.ExecutionData), "config", nil
	}

	// 3. LLM 降级生成
	if b.dispatcher != nil {
		systemPrompt := fmt.Sprintf("你是一名销冠，正在执行 %s 节点。根据客户上下文生成一段简短、自然、专业的话术（不超过 100 字）。", b.nodeType)
		userPrompt := buildLLMUserPrompt(ec)
		req := llm.DispatchRequest{
			Scenario:     b.scenario,
			SystemPrompt: systemPrompt,
			Prompt:       userPrompt,
			MaxTokens:    200,
			Temperature:  0.7,
		}
		result, err := b.dispatcher.Dispatch(ctx, req)
		if err == nil && result != nil && strings.TrimSpace(result.Content) != "" {
			return strings.TrimSpace(result.Content), "llm", nil
		}
		// LLM 失败则继续降级
		logger.Ctx(ctx).Warn().Err(err).
			Str("node_type", b.nodeType).
			Msg("llm dispatch failed, using fallback")
	}

	// 4. 兜底：按节点类型返回默认话术
	return defaultScriptForNodeType(b.nodeType), "fallback", nil
}

// updateSessionLastMessage 更新会话最后消息（best-effort，失败不阻塞 SOP 流程）
func (b *MessageNodeBase) updateSessionLastMessage(ctx context.Context, sessionID, content string) {
	// 通过 repository.NewCustomerSessionRepository() 拿到 repo（避免循环依赖）
	// 此处使用全局 GetDB 模式（与既有 saveOutboundMessage 一致）
	repo := repository.NewCustomerSessionRepository()
	if repo == nil {
		return
	}
	_ = repo.UpdateLastMessageBySessionID(ctx, sessionID, content, "ai")
}

// buildLLMUserPrompt 构造 LLM 输入
func buildLLMUserPrompt(ec *ExecutionContext) string {
	parts := []string{
		fmt.Sprintf("当前节点：%s（%s）", ec.Node.Name, ec.Node.Type),
		fmt.Sprintf("客户 ID：%s", ec.CustomerID),
	}
	if ec.Node.Description != "" {
		parts = append(parts, fmt.Sprintf("节点说明：%s", ec.Node.Description))
	}
	if ec.ExecutionData != nil {
		if v, ok := ec.ExecutionData["customer_name"].(string); ok && v != "" {
			parts = append(parts, fmt.Sprintf("客户姓名：%s", v))
		}
		if v, ok := ec.ExecutionData["intent_score"].(float64); ok {
			parts = append(parts, fmt.Sprintf("意向分数：%.2f", v))
		}
		if v, ok := ec.ExecutionData["last_customer_message"].(string); ok && v != "" {
			parts = append(parts, fmt.Sprintf("客户最近消息：%s", v))
		}
	}
	return strings.Join(parts, "\n")
}

// renderPromptTemplate 渲染话术模板
//
// 支持 {{var}} 变量替换，变量来自 ExecutionData。
// 未匹配的变量保留原样（避免误删业务文本）。
func renderPromptTemplate(tmpl string, data model.JSONMap) string {
	if data == nil {
		return tmpl
	}
	out := tmpl
	for k, v := range data {
		placeholder := fmt.Sprintf("{{%s}}", k)
		if strings.Contains(out, placeholder) {
			out = strings.ReplaceAll(out, placeholder, fmt.Sprintf("%v", v))
		}
	}
	return out
}

// defaultScriptForNodeType 节点类型默认话术（兜底，避免 LLM 不可用时无消息可发）
func defaultScriptForNodeType(nodeType string) string {
	switch nodeType {
	case SOPNodeTypeGreeting:
		return "您好，欢迎咨询，我是您的专属顾问，请问有什么可以帮您？"
	case SOPNodeTypeInquire:
		return "请问您主要想了解哪方面的产品或服务？"
	case SOPNodeTypeIntroduce:
		return "我们的产品具有以下核心优势，我为您简要介绍一下。"
	case SOPNodeTypeHandle:
		return "理解您的考虑，这是非常重要的因素，我们一起来分析一下。"
	case SOPNodeTypeClose:
		return "针对您的情况，我们可以提供专属优惠，您看是否方便确认一下？"
	case SOPNodeTypeInvite:
		return "诚邀您参与我们的体验活动，名额有限，期待您的加入。"
	case SOPNodeTypeFollowUp:
		return "好的，我先把相关资料发给您，后续有任何问题随时联系我。"
	case SOPNodeTypeActivate:
		return "好久不见，最近我们推出了新的产品方案，想与您分享一下。"
	case SOPNodeTypeNurture:
		return "感谢您的关注，我先为您提供一些参考资料，您可以慢慢了解。"
	case SOPNodeTypeMessage, SOPNodeTypeAction:
		return "您好，有什么可以帮您的吗？"
	case SOPNodeTypeSendOffer:
		return "针对您这样的优质客户，我们提供专属优惠方案。"
	default:
		return "您好，我是您的专属顾问。"
	}
}

// SetWSHub 注入 WebSocket Hub（用于运行时注入，修复 #6 不一致点）
//
// 设计：sop_dispatcher 启动时 WSHub 为 nil（避免循环依赖），
// 启动后由 main.go 调用 dispatcher.SetWSHub(hub) 注入，再由 dispatcher
// 调用每个 MessageNodeBase.SetWSHub 真正注入到执行器。
func (b *MessageNodeBase) SetWSHub(ctx context.Context, hub *websocket.Hub) {
	b.wsHub = hub
}

// NewMessageNodeExecutor 创建消息发送类节点执行器
func NewMessageNodeExecutor(nodeType string, scenario llm.DispatchScenario, deps *SOPNodeExecutorDeps) *MessageNodeBase {
	return &MessageNodeBase{
		nodeType:   nodeType,
		scenario:   scenario,
		dispatcher: deps.Dispatcher,
		wsHub:      deps.WSHub,
		msgRepo:    deps.MsgRepo,
	}
}

// ============================================================================
// 3. 条件路由类执行器：condition / branch
// ============================================================================

// ConditionExecutor 条件分支节点执行器
//
// 调用 SOPEvaluateConditionBranches 评估优先级分支，返回 NextNodeID。
// 评估失败兜底走 Next[0]。
type ConditionExecutor struct {
	nodeType string // condition 或 branch（旧版）
}

// NodeType 返回节点类型
func (e *ConditionExecutor) NodeType() string { return e.nodeType }

// IsAsync 同步执行
func (e *ConditionExecutor) IsAsync() bool { return false }

// Execute 评估条件分支
func (e *ConditionExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	// 优先用 Conditions 字段（新版优先级路由）
	if len(ec.Node.Conditions) > 0 {
		br, err := SOPEvaluateConditionBranches(ec.Node.Conditions, ec.ExecutionData)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).
				Str("node_id", ec.Node.ID).
				Msg("evaluate condition branches failed, fallback to Next[0]")
		} else if br.Matched && br.NextNode != "" {
			// 查找匹配分支的 Label（SOPConditionResult 仅返回 NextNode，需回溯分支）
			matchedLabel := ""
			for _, branch := range ec.Node.Conditions {
				if branch.Next == br.NextNode {
					matchedLabel = branch.Label
					break
				}
			}
			logger.Ctx(ctx).Info().
				Str("node_id", ec.Node.ID).
				Str("matched_branch", matchedLabel).
				Str("next_node", br.NextNode).
				Msg("condition branch matched")
			return &NodeExecResult{
				Status:     NodeStatusCompleted,
				Output:     model.JSONMap{"_condition_branch": matchedLabel},
				NextNodeID: br.NextNode,
			}, nil
		}
	}

	// 旧版 Condition 字段兜底
	if ec.Node.Condition != "" {
		result, err := SOPEvaluateNodeCondition(ec.Node, ec.ExecutionData)
		if err == nil {
			if nextID, ok := result["_next_node"].(string); ok && nextID != "" {
				return &NodeExecResult{
					Status:     NodeStatusCompleted,
					Output:     model.JSONMap{"_condition_result": result},
					NextNodeID: nextID,
				}, nil
			}
		}
	}

	// 兜底：Next[0]
	nextID := ""
	if len(ec.Node.Next) > 0 {
		nextID = ec.Node.Next[0]
	}
	logger.Ctx(ctx).Info().
		Str("node_id", ec.Node.ID).
		Str("fallback_next", nextID).
		Msg("condition node fallback to Next[0]")
	return &NodeExecResult{
		Status:     NodeStatusCompleted,
		Output:     model.JSONMap{"_condition_fallback": true},
		NextNodeID: nextID,
	}, nil
}

// ============================================================================
// 4. LLM 决策类执行器：llm / ai_decide
// ============================================================================

// LLMNodeExecutor LLM 决策节点执行器
//
// 通过全局信号量控制并发（默认 4），调用 dispatcher.Dispatch(ScenarioHighQuality, ...)，
// 要求 LLM 返回 JSON {"next_node_id":"...","reason":"..."}，
// 解析失败兜底走 Next[0]。
type LLMNodeExecutor struct {
	nodeType   string // llm 或 ai_decide（旧版）
	dispatcher *llm.Dispatcher
	llmSem     chan struct{}
}

// NodeType 返回节点类型
func (e *LLMNodeExecutor) NodeType() string { return e.nodeType }

// IsAsync 同步执行（LLM 调用阻塞，但通过信号量限流）
func (e *LLMNodeExecutor) IsAsync() bool { return false }

// Execute 调用 LLM 决策下一节点
func (e *LLMNodeExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	// 全局 LLM 并发信号量限流
	if e.llmSem != nil {
		select {
		case e.llmSem <- struct{}{}:
			defer func() { <-e.llmSem }()
		case <-ctx.Done():
			return &NodeExecResult{
				Status:       NodeStatusFailed,
				ErrorMessage: "llm semaphore wait timeout: " + ctx.Err().Error(),
				Retryable:    true,
			}, ctx.Err()
		}
	}

	if e.dispatcher == nil {
		return &NodeExecResult{
			Status:       NodeStatusFailed,
			ErrorMessage: "llm dispatcher not configured",
			Retryable:    false,
		}, fmt.Errorf("llm dispatcher not configured for node %s", ec.Node.ID)
	}

	// 构造 LLM 输入：候选下一节点列表 + 当前上下文
	candidates := ec.Node.Next
	prompt := buildLLMDecisionPrompt(ec, candidates)
	req := llm.DispatchRequest{
		Scenario:     llm.ScenarioHighQuality,
		SystemPrompt: "你是一名销冠决策助手。根据当前对话上下文，从候选节点中选择最合适的下一节点。只返回 JSON。",
		Prompt:       prompt,
		MaxTokens:    200,
		Temperature:  0.3,
		JSONMode:     true,
	}

	result, err := e.dispatcher.Dispatch(ctx, req)
	if err != nil || result == nil {
		logger.Ctx(ctx).Warn().Err(err).
			Str("node_id", ec.Node.ID).
			Msg("llm dispatch failed, fallback to Next[0]")
		return &NodeExecResult{
			Status:     NodeStatusCompleted,
			Output:     model.JSONMap{"_llm_error": fmt.Sprintf("%v", err)},
			NextNodeID: firstOrEmpty(candidates),
			TokensUsed: 0,
		}, nil
	}

	// 解析 LLM 返回的 JSON
	decision, parseErr := parseLLMDecision(result.Content)
	if parseErr != nil || decision.NextNodeID == "" {
		logger.Ctx(ctx).Warn().Err(parseErr).
			Str("node_id", ec.Node.ID).
			Str("raw_content", result.Content).
			Msg("parse llm decision failed, fallback to Next[0]")
		return &NodeExecResult{
			Status:     NodeStatusCompleted,
			Output:     model.JSONMap{"_llm_raw": result.Content},
			NextNodeID: firstOrEmpty(candidates),
			TokensUsed: result.TotalTokens,
		}, nil
	}

	// 校验 LLM 返回的节点 ID 是否在候选列表中（防幻觉）
	if !containsString(candidates, decision.NextNodeID) {
		logger.Ctx(ctx).Warn().
			Str("node_id", ec.Node.ID).
			Str("llm_choice", decision.NextNodeID).
			Strs("candidates", candidates).
			Msg("llm returned invalid node id, fallback to Next[0]")
		return &NodeExecResult{
			Status:     NodeStatusCompleted,
			Output:     model.JSONMap{"_llm_invalid": decision.NextNodeID},
			NextNodeID: firstOrEmpty(candidates),
			TokensUsed: result.TotalTokens,
		}, nil
	}

	logger.Ctx(ctx).Info().
		Str("node_id", ec.Node.ID).
		Str("llm_choice", decision.NextNodeID).
		Str("reason", decision.Reason).
		Int("tokens", result.TotalTokens).
		Msg("llm decision made")

	return &NodeExecResult{
		Status: NodeStatusCompleted,
		Output: model.JSONMap{
			"_llm_decision": decision.NextNodeID,
			"_llm_reason":   decision.Reason,
		},
		NextNodeID: decision.NextNodeID,
		TokensUsed: result.TotalTokens,
	}, nil
}

// NewLLMNodeExecutor 创建 LLM 决策节点执行器
func NewLLMNodeExecutor(nodeType string, deps *SOPNodeExecutorDeps) *LLMNodeExecutor {
	return &LLMNodeExecutor{
		nodeType:   nodeType,
		dispatcher: deps.Dispatcher,
		llmSem:     deps.LLMSem,
	}
}

// llmDecision LLM 决策返回结构
type llmDecision struct {
	NextNodeID string `json:"next_node_id"`
	Reason     string `json:"reason"`
}

// parseLLMDecision 解析 LLM 返回的 JSON 决策
//
// 容错策略：
//  1. 直接 json.Unmarshal
//  2. 失败则尝试提取 {} 包裹的 JSON 子串
//  3. 仍失败则返回错误
func parseLLMDecision(content string) (*llmDecision, error) {
	content = strings.TrimSpace(content)
	// 直接解析
	var d llmDecision
	if err := json.Unmarshal([]byte(content), &d); err == nil {
		return &d, nil
	}
	// 提取 {} 子串
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		sub := content[start : end+1]
		if err := json.Unmarshal([]byte(sub), &d); err == nil {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("invalid llm decision json: %s", content)
}

// buildLLMDecisionPrompt 构造 LLM 决策提示词
func buildLLMDecisionPrompt(ec *ExecutionContext, candidates []string) string {
	parts := []string{
		fmt.Sprintf("当前节点：%s（%s）", ec.Node.Name, ec.Node.Type),
		fmt.Sprintf("候选下一节点：%s", strings.Join(candidates, ", ")),
	}
	if ec.Node.Prompt != "" {
		parts = append(parts, fmt.Sprintf("决策提示：%s", ec.Node.Prompt))
	}
	if ec.ExecutionData != nil {
		if v, ok := ec.ExecutionData["intent_score"].(float64); ok {
			parts = append(parts, fmt.Sprintf("意向分数：%.2f", v))
		}
		if v, ok := ec.ExecutionData["last_customer_message"].(string); ok && v != "" {
			parts = append(parts, fmt.Sprintf("客户最近消息：%s", v))
		}
	}
	parts = append(parts, "请返回 JSON：{\"next_node_id\":\"<候选节点ID>\",\"reason\":\"<选择理由>\"}")
	return strings.Join(parts, "\n")
}

// ============================================================================
// 5. 等待类执行器：wait
// ============================================================================

// WaitExecutor 等待节点执行器
//
// 根据 node.Config.wait_seconds / wait_until / wait_event 写入 sop_timers 表，
// 返回 Status=waiting，调度器据此将 Execution.WaitEvent 字段置位。
// 事件等待默认 24h 超时防卡死。
//
// 五层架构：DB 操作通过 timerRepo 完成。
// 注意：保留 db 字段仅为兼容既有测试（&WaitExecutor{db: nil}），
// 实际 DB 操作由 timerRepo 完成；timerRepo == nil 时跳过 DB 写入。
type WaitExecutor struct {
	db        *gorm.DB // 保留字段以兼容既有测试（&WaitExecutor{db: nil}）
	timerRepo *repository.SOPTimerRepository
}

// NodeType 返回节点类型
func (e *WaitExecutor) NodeType() string { return SOPNodeTypeWait }

// IsAsync 异步执行（不阻塞 Worker Pool）
func (e *WaitExecutor) IsAsync() bool { return true }

// Execute 设置等待状态
func (e *WaitExecutor) Execute(ctx context.Context, ec *ExecutionContext) (*NodeExecResult, error) {
	waitSeconds, _ := ec.Node.Config["wait_seconds"].(float64)
	waitEventStr, _ := ec.Node.Config["wait_event"].(string)
	waitUntilStr, _ := ec.Node.Config["wait_until"].(string)

	// 计算等待截止时间
	var waitUntil time.Time
	waitEvent := WaitEventTimer // 默认 timer

	if waitUntilStr != "" {
		// 绝对时间（RFC3339）
		if t, err := time.Parse(time.RFC3339, waitUntilStr); err == nil {
			waitUntil = t
		}
	}
	if waitUntil.IsZero() && waitSeconds > 0 {
		waitUntil = time.Now().Add(time.Duration(int64(waitSeconds)) * time.Second)
	}
	if waitUntil.IsZero() {
		// 事件等待：默认 24h 超时防卡死
		waitEvent = WaitEventCustomerReply
		if waitEventStr != "" {
			waitEvent = waitEventStr
		}
		waitUntil = time.Now().Add(24 * time.Hour)
	} else if waitEventStr != "" {
		waitEvent = waitEventStr
	}

	// 写入 sop_timers 表（OutboxDispatcher 周期扫描）
	if e.timerRepo != nil {
		timer := &model.SOPTimer{
			ExecutionID: ec.Execution.ID,
			NodeID:      ec.Node.ID,
			WaitEvent:   waitEvent,
			WaitUntil:   waitUntil,
			Status:      "pending",
			Payload: model.JSONMap{
				"trace_id":    ec.TraceID,
				"customer_id": ec.CustomerID,
				"session_id":  ec.SessionID,
				"attempt":     ec.Attempt,
			},
		}
		if err := e.timerRepo.Create(ctx, timer); err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str("node_id", ec.Node.ID).
				Msg("create sop_timer failed")
			return &NodeExecResult{
				Status:       NodeStatusFailed,
				ErrorMessage: fmt.Sprintf("create timer failed: %v", err),
				Retryable:    true,
			}, err
		}
		logger.Ctx(ctx).Info().
			Uint("timer_id", timer.ID).
			Str("node_id", ec.Node.ID).
			Str("wait_event", waitEvent).
			Time("wait_until", waitUntil).
			Msg("sop timer created")
	}

	return &NodeExecResult{
		Status:    NodeStatusWaiting,
		Output:    model.JSONMap{"_wait_event": waitEvent, "_wait_until": waitUntil.Format(time.RFC3339)},
		WaitUntil: &waitUntil,
		WaitEvent: waitEvent,
	}, nil
}

// NewWaitExecutor 创建等待节点执行器
//
// 当 deps.DB 非 nil 时构造 timerRepo，否则 timerRepo 为 nil（Execute 会跳过 DB 写入）。
func NewWaitExecutor(deps *SOPNodeExecutorDeps) *WaitExecutor {
	e := &WaitExecutor{db: deps.DB}
	if deps.DB != nil {
		e.timerRepo = repository.NewSOPTimerRepository(deps.DB)
	}
	return e
}

// ============================================================================
// 6. 旧版节点兼容执行器：message / action / send_offer / ai_decide / branch
// ============================================================================

// 旧版节点映射到新执行器（通过 RegisterLegacyExecutors 注册）

// RegisterAllNodeExecutors 注册所有 14 种节点执行器 + 5 种旧版兼容执行器
//
// 应在 SOPExecutionDispatcher 初始化时调用一次。
// 重复注册会 panic（启动期错误）。
func RegisterAllNodeExecutors(registry *NodeExecutorRegistry, deps *SOPNodeExecutorDeps) {
	// 1. 空动作类
	registry.Register(context.Background(), &StartExecutor{})
	registry.Register(context.Background(), &EndExecutor{})

	// 2. 消息发送类（9 种商用节点）
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeGreeting, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeInquire, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeIntroduce, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeHandle, llm.ScenarioObjection, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeClose, llm.ScenarioHighQuality, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeInvite, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeFollowUp, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeActivate, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeNurture, llm.ScenarioFriendlyChat, deps))

	// 3. 条件路由类
	registry.Register(context.Background(), &ConditionExecutor{nodeType: SOPNodeTypeCondition})

	// 4. LLM 决策类
	registry.Register(context.Background(), NewLLMNodeExecutor(SOPNodeTypeLLM, deps))

	// 5. 等待类
	registry.Register(context.Background(), NewWaitExecutor(deps))

	// 6. 旧版节点兼容映射
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeMessage, llm.ScenarioFriendlyChat, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeAction, llm.ScenarioSOPReply, deps))
	registry.Register(context.Background(), NewMessageNodeExecutor(SOPNodeTypeSendOffer, llm.ScenarioObjection, deps))
	registry.Register(context.Background(), NewLLMNodeExecutor(SOPNodeTypeAIDecide, deps))
	registry.Register(context.Background(), &ConditionExecutor{nodeType: SOPNodeTypeBranch})

	logger.GetLogger().Info().
		Strs("registered_types", registry.AllRegistered(context.Background())).
		Msg("all sop node executors registered")
}

// ===== 辅助函数 =====

// firstOrEmpty 返回切片第一个元素，空切片返回空字符串
func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// containsString 检查字符串切片是否包含指定值
func containsString(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
