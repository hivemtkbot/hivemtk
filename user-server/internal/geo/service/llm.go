package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/aiagent/llm"
)

// LLMAdapter 为 GEO 模块提供 LLM 访问，复用 hivemtk 全局 Dispatcher
// （DB 配置 + 场景路由 + 缓存 + 故障转移 + 可观测性）
type LLMAdapter struct {
	dispatcher *llm.Dispatcher
}

// NewLLMAdapter 创建 GEO LLM 适配器
func NewLLMAdapter() *LLMAdapter {
	return &LLMAdapter{dispatcher: llm.GetGlobalDispatcher()}
}

// LLMResult LLM 调用结果
type LLMResult struct {
	Content  string
	Provider string
	Model    string
}

// Generate 生成内容（使用 high_quality 场景路由）
func (a *LLMAdapter) Generate(ctx context.Context, systemPrompt, prompt string, temperature float64, maxTokens int) (*LLMResult, error) {
	if a.dispatcher == nil {
		return nil, fmt.Errorf("LLM dispatcher 未初始化")
	}
	req := llm.DispatchRequest{
		Scenario:     llm.ScenarioHighQuality,
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
	}
	resp, err := a.dispatcher.Dispatch(ctx, req)
	if err != nil {
		return nil, err
	}
	return &LLMResult{
		Content:  resp.Content,
		Provider: resp.Provider,
		Model:    resp.Model,
	}, nil
}

// GenerateJSON 生成 JSON 格式内容
func (a *LLMAdapter) GenerateJSON(ctx context.Context, systemPrompt, prompt string, maxTokens int) (*LLMResult, error) {
	if a.dispatcher == nil {
		return nil, fmt.Errorf("LLM dispatcher 未初始化")
	}
	req := llm.DispatchRequest{
		Scenario:     llm.ScenarioHighQuality,
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		Temperature:  0.3,
		MaxTokens:    maxTokens,
		JSONMode:     true,
	}
	resp, err := a.dispatcher.Dispatch(ctx, req)
	if err != nil {
		return nil, err
	}
	return &LLMResult{
		Content:  resp.Content,
		Provider: resp.Provider,
		Model:    resp.Model,
	}, nil
}
