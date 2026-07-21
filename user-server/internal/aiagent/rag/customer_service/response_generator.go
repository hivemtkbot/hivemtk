package ragcustomerservice

import (
	"context"
	"encoding/json"
	"fmt"
	"marketing/internal/aiagent/llm"
	"strings"
)

// ResponseGeneratorImpl 回复生成器实现
type ResponseGeneratorImpl struct {
	llmService LLMServiceInterface
	config     *ResponseGenerationConfig
}

// LLMServiceInterface LLM服务接口
type LLMServiceInterface interface {
	Generate(ctx context.Context, config any, prompt string) (string, error)
	GenerateStructured(ctx context.Context, config any, prompt string, schema any) (any, error)
	ValidateConfig(config any) error
	GetDefaultConfig() any
}

// ResponseGenerationConfig 回复生成配置
type ResponseGenerationConfig struct {
	LLMModel           string  `json:"llm_model"`           // LLM模型
	DefaultTemperature float64 `json:"default_temperature"` // 默认温度
	DefaultMaxTokens   int     `json:"default_max_tokens"`  // 默认最大token数
	TopP               float64 `json:"top_p"`               // Top-P采样参数
	FrequencyPenalty   float64 `json:"frequency_penalty"`   // 频率惩罚
	PresencePenalty    float64 `json:"presence_penalty"`    // 存在惩罚
	SystemPrompt       string  `json:"system_prompt"`       // 系统提示词模板
}

// NewResponseGeneratorImpl 创建新的回复生成器
func NewResponseGeneratorImpl(llmService LLMServiceInterface, config *ResponseGenerationConfig) *ResponseGeneratorImpl {
	if config == nil {
		config = &ResponseGenerationConfig{
			LLMModel:           "gpt-3.5-turbo",
			DefaultTemperature: 0.7,
			DefaultMaxTokens:   1000,
			TopP:               0.9,
			FrequencyPenalty:   0.5,
			PresencePenalty:    0.5,
			SystemPrompt:       defaultSystemPrompt,
		}
	}

	return &ResponseGeneratorImpl{
		llmService: llmService,
		config:     config,
	}
}

// GenerateResponse 生成回复
func (g *ResponseGeneratorImpl) GenerateResponse(ctx context.Context, request ResponseGenerationRequest) (string, error) {
	// 构建上下文字符串
	contextStr := g.buildContextString(request.SearchResults, request.Context)

	// 构建提示词
	prompt := g.buildResponsePrompt(request.Query, contextStr, request.Session, request.Context)

	// 准备LLM配置
	llmConfig := prepareLLMConfig(g.config, request.Config)

	// 验证LLM配置
	err := g.llmService.ValidateConfig(llmConfig)
	if err != nil {
		return "", fmt.Errorf("invalid LLM config: %w", err)
	}

	// 调用LLM生成回复
	response, err := g.llmService.Generate(ctx, llmConfig, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	return response, nil
}

// GenerateStructuredResponse 生成结构化回复
func (g *ResponseGeneratorImpl) GenerateStructuredResponse(ctx context.Context, request ResponseGenerationRequest, schema any) (any, error) {
	// 构建上下文字符串
	contextStr := g.buildContextString(request.SearchResults, request.Context)

	// 构建提示词
	prompt := g.buildStructuredResponsePrompt(request.Query, contextStr, request.Session, request.Context, schema)

	// 准备LLM配置
	llmConfig := prepareLLMConfig(g.config, request.Config)

	// 设置响应格式为JSON
	if llmConfigMap, ok := llmConfig.(map[string]any); ok {
		llmConfigMap["response_format"] = "json_object"
	}

	// 验证LLM配置
	err := g.llmService.ValidateConfig(llmConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM config: %w", err)
	}

	// 调用LLM生成结构化回复
	response, err := g.llmService.GenerateStructured(ctx, llmConfig, prompt, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to generate structured response: %w", err)
	}

	return response, nil
}

// BuildContextString 构建上下文字符串
func (g *ResponseGeneratorImpl) BuildContextString(results []any, context Context) string {
	return g.buildContextString(results, context)
}

// BuildResponsePrompt 构建回复提示词
func (g *ResponseGeneratorImpl) BuildResponsePrompt(query, contextStr string, session Session, context Context) string {
	return g.buildResponsePrompt(query, contextStr, session, context)
}

// buildContextString 构建上下文字符串
func (g *ResponseGeneratorImpl) buildContextString(results []any, context Context) string {
	var contextStr string

	// 添加RAG检索结果
	if len(results) > 0 {
		contextStr += "参考知识库信息:\n"
		for i, result := range results {
			if resultStr, ok := result.(string); ok {
				contextStr += fmt.Sprintf("[%d] 参考信息: %s\n\n", i+1, resultStr)
			} else if resultMap, ok := result.(map[string]any); ok {
				content, _ := resultMap["content"].(string)
				docID, _ := resultMap["document_id"].(string)
				score, _ := resultMap["score"].(float64)
				contextStr += fmt.Sprintf("[%d] 来源: %s (相关性: %.2f)\n%s\n\n",
					i+1, docID, score, content)
			}
		}
	} else {
		contextStr += "未找到相关知识库信息。\n"
	}

	// 添加对话上下文
	if context.Topic != "" {
		contextStr += fmt.Sprintf("当前话题: %s\n", context.Topic)
	}

	if context.Intent != "" {
		contextStr += fmt.Sprintf("用户意图: %s\n", context.Intent)
	}

	if len(context.Entities) > 0 {
		contextStr += "识别的实体:\n"
		for entity, values := range context.Entities {
			contextStr += fmt.Sprintf("- %s: %s\n", entity, strings.Join(values, ", "))
		}
	}

	if context.Sentiment.Score != 0 {
		contextStr += fmt.Sprintf("用户情感: %s (%.2f)\n", context.Sentiment.Label, context.Sentiment.Score)
	}

	return contextStr
}

// buildResponsePrompt 构建回复提示词
func (g *ResponseGeneratorImpl) buildResponsePrompt(query, contextStr string, session Session, context Context) string {
	systemPrompt := g.config.SystemPrompt
	if session.Config.SystemPrompt != "" {
		systemPrompt = session.Config.SystemPrompt
	}

	prompt := fmt.Sprintf(`%s

当前对话上下文:
%s

用户问题: %s

注意事项:
1. 回复要专业、友好、简洁
2. 如果知识库信息不足以回答，请诚实告知用户
3. 保持与当前话题的相关性
4. 根据用户情感调整回复语气
5. 如有具体商品或服务询问，基于知识库信息给出详细回复
6. 如遇售后或投诉类问题，表达积极解决问题的态度

客服回复:`, systemPrompt, contextStr, query)

	return prompt
}

// buildStructuredResponsePrompt 构建结构化回复提示词
func (g *ResponseGeneratorImpl) buildStructuredResponsePrompt(query, contextStr string, session Session, context Context, schema any) string {
	schemaJSON, _ := json.Marshal(schema)

	systemPrompt := g.config.SystemPrompt
	if session.Config.SystemPrompt != "" {
		systemPrompt = session.Config.SystemPrompt
	}

	prompt := fmt.Sprintf(`%s

当前对话上下文:
%s

用户问题: %s

请严格按照以下JSON Schema返回结果:
%s

注意事项:
1. 回复要专业、友好、简洁
2. 如果知识库信息不足以回答，请在回复中说明
3. 保持与当前话题的相关性
4. 根据用户情感调整回复语气
5. 遵循JSON Schema格式要求

回复:`, systemPrompt, contextStr, query, string(schemaJSON))

	return prompt
}

// prepareLLMConfig 准备LLM配置
func prepareLLMConfig(config *ResponseGenerationConfig, sessionConfig SessionConfig) any {
	llmConfig := make(map[string]any)

	// 使用会话配置中的参数，如果未设置则使用默认值
	if sessionConfig.Temperature != 0 {
		llmConfig["temperature"] = sessionConfig.Temperature
	} else {
		llmConfig["temperature"] = config.DefaultTemperature
	}

	if sessionConfig.MaxTokens != 0 {
		llmConfig["max_tokens"] = sessionConfig.MaxTokens
	} else {
		llmConfig["max_tokens"] = config.DefaultMaxTokens
	}

	llmConfig["model"] = config.LLMModel
	llmConfig["top_p"] = config.TopP
	llmConfig["frequency_penalty"] = config.FrequencyPenalty
	llmConfig["presence_penalty"] = config.PresencePenalty

	return llmConfig
}

// defaultSystemPrompt 默认系统提示词
const defaultSystemPrompt = `你是专业的电商客服助手，正在为客户提供咨询服务。请基于提供的信息回答用户问题，遵循以下原则：
1. 专业礼貌：使用专业、友好的语气
2. 准确性：基于提供的信息进行回复
3. 清晰简洁：提供清晰、简洁的答案
4. 积极态度：展现积极解决问题的态度
5. 个性化：根据用户的具体情况进行个性化回复`

// RemoteLLMService 真实 LLM 服务适配器(对接 OpenAI 兼容 /v1/chat/completions)
// Session 级别配置会覆盖全局默认。
type RemoteLLMService struct {
	svc *llm.LLMService
}

// NewRemoteLLMService 创建真实 LLM 服务
func NewRemoteLLMService() *RemoteLLMService {
	return &RemoteLLMService{svc: llm.NewLLMService()}
}

// Generate 真实调用 LLM
func (r *RemoteLLMService) Generate(ctx context.Context, config any, prompt string) (string, error) {
	llmCfg, err := toLLMConfig(r.svc, config)
	if err != nil {
		return "", err
	}
	return r.svc.Generate(ctx, llmCfg, prompt)
}

// GenerateStructured 真实结构化调用
func (r *RemoteLLMService) GenerateStructured(ctx context.Context, config any, prompt string, schema any) (any, error) {
	llmCfg, err := toLLMConfig(r.svc, config)
	if err != nil {
		return nil, err
	}
	llmCfg.ResponseFormat = "json_object"
	return r.svc.GenerateStructured(ctx, llmCfg, prompt, schema)
}

// ValidateConfig 校验配置
func (r *RemoteLLMService) ValidateConfig(config any) error {
	llmCfg, err := toLLMConfig(r.svc, config)
	if err != nil {
		return err
	}
	return r.svc.ValidateConfig(llmCfg)
}

// GetDefaultConfig 返回默认 LLMConfig
func (r *RemoteLLMService) GetDefaultConfig() any {
	return r.svc.GetDefaultConfig()
}

// toLLMConfig 把任意 map / 结构体 / *llm.LLMConfig 归一为 *llm.LLMConfig
func toLLMConfig(svc *llm.LLMService, raw any) (*llm.LLMConfig, error) {
	if raw == nil {
		return svc.GetDefaultConfig(), nil
	}
	switch v := raw.(type) {
	case *llm.LLMConfig:
		if v.APIKey == "" {
			v.APIKey = svc.GetDefaultConfig().APIKey
		}
		if v.BaseURL == "" {
			v.BaseURL = svc.GetDefaultConfig().BaseURL
		}
		return v, nil
	case llm.LLMConfig:
		return toLLMConfig(svc, &v)
	case map[string]any:
		cfg := svc.GetDefaultConfig()
		if s, ok := v["model"].(string); ok && s != "" {
			cfg.Model = s
		}
		if s, ok := v["base_url"].(string); ok && s != "" {
			cfg.BaseURL = s
		}
		if s, ok := v["api_key"].(string); ok && s != "" {
			cfg.APIKey = s
		}
		if s, ok := v["api_type"].(string); ok && s != "" {
			cfg.APIType = s
		}
		if f, ok := v["temperature"].(float64); ok {
			cfg.Temperature = f
		}
		if i, ok := v["max_tokens"].(int); ok && i > 0 {
			cfg.MaxTokens = i
		} else if f, ok := v["max_tokens"].(float64); ok && f > 0 {
			cfg.MaxTokens = int(f)
		}
		if s, ok := v["response_format"].(string); ok && s != "" {
			cfg.ResponseFormat = s
		}
		return cfg, nil
	case SessionConfig:
		return toLLMConfig(svc, map[string]any{
			"temperature": v.Temperature,
			"max_tokens":  v.MaxTokens,
		})
	default:
		// 兜底:返回默认配置
		return svc.GetDefaultConfig(), nil
	}
}
