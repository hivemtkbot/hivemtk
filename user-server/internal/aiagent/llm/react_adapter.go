package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// react_adapter.go P2-A: ReAct prompting 适配器
//
// 设计目标：
//   让无 Function Calling（FC）能力的 LLM（如 mtk-llm 本地 Qwen2.5-3B-Instruct、
//   Llama 系列、ChatGLM 等开源模型）也能接入 Agent Loop，通过文本协议
//   （Thought/Action/Observation）完成工具调用。
//
// 协议设计（ReAct prompting）：
//   System Prompt 注入工具描述 + ReAct 范例
//   LLM 输出格式：
//     Thought: 我需要查询客户信息
//     Action: customer.search
//     Action Input: {"phone":"13800138000"}
//     (调用方解析后执行工具，将结果作为 Observation 回灌)
//     Observation: {"id":"cust-001","phone":"13800138000",...}
//     ...
//     Thought: 我已经获得客户信息，可以回复用户
//     Final Answer: 您好，客户信息如下...
//
// 适用场景：
//   - 本地 LLM 无 FC 能力（mtk-llm / llama.cpp 等）
//   - 私域部署不依赖云端 FC 能力
//   - 测试环境无 API key 时降级使用
//
// 与原生 FC 路径的差异：
//   1. 原生 FC：LLM 直接返回 tool_calls JSON 结构
//   2. ReAct：LLM 返回文本，需解析 Thought/Action/Action Input
//   3. 性能：ReAct 多一轮 token 生成，但兼容性更强

// ===== 错误定义 =====

var (
	// ErrReActParseFailed ReAct 解析失败（LLM 输出不符合协议）
	ErrReActParseFailed = fmt.Errorf("react parse failed")
	// ErrReActNoAction LLM 既无 Action 也无 Final Answer
	ErrReActNoAction = fmt.Errorf("react no action or final answer")
)

// ===== ReAct 解析器 =====

// ReActParseResult ReAct 解析结果
type ReActParseResult struct {
	Thought     string // Thought 文本（可为空）
	Action      string // 工具名（Action: xxx）
	ActionInput string // 工具参数 JSON（Action Input: {...}）
	FinalAnswer string // 最终回复（Final Answer: xxx）
	IsFinal     bool   // 是否是最终回复（含 Final Answer）
	RawContent  string // 原始 LLM 输出（用于回灌对话历史）
}

// reactActionRe 匹配 Action: xxx
var reactActionRe = regexp.MustCompile(`(?i)Action\s*:\s*([a-zA-Z0-9_.\-]+)`)

// reactActionInputRe 匹配 Action Input: {...}（多行 JSON）
var reactActionInputRe = regexp.MustCompile(`(?is)Action\s*Input\s*:\s*(\{.*?\})`)

// reactFinalAnswerRe 匹配 Final Answer: xxx
var reactFinalAnswerRe = regexp.MustCompile(`(?is)Final\s*Answer\s*:\s*(.+)`)

// reactThoughtRe 匹配 Thought: xxx（单行）
var reactThoughtRe = regexp.MustCompile(`(?is)Thought\s*:\s*(.+?)(?:\n\s*(?:Action|Final)|$)`)

// ParseReActResponse 解析 LLM ReAct 文本响应
//
// 返回值：
//   - 若含 Final Answer：IsFinal=true，FinalAnswer 非空
//   - 若含 Action：IsFinal=false，Action + ActionInput 非空
//   - 都不含：返回 ErrReActNoAction
func ParseReActResponse(content string) (*ReActParseResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: empty content", ErrReActParseFailed)
	}

	result := &ReActParseResult{RawContent: content}

	// 优先匹配 Final Answer（最终回复）
	if m := reactFinalAnswerRe.FindStringSubmatch(content); m != nil {
		result.IsFinal = true
		result.FinalAnswer = strings.TrimSpace(m[1])
		// 尝试提取 Thought（如有）
		if tm := reactThoughtRe.FindStringSubmatch(content); tm != nil {
			result.Thought = strings.TrimSpace(tm[1])
		}
		return result, nil
	}

	// 其次匹配 Action + Action Input（工具调用）
	if m := reactActionRe.FindStringSubmatch(content); m != nil {
		result.Action = strings.TrimSpace(m[1])
		// 提取 Action Input（JSON）
		if im := reactActionInputRe.FindStringSubmatch(content); im != nil {
			result.ActionInput = strings.TrimSpace(im[1])
		} else {
			// Action 无 Action Input，默认空对象
			result.ActionInput = "{}"
		}
		// 提取 Thought
		if tm := reactThoughtRe.FindStringSubmatch(content); tm != nil {
			result.Thought = strings.TrimSpace(tm[1])
		}
		return result, nil
	}

	// 都未匹配：可能是 LLM 直接回复（无 ReAct 协议）
	// 兼容性处理：将整段作为 Final Answer 返回（不强制要求 LLM 输出 Final Answer 前缀）
	return &ReActParseResult{
		IsFinal:     true,
		FinalAnswer: content,
		RawContent:  content,
	}, nil
}

// ToToolCall 将 ReAct Action 转换为 OpenAI 兼容的 ToolCall 结构
//
// 用于将 ReAct 解析结果注入到 DispatchResult.ToolCalls 中，
// 让上层 Agent Loop 可以统一处理 FC 和 ReAct 两种路径
func (r *ReActParseResult) ToToolCall(id string) *ToolCall {
	if r == nil || r.IsFinal || r.Action == "" {
		return nil
	}
	return &ToolCall{
		ID:   id,
		Type: "function",
		Function: ToolCallFunction{
			Name:      r.Action,
			Arguments: r.ActionInput,
		},
	}
}

// ===== ReAct 适配器 =====

// ReActAdapter 将无 FC 能力的 LLM 适配为 ReAct 协议
//
// 工作流程：
//  1. 在 Dispatch 调用前，将 Tools 转为 ReAct 系统提示词追加到 SystemPrompt
//  2. 在 Dispatch 调用后，解析 LLM 输出为 ReActParseResult
//  3. 若含 Action：构造 ToolCall 注入 DispatchResult.ToolCalls
//  4. 若含 Final Answer：直接作为 Content 返回
//
// 适配器是无状态的，可安全并发使用
type ReActAdapter struct {
	mu          sync.Mutex
	toolCallSeq uint64 // tool_call ID 生成器（线程安全）
}

// NewReActAdapter 创建 ReAct 适配器
func NewReActAdapter() *ReActAdapter {
	return &ReActAdapter{}
}

// WrapSystemPrompt 在原 SystemPrompt 后追加 ReAct 工具描述
//
// 参数：
//   - originalSystemPrompt: 原 SystemPrompt（可为空）
//   - tools: 可用工具列表
//
// 返回：追加 ReAct 协议描述后的 SystemPrompt
func (a *ReActAdapter) WrapSystemPrompt(originalSystemPrompt string, tools []ToolDefinition) string {
	var sb strings.Builder
	if originalSystemPrompt != "" {
		sb.WriteString(originalSystemPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("你是一个可以使用工具的智能体。请严格按照以下 ReAct 协议输出：\n\n")
	sb.WriteString("【可用工具】\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
		if t.Function.Parameters != nil {
			if b, err := json.Marshal(t.Function.Parameters); err == nil {
				sb.WriteString(fmt.Sprintf("  参数: %s\n", string(b)))
			}
		}
	}

	sb.WriteString("\n【输出格式】\n")
	sb.WriteString("需要调用工具时：\n")
	sb.WriteString("Thought: <你的思考过程>\n")
	sb.WriteString("Action: <工具名>\n")
	sb.WriteString("Action Input: <JSON 参数>\n\n")
	sb.WriteString("获得工具结果后，最终回复时：\n")
	sb.WriteString("Thought: <你的思考过程>\n")
	sb.WriteString("Final Answer: <最终回复给用户的内容>\n\n")
	sb.WriteString("注意：\n")
	sb.WriteString("1. Action 必须是【可用工具】列表中的工具名\n")
	sb.WriteString("2. Action Input 必须是合法 JSON\n")
	sb.WriteString("3. 收到 Observation 后才能输出 Final Answer\n")
	sb.WriteString("4. 不要编造工具未返回的数据\n")

	return sb.String()
}

// AdaptResult 将 LLM 文本响应适配为 DispatchResult
//
// 参数：
//   - result: 原 DispatchResult（含 LLM 文本输出）
//
// 返回：
//   - 若 LLM 输出含 Action：在 result.ToolCalls 注入对应 ToolCall，FinishReason 改为 "tool_calls"
//   - 若 LLM 输出含 Final Answer：result.Content 改为 Final Answer 文本
//   - 若都未匹配：保持原 result 不变（兼容性降级）
func (a *ReActAdapter) AdaptResult(result *DispatchResult) *DispatchResult {
	if result == nil || result.Content == "" {
		return result
	}

	parsed, err := ParseReActResponse(result.Content)
	if err != nil {
		// 解析失败：保持原 result 不变（降级为纯文本回复）
		return result
	}

	if parsed.IsFinal {
		// 含 Final Answer：使用 FinalAnswer 替换 Content
		// FinishReason 保持 "stop"
		result.Content = parsed.FinalAnswer
		return result
	}

	// 含 Action：构造 ToolCall 注入
	toolCallID := a.nextToolCallID()
	if tc := parsed.ToToolCall(toolCallID); tc != nil {
		result.ToolCalls = []ToolCall{*tc}
		result.FinishReason = "tool_calls"
		// 保留 Thought 作为 Content（便于上层调试）
		if parsed.Thought != "" {
			result.Content = parsed.Thought
		}
	}
	return result
}

// nextToolCallID 生成唯一 tool_call ID（线程安全）
//
// 格式：react_<unix_nano>_<seq>
// 用 nano + seq 确保并发场景下不重复
func (a *ReActAdapter) nextToolCallID() string {
	seq := atomic.AddUint64(&a.toolCallSeq, 1)
	return fmt.Sprintf("react_%d_%d", time.Now().UnixNano(), seq)
}

// ===== 工具：构造 Observation 回灌消息 =====

// BuildObservationMessage 构造 ReAct Observation 消息
//
// 在 Agent Loop 中，工具执行结果需要回灌给 LLM。
// 原生 FC 路径用 role=tool 消息，ReAct 路径用 role=user 消息
// （因为无 FC 能力 LLM 不识别 role=tool）
//
// 格式：
//
//	Observation: <工具返回 JSON>
func BuildObservationMessage(toolName, toolCallID, observation string) ChatMessage {
	content := fmt.Sprintf("Observation: %s\n", observation)
	return ChatMessage{
		Role:       "user",
		Content:    content,
		ToolCallID: toolCallID, // 保留关联（虽然 ReAct LLM 不使用，但便于审计）
		Name:       toolName,
	}
}

// IsReActMode 判断当前 DispatchRequest 是否走 ReAct 模式
//
// 触发条件：
//  1. 请求中含 Tools（智能体模式）
//  2. provider 配置中标记为 NoFC（无 Function Calling 能力）
//
// 简化判断：在 Dispatcher 层根据 provider 标记决定是否启用 ReAct
// 此处仅做工具是否存在的判断，provider 标记由 Dispatcher 处理
func IsReActMode(req *DispatchRequest, providerNoFC bool) bool {
	if providerNoFC && req != nil && len(req.Tools) > 0 {
		return true
	}
	return false
}
