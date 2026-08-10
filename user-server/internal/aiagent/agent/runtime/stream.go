package agent_runtime

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// ============================================================================
// StreamHandler 流式输出支持（调研结果：流式输出）
// ----------------------------------------------------------------------------
// 设计目标：
//  1. 支持工具调用过程的实时流式输出
//  2. 支持多种事件类型（工具调用、工具结果、消息）
//  3. 支持错误处理和完成通知
//  4. 兼容LangChain stream_events格式
//
// 与业界标准对比：
//  - LangChain: stream_events API
//  - OpenAI: streaming function calls
//  - AutoGPT: 实时进度显示
// ============================================================================

// StreamEventType 流式事件类型
type StreamEventType string

const (
	StreamEventToolCall   StreamEventType = "tool_call"   // 工具调用开始
	StreamEventToolResult StreamEventType = "tool_result" // 工具调用结果
	StreamEventMessage    StreamEventType = "message"     // 消息输出
	StreamEventError      StreamEventType = "error"       // 错误事件
	StreamEventComplete   StreamEventType = "complete"    // 完成事件
	StreamEventStageStart StreamEventType = "stage_start" // 阶段开始
	StreamEventStageEnd   StreamEventType = "stage_end"   // 阶段结束
)

// StreamEvent 流式事件
type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	ToolName  string          `json:"tool_name,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	StageName string          `json:"stage_name,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Duration  time.Duration   `json:"duration,omitempty"`
}

// StreamHandler 流式处理器接口
type StreamHandler interface {
	// OnEvent 处理流式事件
	OnEvent(event StreamEvent)
	// OnError 处理错误
	OnError(err error)
	// OnComplete 处理完成
	OnComplete()
}

// ============================================================================
// BufferedStreamHandler 缓冲流式处理器
// ============================================================================

// BufferedStreamHandler 缓冲流式处理器
type BufferedStreamHandler struct {
	events   []StreamEvent
	mu       sync.Mutex
	callback func(event StreamEvent)
}

// NewBufferedStreamHandler 创建缓冲流式处理器
func NewBufferedStreamHandler(callback func(event StreamEvent)) *BufferedStreamHandler {
	return &BufferedStreamHandler{
		events:   make([]StreamEvent, 0),
		callback: callback,
	}
}

// OnEvent 处理流式事件
func (h *BufferedStreamHandler) OnEvent(event StreamEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event.Timestamp = time.Now()
	h.events = append(h.events, event)

	if h.callback != nil {
		h.callback(event)
	}
}

// OnError 处理错误
func (h *BufferedStreamHandler) OnError(err error) {
	h.OnEvent(StreamEvent{
		Type:  StreamEventError,
		Error: err.Error(),
	})
}

// OnComplete 处理完成
func (h *BufferedStreamHandler) OnComplete() {
	h.OnEvent(StreamEvent{
		Type: StreamEventComplete,
	})
}

// Events 获取所有事件
func (h *BufferedStreamHandler) Events() []StreamEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	events := make([]StreamEvent, len(h.events))
	copy(events, h.events)
	return events
}

// Clear 清空事件
func (h *BufferedStreamHandler) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = make([]StreamEvent, 0)
}

// ============================================================================
// ChannelStreamHandler 通道流式处理器
// ============================================================================

// ChannelStreamHandler 通道流式处理器
type ChannelStreamHandler struct {
	channel chan StreamEvent
	done    chan struct{}
}

// NewChannelStreamHandler 创建通道流式处理器
func NewChannelStreamHandler(bufferSize int) *ChannelStreamHandler {
	return &ChannelStreamHandler{
		channel: make(chan StreamEvent, bufferSize),
		done:    make(chan struct{}),
	}
}

// OnEvent 处理流式事件
func (h *ChannelStreamHandler) OnEvent(event StreamEvent) {
	event.Timestamp = time.Now()
	select {
	case h.channel <- event:
	default:
		// 通道满，丢弃事件
	}
}

// OnError 处理错误
func (h *ChannelStreamHandler) OnError(err error) {
	h.OnEvent(StreamEvent{
		Type:  StreamEventError,
		Error: err.Error(),
	})
}

// OnComplete 处理完成
func (h *ChannelStreamHandler) OnComplete() {
	h.OnEvent(StreamEvent{
		Type: StreamEventComplete,
	})
	close(h.channel)
	close(h.done)
}

// Events 获取事件通道
func (h *ChannelStreamHandler) Events() <-chan StreamEvent {
	return h.channel
}

// Done 获取完成信号
func (h *ChannelStreamHandler) Done() <-chan struct{} {
	return h.done
}

// ============================================================================
// CompositeStreamHandler 组合流式处理器
// ============================================================================

// CompositeStreamHandler 组合流式处理器
type CompositeStreamHandler struct {
	handlers []StreamHandler
	mu       sync.RWMutex
}

// NewCompositeStreamHandler 创建组合流式处理器
func NewCompositeStreamHandler(handlers ...StreamHandler) *CompositeStreamHandler {
	return &CompositeStreamHandler{
		handlers: handlers,
	}
}

// AddHandler 添加处理器
func (h *CompositeStreamHandler) AddHandler(handler StreamHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers = append(h.handlers, handler)
}

// OnEvent 处理流式事件
func (h *CompositeStreamHandler) OnEvent(event StreamEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, handler := range h.handlers {
		handler.OnEvent(event)
	}
}

// OnError 处理错误
func (h *CompositeStreamHandler) OnError(err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, handler := range h.handlers {
		handler.OnError(err)
	}
}

// OnComplete 处理完成
func (h *CompositeStreamHandler) OnComplete() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, handler := range h.handlers {
		handler.OnComplete()
	}
}

// ============================================================================
// InferenceCycle 流式输出支持
// ============================================================================

// RunOnceStream 执行一次完整推理闭环（支持流式输出）
//
// 参数：
//   - ctx: 上下文
//   - payload: 客户消息载荷
//   - agentCtx: 智能体上下文
//   - handler: 流式处理器
//
// 返回：
//   - InferenceDecision: 最终决策
//   - error: 仅在编排器本身失败时返回错误
func (c *InferenceCycle) RunOnceStream(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext, handler StreamHandler) (*InferenceDecision, error) {
	if c.stopped {
		return nil, ErrRuntimeStopped
	}

	start := time.Now()

	// 默认 agentCtx
	if agentCtx == nil {
		agentCtx = &AgentContext{
			AgentID:   0,
			AgentCode: "default",
			Name:      "默认智能体",
			AgentType: "customer_service",
			LoadedAt:  time.Now(),
		}
	}

	// 构造总超时 ctx
	tctx, cancel := context.WithTimeout(ctx, c.TotalTimeout)
	defer cancel()

	ic := &InferenceContext{
		Payload:   payload,
		AgentCtx:  agentCtx,
		StartTime: start,
		Stages:    []StageDecision{},
		Decision: InferenceDecision{
			ReplyType:  "text",
			Confidence: 0,
		},
	}

	// E1 补全：读取跨会话情境记忆
	c.mu.RLock()
	provider := c.memoryProvider
	c.mu.RUnlock()
	if provider != nil {
		mem, err := provider.LoadEpisodicMemory(tctx, payload.SessionID, payload.CustomerID)
		if err != nil {
			logger.Warnf("[inference_cycle] load episodic memory failed session=%s customer=%s err=%v",
				payload.SessionID, payload.CustomerID, err)
		} else if mem != "" {
			ic.EpisodicMemory = mem
		}
	}

	// 阶段执行列表
	stages := []InferenceStage{
		c.PerceptionStage,
		c.AlignmentStage,
		c.GatekeeperStage,
		c.PlannerStage,
	}

	// 顺序执行
	for _, stage := range stages {
		if stage == nil {
			continue
		}

		// 发送阶段开始事件
		if handler != nil {
			handler.OnEvent(StreamEvent{
				Type:      StreamEventStageStart,
				StageName: stage.Name(),
			})
		}

		// 阶段超时
		sctx, scancel := context.WithTimeout(tctx, c.StageTimeout)
		stageStart := time.Now()
		result := stage.Execute(sctx, ic)
		stageDuration := time.Since(stageStart)
		scancel()

		// 发送阶段结束事件
		if handler != nil {
			handler.OnEvent(StreamEvent{
				Type:      StreamEventStageEnd,
				StageName: stage.Name(),
				Duration:  stageDuration,
			})
		}

		// 错误处理
		if result.Error != nil {
			logger.Warnf("[inference_cycle] stage=%s error=%v", stage.Name(), result.Error)
			if handler != nil {
				handler.OnError(result.Error)
			}
		}

		// 早退判定
		if result.EarlyReturn {
			if result.Decision != nil {
				ic.Decision = mergeDecision(ic.Decision, *result.Decision)
			}
			ic.Decision.TotalDuration = time.Since(start)
			ic.Decision.Crisis = ic.Crisis
			ic.Decision.Sentiment = ic.Sentiment
			ic.Decision.Intent = ic.Intent
			ic.Decision.Alignment = ic.Alignment
			c.recordStats(ic.Decision)

			if handler != nil {
				handler.OnComplete()
			}

			logger.Infof("[inference_cycle] early return at stage=%s handoff=%v reason=%s duration=%s",
				stage.Name(), ic.Decision.HandoffToHuman, ic.Decision.StopReason, ic.Decision.TotalDuration)
			return &ic.Decision, nil
		}
	}

	// 全部阶段完成：聚合最终决策
	ic.Decision.TotalDuration = time.Since(start)
	if ic.Plan != nil {
		ic.Decision.ReplyType = "text"
		ic.Decision.Confidence = ic.Plan.Confidence
		if ic.Plan.SkipLLM {
			ic.Decision.Reply = ic.Plan.ReplyHint
			ic.Decision.ReplyType = "text"
			ic.Decision.StopReason = "faq_skip_llm"
		} else {
			ic.Decision.StopReason = "plan_ready"
		}
	} else {
		ic.Decision.StopReason = "no_plan"
	}
	ic.Decision.Crisis = ic.Crisis
	ic.Decision.Sentiment = ic.Sentiment
	ic.Decision.Intent = ic.Intent
	ic.Decision.Alignment = ic.Alignment

	c.recordStats(ic.Decision)

	if handler != nil {
		handler.OnComplete()
	}

	logger.Infof("[inference_cycle] completed trace=%s stages=%d plan_type=%s confidence=%.2f duration=%s",
		payload.TraceID, len(ic.Stages),
		planTypeOf(ic.Plan), ic.Decision.Confidence, ic.Decision.TotalDuration)

	return &ic.Decision, nil
}
