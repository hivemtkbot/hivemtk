package tooluse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 流式状态机：双拦截核心
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/工具链调用逻辑.md
//
// 核心设计：
//  - LLM 流式输出按"chunk"逐个到达（每个 chunk 是 SSE 增量）
//  - 状态机逐 chunk 推进，状态转换决定"是否继续向前端推送"
//  - 6 个状态：Normal / Detected / Parsing / Executing / Reassembling / Done
//
// 双拦截含义：
//  - 第一次拦截：检测到"调用工具："触发词时，立即"掐断"前端推送（不再 SendToClient）
//  - 第二次拦截：LLM 二进宫（第二次推理）时若再次触发工具调用，需拦截后合并执行
// ============================================================================

// StreamState 状态枚举
type StreamState string

const (
	// StateNormal 正常文本流模式
	StateNormal StreamState = "normal"
	// StateDetected 检测到"调用工具："触发词，已掐断前端
	StateDetected StreamState = "detected"
	// StateParsing 正在解析工具 JSON（吞下 chunk，攒到 buffer）
	StateParsing StreamState = "parsing"
	// StateExecuting 工具执行中（同步等结果）
	StateExecuting StreamState = "executing"
	// StateReassembling 二次组装（"二进宫"，把工具结果回填 LLM 让其生成最终回复）
	StateReassembling StreamState = "reassembling"
	// StateDone 状态机结束
	StateDone StreamState = "done"
)

// StreamEvent 状态机事件
type StreamEvent string

const (
	// EventChunk 收到新 chunk
	EventChunk StreamEvent = "chunk"
	// EventTriggerDetected 检测到触发词
	EventTriggerDetected StreamEvent = "trigger_detected"
	// EventJSONComplete JSON 完整（检测到闭合括号）
	EventJSONComplete StreamEvent = "json_complete"
	// EventParseError JSON 解析失败
	EventParseError StreamEvent = "parse_error"
	// EventToolExecuted 工具执行完成
	EventToolExecuted StreamEvent = "tool_executed"
	// EventReassembleReady 二进宫组装就绪
	EventReassembleReady StreamEvent = "reassemble_ready"
	// EventStreamEnd 流结束
	EventStreamEnd StreamEvent = "stream_end"
	// EventTimeout 状态机超时
	EventTimeout StreamEvent = "timeout"
)

// StreamStateMachine 流式状态机
//
// 用途：在 LLM 流式输出过程中，识别工具调用触发词、截取 JSON、执行工具、二次组装
//
// 典型用法：
//
//	sm := NewStreamStateMachine()
//	sm.SetTrigger("调用工具：")
//	sm.SetToolExecutor(executor)
//	for chunk := range llmStream {
//	    action, err := sm.Process(ctx, chunk)
//	    switch action {
//	    case ActionForwardToClient: // 推到前端
//	    case ActionBuffer:          // 暂存
//	    case ActionExecuteTool:     // 执行工具
//	    case ActionDone:            // 结束
//	    }
//	}
type StreamStateMachine struct {
	mu sync.Mutex

	// 配置
	trigger   string // 触发词，如 "调用工具："
	maxBuffer int    // buffer 最大字节（防止异常累积）
	timeout   time.Duration

	// 状态
	state     StreamState
	buffer    strings.Builder // 当前累积的 buffer
	toolName  string          // 解析出的工具名
	toolArgs  map[string]any  // 解析出的参数
	jsonStart int             // JSON 开始位置（在 buffer 中的偏移）
	jsonEnd   int             // JSON 结束位置

	// 关联组件
	executor ToolExecutorAPI // 工具执行器接口

	// 历史
	transitions []StateTransition
	startTime   time.Time

	// 完成回调
	onToolCall func(toolName string, args map[string]any) // 检测到工具调用时回调
	onComplete func(reply string)                         // 状态机完成时回调
}

// StateTransition 状态转换记录
type StateTransition struct {
	From   StreamState `json:"from"`
	To     StreamState `json:"to"`
	Event  StreamEvent `json:"event"`
	Reason string      `json:"reason,omitempty"`
	At     time.Time   `json:"at"`
}

// ToolExecutorAPI 工具执行器接口（用于状态机）
//
// 状态机只需要"按名称+参数执行工具"的能力，通过接口解耦
type ToolExecutorAPI interface {
	ExecuteByName(ctx context.Context, toolName string, args map[string]any) (ToolResult, error)
}

// StreamAction 状态机处理结果（对调用方的指令）
type StreamAction string

const (
	// ActionForwardToClient 继续向前端推送
	ActionForwardToClient StreamAction = "forward"
	// ActionBuffer 暂存（不再推送）
	ActionBuffer StreamAction = "buffer"
	// ActionExecuteTool 执行工具
	ActionExecuteTool StreamAction = "execute_tool"
	// ActionDone 结束
	ActionDone StreamAction = "done"
	// ActionFail 失败
	ActionFail StreamAction = "fail"
)

// NewStreamStateMachine 默认状态机
func NewStreamStateMachine() *StreamStateMachine {
	return &StreamStateMachine{
		trigger:   "调用工具：",
		maxBuffer: 64 * 1024, // 64 KB
		timeout:   30 * time.Second,
		state:     StateNormal,
	}
}

// SetTrigger 设置触发词（默认"调用工具："）
func (sm *StreamStateMachine) SetTrigger(t string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.trigger = t
}

// SetExecutor 设置工具执行器
func (sm *StreamStateMachine) SetExecutor(exec ToolExecutorAPI) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.executor = exec
}

// SetOnToolCall 设置工具调用回调
func (sm *StreamStateMachine) SetOnToolCall(cb func(toolName string, args map[string]any)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onToolCall = cb
}

// SetOnComplete 设置完成回调
func (sm *StreamStateMachine) SetOnComplete(cb func(reply string)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onComplete = cb
}

// State 获取当前状态
func (sm *StreamStateMachine) State() StreamState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.state
}

// Process 处理一个 chunk
//
// 返回 (action, error)：
//   - ActionForwardToClient: 调用方应把 chunk 推给前端
//   - ActionBuffer: 调用方应丢弃 chunk（不推送）
//   - ActionExecuteTool: 调用方应执行工具（通过 sm.Executor）
//   - ActionDone: 流处理完成
//   - ActionFail: 处理失败（错误已写入 sm.LastError）
func (sm *StreamStateMachine) Process(ctx context.Context, chunk string) (StreamAction, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.startTime.IsZero() {
		sm.startTime = time.Now()
	}

	// 写入 buffer
	sm.buffer.WriteString(chunk)

	// 超时检查
	if time.Since(sm.startTime) > sm.timeout {
		sm.transition(StateDone, EventTimeout, "state machine timeout")
		return ActionFail, errors.New("stream state machine timeout")
	}

	// 防止 buffer 爆炸
	if sm.buffer.Len() > sm.maxBuffer {
		return ActionFail, errors.New("buffer overflow")
	}

	switch sm.state {
	case StateNormal:
		// 检测触发词
		if idx := strings.Index(sm.buffer.String(), sm.trigger); idx >= 0 {
			// 找到触发词
			prefix := sm.buffer.String()[:idx] // 触发词之前是正常文本
			// 清空 buffer，从 JSON 开始重新累积
			sm.buffer.Reset()
			sm.buffer.WriteString(prefix) // 保留前文
			sm.buffer.WriteString(chunk)  // 重新写入当前 chunk
			sm.jsonStart = 0
			sm.transition(StateDetected, EventTriggerDetected, "trigger found at "+itoa(idx))
			// 继续到 StateDetected 处理
			return sm.processDetected()
		}
		// 正常文本：清空 buffer（避免重复扫描）+ 推送
		sm.buffer.Reset()
		sm.buffer.WriteString(chunk)
		return ActionForwardToClient, nil

	case StateDetected, StateParsing:
		return sm.processDetected()

	case StateExecuting, StateReassembling:
		// 执行中或组装中，chunk 应被忽略（不推送）
		return ActionBuffer, nil

	case StateDone:
		return ActionDone, nil

	default:
		return ActionFail, errors.New("unknown state: " + string(sm.state))
	}
}

// processDetected 处理 Detected/Parsing 状态
//
// 在 Detected/Parsing 状态下累积 buffer，尝试找到完整 JSON
func (sm *StreamStateMachine) processDetected() (StreamAction, error) {
	// 找 JSON 开始（{）
	jsonStartIdx := strings.Index(sm.buffer.String(), "{")
	if jsonStartIdx < 0 {
		// 还没看到 {，继续等
		sm.state = StateParsing
		return ActionBuffer, nil
	}

	// 尝试匹配 JSON 结束
	endIdx, balanced := matchJSONEnd(sm.buffer.String(), jsonStartIdx)
	if !balanced {
		// JSON 还没结束
		sm.state = StateParsing
		return ActionBuffer, nil
	}

	// JSON 完整，提取
	jsonStr := sm.buffer.String()[jsonStartIdx : endIdx+1]
	var tc struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &tc); err != nil {
		sm.transition(StateDone, EventParseError, err.Error())
		return ActionFail, errors.New("parse tool json: " + err.Error())
	}

	if tc.Tool == "" {
		sm.transition(StateDone, EventParseError, "tool name empty")
		return ActionFail, errors.New("tool name empty")
	}

	sm.toolName = tc.Tool
	sm.toolArgs = tc.Args
	sm.jsonEnd = endIdx

	// 触发回调
	if sm.onToolCall != nil {
		sm.onToolCall(tc.Tool, tc.Args)
	}

	sm.transition(StateExecuting, EventJSONComplete, "parsed: "+tc.Tool)
	return ActionExecuteTool, nil
}

// MarkToolExecuted 标记工具执行完成（外部调用方在收到 ActionExecuteTool 后执行工具，然后调用本方法）
//
// 内部转换：Executing → Reassembling
func (sm *StreamStateMachine) MarkToolExecuted(_ ToolResult, _ error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.transition(StateReassembling, EventToolExecuted, "")
}

// MarkReassembled 标记二进宫组装完成
//
// 内部转换：Reassembling → Done
func (sm *StreamStateMachine) MarkReassembled() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.transition(StateDone, EventReassembleReady, "")
}

// MarkFailed 标记失败
func (sm *StreamStateMachine) MarkFailed(reason string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.transition(StateDone, EventParseError, reason)
}

// transition 状态转换
func (sm *StreamStateMachine) transition(to StreamState, event StreamEvent, reason string) {
	sm.transitions = append(sm.transitions, StateTransition{
		From:   sm.state,
		To:     to,
		Event:  event,
		Reason: reason,
		At:     time.Now(),
	})
	sm.state = to
}

// Transitions 获取所有状态转换记录
func (sm *StreamStateMachine) Transitions() []StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]StateTransition, len(sm.transitions))
	copy(out, sm.transitions)
	return out
}

// ToolName 获取解析出的工具名
func (sm *StreamStateMachine) ToolName() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.toolName
}

// ToolArgs 获取解析出的工具参数
func (sm *StreamStateMachine) ToolArgs() map[string]any {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.toolArgs
}

// Reset 重置（用于测试）
func (sm *StreamStateMachine) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = StateNormal
	sm.buffer.Reset()
	sm.toolName = ""
	sm.toolArgs = nil
	sm.jsonStart = 0
	sm.jsonEnd = 0
	sm.transitions = nil
	sm.startTime = time.Time{}
}

// ============================================================================
// 辅助函数
// ============================================================================

// matchJSONEnd 找 JSON 结束位置（考虑嵌套 {} 和字符串内的 {}）
//
// 返回 (endIdx, balanced)：
//   - balanced=true 时，endIdx 是匹配 '}' 的索引
//   - balanced=false 时，表示 JSON 还没结束
func matchJSONEnd(s string, start int) (int, bool) {
	if start >= len(s) || s[start] != '{' {
		return -1, false
	}

	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
