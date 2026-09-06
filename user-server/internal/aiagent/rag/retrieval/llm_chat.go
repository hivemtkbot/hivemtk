package ragretrieval

import (
	"context"
	"fmt"

	"hivemtk-user/internal/aiagent/llm"
)

// LLMChatOptions LLM Chat 调用选项
type LLMChatOptions struct {
	Temperature  float64
	MaxTokens    int
	SystemPrompt string
}

// LLMChatClient LLM 对话客户端接口
//
// 抽象出 HyDE / Multi-Query / Contextual Retrieval 共同需要的"对话"能力，
// 解耦 ragretrieval 包对 *llm.Dispatcher 具体类型的依赖。
type LLMChatClient interface {
	Chat(ctx context.Context, prompt string, opts LLMChatOptions) (string, error)
}

// DispatcherChatAdapter 把 *llm.Dispatcher 适配为 LLMChatClient
//
// 现有 *llm.Dispatcher 通过 Dispatch(ctx, DispatchRequest) 调用，
// 这里桥接到 LLMChatClient.Chat(ctx, prompt, opts)。
// scenario 固定为 ScenarioLowCost（低成本批量场景），由 Dispatcher 内部路由到具体 provider。
type DispatcherChatAdapter struct {
	dispatcher *llm.Dispatcher
	scenario   llm.DispatchScenario
}

// NewDispatcherChatAdapter 创建适配器（默认 scenario=ScenarioLowCost）
func NewDispatcherChatAdapter(dispatcher *llm.Dispatcher) *DispatcherChatAdapter {
	return &DispatcherChatAdapter{
		dispatcher: dispatcher,
		scenario:   llm.ScenarioLowCost,
	}
}

// WithScenario 设置调度场景（默认 ScenarioLowCost）
//
// HyDE / Multi-Query 用默认低成本场景即可；
// Contextual Retrieval 建议使用 ScenarioLongSummary（长上下文）。
func (a *DispatcherChatAdapter) WithScenario(s llm.DispatchScenario) *DispatcherChatAdapter {
	a.scenario = s
	return a
}

// Chat 实现 LLMChatClient 接口
func (a *DispatcherChatAdapter) Chat(ctx context.Context, prompt string, opts LLMChatOptions) (string, error) {
	if a == nil || a.dispatcher == nil {
		return "", fmt.Errorf("LLM dispatcher 未初始化")
	}
	req := llm.DispatchRequest{
		Scenario:     a.scenario,
		Prompt:       prompt,
		SystemPrompt: opts.SystemPrompt,
		MaxTokens:    opts.MaxTokens,
		Temperature:  opts.Temperature,
	}
	result, err := a.dispatcher.Dispatch(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM dispatch 失败: %w", err)
	}
	return result.Content, nil
}

// NoopLLMChatClient 空实现（用于禁用查询改写场景）
//
// 调用 Chat 始终返回错误，使上层走"未启用"分支
type NoopLLMChatClient struct{}

// Chat 始终返回错误
func (NoopLLMChatClient) Chat(_ context.Context, _ string, _ LLMChatOptions) (string, error) {
	return "", fmt.Errorf("LLM chat disabled (noop client)")
}

var _ LLMChatClient = (*DispatcherChatAdapter)(nil)
var _ LLMChatClient = NoopLLMChatClient{}
