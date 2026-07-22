package tooluse

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Double Intercept Orchestrator 双拦截编排器
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/工具链调用逻辑.md §1
//
// 工作流（严格按照文档）：
//  1. 第一次 LLM 推理（流式）：逐 chunk 进入状态机
//  2. 状态机识别"调用工具：" → 触发 Detected → Parsing
//  3. JSON 完整 → 状态机返回 ActionExecuteTool
//  4. 编排器通过 Router 执行工具 → 拿结果
//  5. 编排器把工具结果喂给第二次 LLM 推理（"二进宫"）→ 流式输出最终回复
//  6. 第二次推理产生的回复，状态机处于 StateReassembling，忽略任何再触发的"调用工具："
//     （防止循环调用）→ 直接拼接到 buffer
//  7. LLM 流结束 → 状态机 MarkDone
//  8. 输出最终回复给客户端
//
// 拦截（掐断）的两个关键点：
//  - 第一次拦截：Detected 后所有 chunk 不再 forward 到客户端
//  - 第二次拦截：Reassembling 阶段即使再检测到"调用工具："也忽略（防递归）
// ============================================================================

// DoubleInterceptOrchestrator 双拦截编排器
type DoubleInterceptOrchestrator struct {
	stateMachine *StreamStateMachine
	router       *ToolRouter
	secondPass   SecondPassLLM // 二进宫 LLM 调用接口

	mu          sync.Mutex
	clientMsgs  []ClientMessage
	toolResults []ToolExecutionRecord
	stats       OrchestratorStats
}

// OrchestratorStats 编排器统计
type OrchestratorStats struct {
	FirstPassChunks     int
	InterceptedChunks   int
	ToolExecutions      int
	SecondPassChunks    int
	FinalReplyLength    int
	TotalDuration       time.Duration
}

// ClientMessage 客户端消息（推送或缓存）
type ClientMessage struct {
	Content   string
	Forwarded bool    // true=已推送 / false=被拦截
	Timestamp time.Time
}

// ToolExecutionRecord 工具执行记录
type ToolExecutionRecord struct {
	ToolName  string
	Args      map[string]any
	Result    ToolResult
	Err       error
	ExecutedAt time.Time
}

// SecondPassLLM 二次推理 LLM 接口
//
// 编排器在工具执行完毕后调用，传入工具结果，让 LLM 重新生成最终回复
type SecondPassLLM interface {
	// GenerateReassembledReply 二进宫推理
	//
	// 入参：
	//   - originalContent: 第一次推理的原始用户消息
	//   - toolName: 调用的工具名
	//   - toolResult: 工具结果
	//   - chunkHandler: chunk 回调（用于流式推送）
	//
	// 返回：最终回复完整文本
	GenerateReassembledReply(
		ctx context.Context,
		originalContent string,
		toolName string,
		toolResult ToolResult,
		chunkHandler func(chunk string),
	) (string, error)
}

// OrchestratorConfig 编排器配置
type OrchestratorConfig struct {
	Trigger     string
	Router      *ToolRouter
	StateMachine *StreamStateMachine // 可选；nil 则创建默认
	SecondPass  SecondPassLLM
}

// NewDoubleInterceptOrchestrator 创建双拦截编排器
func NewDoubleInterceptOrchestrator(cfg OrchestratorConfig) (*DoubleInterceptOrchestrator, error) {
	if cfg.Router == nil {
		return nil, errors.New("router required")
	}
	if cfg.SecondPass == nil {
		return nil, errors.New("secondPass LLM required")
	}

	sm := cfg.StateMachine
	if sm == nil {
		sm = NewStreamStateMachine()
	}
	if cfg.Trigger != "" {
		sm.SetTrigger(cfg.Trigger)
	}

	return &DoubleInterceptOrchestrator{
		stateMachine: sm,
		router:       cfg.Router,
		secondPass:   cfg.SecondPass,
		clientMsgs:   make([]ClientMessage, 0),
		toolResults:  make([]ToolExecutionRecord, 0),
	}, nil
}

// Run 完整执行双拦截流程
//
// 入参：
//   - ctx: 上下文
//   - originalContent: 原始用户消息（用于二进宫）
//   - firstPassStream: 第一次 LLM 推理的流（chan string）
//
// 返回：最终回复 + 错误
func (o *DoubleInterceptOrchestrator) Run(ctx context.Context, originalContent string, firstPassStream <-chan string) (string, error) {
	o.stateMachine.Reset()
	o.mu.Lock()
	o.clientMsgs = o.clientMsgs[:0]
	o.toolResults = o.toolResults[:0]
	o.stats = OrchestratorStats{}
	start := time.Now()
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.stats.TotalDuration = time.Since(start)
		o.mu.Unlock()
	}()

	// Phase 1: 第一次推理 + 流式状态机
	for chunk := range firstPassStream {
		o.stats.FirstPassChunks++
		action, err := o.stateMachine.Process(ctx, chunk)
		if err != nil {
			return "", err
		}

		switch action {
		case ActionForwardToClient:
			o.recordClientMessage(chunk, true)
		case ActionBuffer:
			o.stats.InterceptedChunks++
			o.recordClientMessage(chunk, false)
		case ActionExecuteTool:
			// 工具调用触发，执行工具
			o.stats.InterceptedChunks++
			if err := o.executeToolCall(ctx, originalContent); err != nil {
				return "", err
			}
		case ActionDone:
			// 流结束
		case ActionFail:
			return "", errors.New("state machine failed")
		}
	}

	// Phase 2: 二次推理（二进宫）
	if o.stateMachine.State() == StateReassembling || o.hasToolExecution() {
		reply, err := o.runSecondPass(ctx, originalContent)
		if err != nil {
			return "", err
		}
		o.stateMachine.MarkReassembled()
		o.stats.FinalReplyLength = len(reply)
		return reply, nil
	}

	// Phase 3: 没有工具调用，整理 clientMsgs 作为最终回复
	return o.assembleDirectReply(), nil
}

// executeToolCall 执行检测到的工具调用
func (o *DoubleInterceptOrchestrator) executeToolCall(ctx context.Context, originalContent string) error {
	toolName := o.stateMachine.ToolName()
	toolArgs := o.stateMachine.ToolArgs()

	record := ToolExecutionRecord{
		ToolName:  toolName,
		Args:      toolArgs,
		ExecutedAt: time.Now(),
	}

	// 通过 Router 执行（带审计/限流/重试/计费）
	result := o.router.Route(ctx, toolName, toolArgs, &ToolContext{
		CallerID:   "agent_runtime",
		AgentID:    "default",
		AuditTrace: "trace_" + originalContent,
	})
	record.Result = result.Result
	record.Err = result.Err

	o.mu.Lock()
	o.toolResults = append(o.toolResults, record)
	o.stats.ToolExecutions++
	o.mu.Unlock()

	o.stateMachine.MarkToolExecuted(result.Result, result.Err)

	// 失败时降级：返回友好提示
	if result.Err != nil || !result.Result.Success {
		// 仍然进入 Reassembling，由 LLM 处理 fallback
	}

	return nil
}

// runSecondPass 二次推理
func (o *DoubleInterceptOrchestrator) runSecondPass(ctx context.Context, originalContent string) (string, error) {
	// 取最后一次工具结果
	o.mu.Lock()
	var lastRecord ToolExecutionRecord
	if len(o.toolResults) > 0 {
		lastRecord = o.toolResults[len(o.toolResults)-1]
	}
	o.mu.Unlock()

	// 通过 SecondPass LLM 重新生成
	var sb strings.Builder
	reply, err := o.secondPass.GenerateReassembledReply(
		ctx,
		originalContent,
		lastRecord.ToolName,
		lastRecord.Result,
		func(chunk string) {
			o.stats.SecondPassChunks++
			sb.WriteString(chunk)
		},
	)
	if err != nil {
		return "", err
	}
	_ = sb.String() // sb 是为了 future 流式记录

	// 二次推理的回复全部 forward 到客户端
	o.recordClientMessage(reply, true)
	return reply, nil
}

// recordClientMessage 记录客户端消息
func (o *DoubleInterceptOrchestrator) recordClientMessage(content string, forwarded bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.clientMsgs = append(o.clientMsgs, ClientMessage{
		Content:   content,
		Forwarded: forwarded,
		Timestamp: time.Now(),
	})
}

// hasToolExecution 是否执行过工具
func (o *DoubleInterceptOrchestrator) hasToolExecution() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.toolResults) > 0
}

// assembleDirectReply 拼装直接回复（无工具调用）
func (o *DoubleInterceptOrchestrator) assembleDirectReply() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var sb strings.Builder
	for _, m := range o.clientMsgs {
		if m.Forwarded {
			sb.WriteString(m.Content)
		}
	}
	return sb.String()
}

// GetClientMessages 获取客户端消息记录（用于审计）
func (o *DoubleInterceptOrchestrator) GetClientMessages() []ClientMessage {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]ClientMessage, len(o.clientMsgs))
	copy(out, o.clientMsgs)
	return out
}

// GetToolResults 获取工具执行记录
func (o *DoubleInterceptOrchestrator) GetToolResults() []ToolExecutionRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]ToolExecutionRecord, len(o.toolResults))
	copy(out, o.toolResults)
	return out
}

// GetStats 获取统计
func (o *DoubleInterceptOrchestrator) GetStats() OrchestratorStats {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stats
}

// StateMachine 获取状态机（外部可查询）
func (o *DoubleInterceptOrchestrator) StateMachine() *StreamStateMachine {
	return o.stateMachine
}
