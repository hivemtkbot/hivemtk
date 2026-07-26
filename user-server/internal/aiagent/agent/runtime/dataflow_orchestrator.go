package agent_runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 方向8: 核心数据流向编排器 (Core Data Flow Orchestrator)
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/核心数据流向.md
//
// 端到端数据流向：
//  客户消息 → InboxIngress 标准化 → AssetBundle 织布 (L1/L2/Prompt)
//           → InferenceCycle (感知→对齐→门禁→规划)
//           → 若转人工：HumanEscalation
//           → 若工具调用：StreamStateMachine → ToolRouter → 二进宫
//           → 裁剪 + 发布 → 客户
//
// 职责：
//  1. 把分散的子组件（感知/对齐/门禁/规划/工具/裁剪/发布）编排成一条数据流
//  2. 阶段间数据通过 InferenceContext 透传
//  3. 错误隔离：单阶段失败不阻塞整体
//  4. 全链路可观测：每阶段记录耗时与决策
// ============================================================================

// CoreDataFlowOrchestrator 核心数据流编排器
type CoreDataFlowOrchestrator struct {
	cycle    *InferenceCycle
	escalate EscalationTrigger

	// 可选组件（不注入则用 NoOp）
	assetLoader AssetLoader
	toolRouter  ToolRouterExecutor
	trimmer     ResponseTrimmer
	publisher   ResponsePublisher

	// 内部统计
	mu    sync.RWMutex
	stats DataFlowStats
}

// DataFlowStats 数据流统计
type DataFlowStats struct {
	TotalTasks       int64
	FastHandoffCount int64
	ToolCallCount    int64
	DirectReplyCount int64
	ErrorCount       int64
	AvgTotalDuration time.Duration
}

// EscalationTrigger 转人工触发器（解耦方向7 的 HumanEscalationManager）
type EscalationTrigger interface {
	Trigger(ctx context.Context, sessionID, reason string) error
}

// AssetLoader 资产包加载器（解耦方向9 的 AssetBundle）
type AssetLoader interface {
	LoadContext(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext) (*AssetContext, error)
}

// AssetContext 资产上下文
type AssetContext struct {
	L1ShortTerm map[string]string // L1 短期（Redis）
	L2Profile   map[string]string // L2 长期画像（PG）
	PromptText  string            // 资产包 Prompt
	SystemTools []string          // 可用工具列表
}

// ToolRouterExecutor 工具路由执行器（解耦方向5 的 ToolRouter）
type ToolRouterExecutor interface {
	Execute(ctx context.Context, toolName string, args map[string]any) (ToolOutput, error)
}

// ToolOutput 工具输出
type ToolOutput struct {
	Success bool
	Data    string
	Error   string
}

// ResponseTrimmer 响应裁剪器（裁掉末尾 JSON / 调试信息）
type ResponseTrimmer interface {
	Trim(reply string) string
}

// ResponsePublisher 响应发布器（推送到前端 SSE/WS）
type ResponsePublisher interface {
	Publish(ctx context.Context, channel, customerID, content string) error
}

// NewCoreDataFlowOrchestrator 构造编排器
func NewCoreDataFlowOrchestrator(cycle *InferenceCycle, escalate EscalationTrigger) *CoreDataFlowOrchestrator {
	if cycle == nil {
		cycle = NewInferenceCycle()
	}
	return &CoreDataFlowOrchestrator{
		cycle:       cycle,
		escalate:    escalate,
		assetLoader: noopAssetLoader{},
		toolRouter:  noopToolRouter{},
		trimmer:     defaultTrimmer{},
		publisher:   noopPublisher{},
	}
}

// SetAssetLoader 注入资产加载器
func (o *CoreDataFlowOrchestrator) SetAssetLoader(al AssetLoader) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.assetLoader = al
}

// SetToolRouter 注入工具路由
func (o *CoreDataFlowOrchestrator) SetToolRouter(tr ToolRouterExecutor) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.toolRouter = tr
}

// SetTrimmer 注入裁剪器
func (o *CoreDataFlowOrchestrator) SetTrimmer(t ResponseTrimmer) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.trimmer = t
}

// SetPublisher 注入发布器
func (o *CoreDataFlowOrchestrator) SetPublisher(p ResponsePublisher) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.publisher = p
}

// OrchestratorResult 编排结果
type OrchestratorResult struct {
	SessionID      string
	FinalReply     string
	HandoffToHuman bool
	HandoffReason  string
	ToolCallCount  int
	Stages         []StageDecision
	TotalDuration  time.Duration
	CrisisLevel    CrisisLevel
	AssetContext   *AssetContext
}

// Process 处理一次端到端数据流
//
// 入参：
//   - payload: 客户消息
//   - agentCtx: 智能体上下文
//
// 流程：
//
//	A1-A2: 入站已由 InboxIngress 处理（调用方保证 sessionID 存在）
//	A5:    加载资产上下文（L1/L2/Prompt）
//	A6-A9: 运行推理闭环（感知→对齐→门禁→规划）
//	B4:    若转人工，触发 HumanEscalation
//	A11:   裁剪 + 发布最终回复
func (o *CoreDataFlowOrchestrator) Process(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext) (*OrchestratorResult, error) {
	if agentCtx == nil {
		agentCtx = &AgentContext{
			AgentID:   0,
			AgentCode: "default",
			AgentType: "customer_service",
			LoadedAt:  time.Now(),
		}
	}

	start := time.Now()
	result := &OrchestratorResult{
		SessionID: payload.SessionID,
	}

	// 0. 解析 SessionID（若缺失则从 CustomerID 构造）
	if result.SessionID == "" {
		result.SessionID = payload.ChannelType + ":" + payload.CustomerID
	}

	// 1. A5: 加载资产上下文（L1 短期 + L2 画像 + Prompt）
	if o.assetLoader != nil {
		assetCtx, err := o.assetLoader.LoadContext(ctx, payload, agentCtx)
		if err != nil {
			logger.Warnf("[dataflow] asset load failed session=%s err=%v", result.SessionID, err)
		} else {
			result.AssetContext = assetCtx
		}
	}

	// 2. A6-A9: 推理闭环
	decision, err := o.cycle.RunOnce(ctx, payload, agentCtx)
	if err != nil {
		o.recordError()
		return result, err
	}
	if decision == nil {
		o.recordError()
		return result, errors.New("inference decision is nil")
	}
	result.HandoffToHuman = decision.HandoffToHuman
	result.HandoffReason = decision.HandoffReason
	result.CrisisLevel = decision.Crisis.Level
	result.TotalDuration = time.Since(start)

	// 3. B4: 若转人工 → 触发 HumanEscalation
	if decision.HandoffToHuman {
		if o.escalate != nil {
			if err := o.escalate.Trigger(ctx, result.SessionID, decision.HandoffReason); err != nil {
				logger.Warnf("[dataflow] escalate trigger failed session=%s err=%v", result.SessionID, err)
			}
		}
		o.recordHandoff()
	}

	// 4. A9-A11: 裁剪 + 发布
	finalReply := decision.Reply
	if finalReply == "" && decision.Plan != nil {
		finalReply = decision.Plan.ReplyHint
	}
	if o.trimmer != nil {
		finalReply = o.trimmer.Trim(finalReply)
	}
	result.FinalReply = finalReply

	if !decision.HandoffToHuman && finalReply != "" && o.publisher != nil {
		if err := o.publisher.Publish(ctx, payload.ChannelType, payload.CustomerID, finalReply); err != nil {
			logger.Warnf("[dataflow] publish failed session=%s err=%v", result.SessionID, err)
		}
	}

	// 5. 统计
	result.Stages = nil // 由调用方从 cycle 取
	if decision.Plan != nil {
		result.ToolCallCount = len(decision.Plan.ToolCalls)
		if result.ToolCallCount > 0 {
			o.recordToolCall()
		} else {
			o.recordDirectReply()
		}
	}
	o.mu.Lock()
	if o.stats.AvgTotalDuration == 0 {
		o.stats.AvgTotalDuration = result.TotalDuration
	} else {
		o.stats.AvgTotalDuration = (o.stats.AvgTotalDuration*9 + result.TotalDuration) / 10
	}
	o.stats.TotalTasks++
	o.mu.Unlock()

	logger.Infof("[dataflow] session=%s handoff=%v reply_len=%d duration=%s",
		result.SessionID, result.HandoffToHuman, len(result.FinalReply), result.TotalDuration)
	return result, nil
}

// GetStats 获取统计
func (o *CoreDataFlowOrchestrator) GetStats() DataFlowStats {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.stats
}

func (o *CoreDataFlowOrchestrator) recordHandoff() {
	o.mu.Lock()
	o.stats.FastHandoffCount++
	o.mu.Unlock()
}

func (o *CoreDataFlowOrchestrator) recordToolCall() {
	o.mu.Lock()
	o.stats.ToolCallCount++
	o.mu.Unlock()
}

func (o *CoreDataFlowOrchestrator) recordDirectReply() {
	o.mu.Lock()
	o.stats.DirectReplyCount++
	o.mu.Unlock()
}

func (o *CoreDataFlowOrchestrator) recordError() {
	o.mu.Lock()
	o.stats.ErrorCount++
	o.mu.Unlock()
}

// ============================================================================
// NoOp 默认实现
// ============================================================================

type noopAssetLoader struct{}

func (noopAssetLoader) LoadContext(_ context.Context, _ CustomerMessagePayload, _ *AgentContext) (*AssetContext, error) {
	return &AssetContext{
		L1ShortTerm: map[string]string{},
		L2Profile:   map[string]string{},
		PromptText:  "",
		SystemTools: []string{},
	}, nil
}

type noopToolRouter struct{}

func (noopToolRouter) Execute(_ context.Context, _ string, _ map[string]any) (ToolOutput, error) {
	return ToolOutput{Success: false, Error: "tool router not configured"}, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _, _, _ string) error {
	return nil
}

// defaultTrimmer 默认裁剪器：裁掉末尾 ```json {...}``` 块和 ```intent: {...}```
type defaultTrimmer struct{}

// Trim 裁剪末尾 JSON / 调试块
//
// 处理顺序：
//  1. 找最后一个 ```json 开头，看是否有匹配的 ``` 结束
//     若是 → 裁掉整段代码块
//  2. 找最后一个 {"intent" 开头，匹配大括号深度，切掉 JSON 段
func (defaultTrimmer) Trim(reply string) string {
	if reply == "" {
		return reply
	}

	// 1. 裁掉末尾 ```json ... ``` 代码块
	for {
		idx := strings.LastIndex(reply, "```json")
		if idx < 0 {
			break
		}
		// 从 idx 起找下一个 ```
		rest := reply[idx+len("```json"):]
		// 跳过 json 后的语言提示行（如 ```json\n）
		if nl := strings.Index(rest, "\n"); nl == 0 {
			rest = rest[1:]
		} else if nl > 0 {
			// 注意：若 nl 之前没有内容且整段只是 ```json
			// 跳到 nl 之后
			rest = rest[nl+1:]
		}
		closeIdx := strings.Index(rest, "```")
		if closeIdx < 0 {
			// 没有闭合，放弃裁剪
			break
		}
		// 裁掉 [idx, idx+len("```json")+nl+1+closeIdx+3) 这段
		before := reply[:idx]
		after := rest[closeIdx+3:]
		reply = strings.TrimRight(before, "\n") + after
	}

	// 2. 裁掉末尾 {"intent":...} JSON 段
	for {
		idx := strings.LastIndex(reply, "{\"intent\"")
		if idx < 0 {
			break
		}
		// 找匹配的右大括号
		depth := 0
		end := -1
		for i := idx; i < len(reply); i++ {
			if reply[i] == '{' {
				depth++
			} else if reply[i] == '}' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			break
		}
		// 裁掉 [idx, end+1)
		before := reply[:idx]
		// 留下一个换行，避免被前面字符粘住
		reply = strings.TrimRight(before, "\n")
	}

	return reply
}

// ============================================================================
// 适配方向7 的 HumanEscalationManager
// ============================================================================

// EscalationAdapter 把 HumanEscalationManager 适配成 EscalationTrigger
type EscalationAdapter struct {
	Fn func(ctx context.Context, sessionID, reason string) error
}

// Trigger 触发转人工
func (a EscalationAdapter) Trigger(ctx context.Context, sessionID, reason string) error {
	if a.Fn == nil {
		return nil
	}
	return a.Fn(ctx, sessionID, reason)
}
