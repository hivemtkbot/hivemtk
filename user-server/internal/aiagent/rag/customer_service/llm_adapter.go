package ragcustomerservice

import (
	"context"

	"marketing/internal/aiagent/llm"
)

// LLMServiceAdapter 适配 *llm.LLMService → LLMServiceInterface
// 解决 LLMService 的方法签名与本包 LLMServiceInterface 不一致的问题
type LLMServiceAdapter struct {
	svc *llm.LLMService
}

// NewLLMServiceAdapter 构造适配器
func NewLLMServiceAdapter(svc *llm.LLMService) *LLMServiceAdapter {
	return &LLMServiceAdapter{svc: svc}
}

// Generate 适配 Generate 接口
func (a *LLMServiceAdapter) Generate(ctx context.Context, config any, prompt string) (string, error) {
	// 尝试从 config 中提取 LLMConfig（如有）
	if llmCfg, ok := config.(*llm.LLMConfig); ok {
		return a.svc.Generate(ctx, llmCfg, prompt)
	}
	// 兜底：用默认 config
	return a.svc.Generate(ctx, &llm.LLMConfig{}, prompt)
}

// GenerateStructured 适配结构化生成
func (a *LLMServiceAdapter) GenerateStructured(ctx context.Context, config any, prompt string, schema any) (any, error) {
	// 简化实现：复用 Generate
	out, err := a.Generate(ctx, config, prompt)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ValidateConfig 校验配置
func (a *LLMServiceAdapter) ValidateConfig(config any) error {
	return nil
}

// GetDefaultConfig 获取默认配置
func (a *LLMServiceAdapter) GetDefaultConfig() any {
	return &llm.LLMConfig{}
}
