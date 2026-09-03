package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/service"
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
	Content      string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
}

// modelPrice 模型定价（USD / 1M tokens）
type modelPrice struct {
	aliases []string
	input   float64
	output  float64
}

// modelPrices 常见模型内置定价表（公开牌价的保守近似，用于成本报表估算而非计费）。
// 匹配策略：模型名小写后包含任一 alias 即命中（兼容 gpt-4o-2024-08-06 等带版本号名称）。
var modelPrices = []modelPrice{
	// 注意：alias 按子串包含匹配，长名（如 gpt-4o-mini）必须排在短名（gpt-4o）之前
	{[]string{"gpt-4o-mini"}, 0.15, 0.6},
	{[]string{"gpt-4o"}, 2.5, 10},
	{[]string{"gpt-4.1-nano"}, 0.1, 0.4},
	{[]string{"gpt-4.1-mini"}, 0.4, 1.6},
	{[]string{"gpt-4.1"}, 2, 8},
	{[]string{"gpt-4-turbo"}, 10, 30},
	{[]string{"gpt-4"}, 30, 60},
	{[]string{"gpt-3.5"}, 0.5, 1.5},
	{[]string{"o1-preview", "o1-2024"}, 15, 60},
	{[]string{"o1-mini"}, 1.1, 4.4},
	{[]string{"o1"}, 15, 60},
	{[]string{"o3-mini", "o4-mini"}, 1.1, 4.4},
	{[]string{"o3"}, 10, 40},
	{[]string{"claude-3-opus", "claude-opus"}, 15, 75},
	{[]string{"claude-3-haiku"}, 0.25, 1.25},
	{[]string{"claude-3-5-sonnet", "claude-sonnet", "claude-3-7"}, 3, 15},
	{[]string{"claude-3-5-haiku", "claude-haiku"}, 0.8, 4},
	{[]string{"deepseek-reasoner", "deepseek-r1"}, 0.55, 2.19},
	{[]string{"deepseek"}, 0.27, 1.1},
	{[]string{"qwen-max"}, 1.6, 6.4},
	{[]string{"qwen-plus"}, 0.4, 1.2},
	{[]string{"qwen-turbo", "qwen-flash"}, 0.05, 0.2},
	{[]string{"glm-4-plus", "glm-4-0520"}, 7, 7},
	{[]string{"glm-4"}, 0.7, 0.7},
}

// 未识别模型的兜底单价（避免金额恒零导致报表失真，取中位保守值）
const (
	fallbackPriceIn  = 1.0
	fallbackPriceOut = 2.0
	// usdCnyRate USD→CNY 固定估算汇率（如需动态汇率/可配置化再演进）
	usdCnyRate = 7.2
)

// EstimateCostUSD 估算单次调用的美元成本（按内置定价表，未识别模型走兜底价）
func EstimateCostUSD(modelName string, inputTokens, outputTokens int) (float64, float64) {
	in, out := fallbackPriceIn, fallbackPriceOut
	name := strings.ToLower(modelName)
	for _, p := range modelPrices {
		matched := false
		for _, a := range p.aliases {
			if strings.Contains(name, a) {
				matched = true
				break
			}
		}
		if matched {
			in, out = p.input, p.output
			break
		}
	}
	cost := float64(inputTokens)/1e6*in + float64(outputTokens)/1e6*out
	return cost, cost * usdCnyRate
}

// Generate 生成内容（使用 high_quality 场景路由）

// geoMaxTokensPerCall 单次 LLM 调用 token 上限（v3 审计 P2-9 成本熔断）
// seed: agent_llm.geo_max_tokens_per_call
func geoMaxTokensPerCall() int {
	return service.GlobalConfigParam().GetInt(context.Background(), "agent_llm", "geo_max_tokens_per_call", 4000)
}

func (a *LLMAdapter) Generate(ctx context.Context, systemPrompt, prompt string, temperature float64, maxTokens int) (*LLMResult, error) {
	if a.dispatcher == nil {
		return nil, fmt.Errorf("LLM dispatcher 未初始化")
	}
	// v3 审计 P2-9：调用方可能传 WithoutCancel 的 ctx（工作流长执行），
	// 必须有单次调用超时兜底，防止不可中止的分钟级挂起
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if maxTokens <= 0 || maxTokens > geoMaxTokensPerCall() {
		maxTokens = geoMaxTokensPerCall()
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
		Content:      resp.Content,
		Provider:     resp.Provider,
		Model:        resp.Model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

// GenerateJSON 生成 JSON 格式内容
func (a *LLMAdapter) GenerateJSON(ctx context.Context, systemPrompt, prompt string, maxTokens int) (*LLMResult, error) {
	if a.dispatcher == nil {
		return nil, fmt.Errorf("LLM dispatcher 未初始化")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if maxTokens <= 0 || maxTokens > geoMaxTokensPerCall() {
		maxTokens = geoMaxTokensPerCall()
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
		Content:      resp.Content,
		Provider:     resp.Provider,
		Model:        resp.Model,
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}
