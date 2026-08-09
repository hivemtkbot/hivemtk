package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
)

// LLMConfig LLM配置
type LLMConfig struct {
	APIKey           string
	BaseURL          string
	APIType          string // openai, anthropic, custom, azure
	Model            string
	MaxRetries       int
	RequestTimeout   int
	Temperature      float64
	MaxTokens        int
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
	ResponseFormat   string // json_object, text
	SystemPrompt     string
	// 用于置信度计算（§15.5.4 LLMEntropy 信号）
	// - Logprobs: 请求 LLM 返回每个 token 的 log 概率
	// - TopLogprobs: 返回 top-N 候选 token 的 log 概率（用于计算 TopTokenEntropy）
	// 实现 chatResponse.choices[0].logprobs 解析。
	Logprobs    bool
	TopLogprobs int
	// ===== 智能体 tool_call 支持 =====
	// Tools: OpenAI Function Calling 工具定义；非空时 chatRequest 会携带 tools 字段
	//        并设置 tool_choice=auto（除非 ToolChoice 显式指定）
	// ToolChoice: "auto"/"none"/"required" 或 "{\"type\":\"function\",\"function\":{\"name\":\"xxx\"}}"
	//             留空且 Tools 非空时默认 "auto"
	Tools      []ToolDefinition
	ToolChoice string
	// Messages: 多轮对话历史（含 tool 角色回灌工具结果）
	// 留空时使用 SystemPrompt+Prompt 两段式（向后兼容）
	Messages []ChatMessage
}

// ToolDefinition OpenAI 兼容的工具定义
type ToolDefinition struct {
	Type     string             `json:"type"` // 固定 "function"
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema 工具函数 schema
type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema（直接用 map[string]any 灵活透传）
}

// ChatMessage 通用对话消息（支持 user/assistant/tool/system 角色）
type ChatMessage struct {
	Role       string     `json:"role"`                   // system / user / assistant / tool
	Content    string     `json:"content,omitempty"`      // 文本内容（assistant 角色在返回 tool_calls 时可为空）
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant 角色携带的工具调用
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 时关联的 tool_call ID
	Name       string     `json:"name,omitempty"`         // role=tool 时关联的工具名
}

// ToolCall OpenAI 兼容的工具调用结构
type ToolCall struct {
	ID       string           `json:"id"`   // 调用 ID（用于回传 tool result 给 LLM）
	Type     string           `json:"type"` // 固定 "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用 function 部分
type ToolCallFunction struct {
	Name      string `json:"name"`      // 工具名
	Arguments string `json:"arguments"` // JSON 字符串参数
}

// LLMServiceInterface LLM服务接口
type LLMServiceInterface interface {
	Generate(ctx context.Context, config *LLMConfig, prompt string) (string, error)
	GenerateWithTools(ctx context.Context, config *LLMConfig, prompt string) (*GenerateResult, error)
	GenerateStructured(ctx context.Context, config *LLMConfig, prompt string, responseSchema any) (any, error)
	ValidateConfig(config *LLMConfig) error
	GetDefaultConfig() *LLMConfig
}

// LLMService LLM服务实现
// 支持 OpenAI 兼容协议（OpenAI/Azure OpenAI/DeepSeek/通义千问/Moonshot 等）
type LLMService struct {
	httpClient *http.Client
}

// defaultHTTPTimeout 默认 HTTP 客户端超时
// 可配置，由 NewDispatcherFromConfig 启动时通过 setDefaultHTTPTimeout 注入
// 默认 180s（覆盖大多数 CPU 推理场景）；开发模式可在 config.yaml 设 720s 等大值。
// 与 dispatcher.MaxLatency、sales_engine.agentLoopTotalTimeout 共享同一配置源。
var defaultHTTPTimeout = 180 * time.Second

// setDefaultHTTPTimeout 注入 HTTP 客户端默认超时
// 由 NewDispatcherFromConfig 启动时调用，从 inference.llm.timeout_seconds 派生
func setDefaultHTTPTimeout(d time.Duration) {
	if d <= 0 {
		return
	}
	defaultHTTPTimeout = d
}

// NewLLMService 创建新的LLM服务
func NewLLMService() *LLMService {
	return &LLMService{
		// 单次 HTTP 超时由 defaultHTTPTimeout 控制（默认 180s，可由配置注入）
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// chatRequest OpenAI 兼容的聊天请求体
type chatRequest struct {
	Model            string         `json:"model"`
	Messages         []chatMessage  `json:"messages"`
	Temperature      float64        `json:"temperature,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	TopP             float64        `json:"top_p,omitempty"`
	FrequencyPenalty float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64        `json:"presence_penalty,omitempty"`
	ResponseFormat   map[string]any `json:"response_format,omitempty"`
	Stream           bool           `json:"stream"`
	// 智能体 tool_call 字段
	Tools      []map[string]any `json:"tools,omitempty"`       // OpenAI tools 数组
	ToolChoice any              `json:"tool_choice,omitempty"` // "auto"/"none"/"required" 或 {type:function,function:{name:xxx}}
}

// chatMessage OpenAI 兼容的聊天消息
// 增加 ToolCalls / ToolCallID 字段以支持 tool_call 往返
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`   // assistant 携带的工具调用
	ToolCallID string         `json:"tool_call_id,omitempty"` // role=tool 时关联的 tool_call ID
	Name       string         `json:"name,omitempty"`         // role=tool 时关联的工具名
}

// chatToolCall OpenAI 兼容的 tool_call 结构
type chatToolCall struct {
	ID       string           `json:"id"`   // 调用 ID
	Type     string           `json:"type"` // 固定 "function"
	Function chatToolCallFunc `json:"function"`
}

// chatToolCallFunc 工具调用 function 部分
type chatToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串参数
}

// chatResponse OpenAI 兼容的聊天响应
type chatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role             string         `json:"role"`
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning,omitempty"`      // 推理模型（如 sensenova-6.7-flash-lite）的链式思考
			ReasoningContent2 string        `json:"reasoning_content,omitempty"` // DeepSeek-R1 等用此键
			ToolCalls        []chatToolCall `json:"tool_calls,omitempty"` // 解析 LLM 返回的 tool_calls
		} `json:"message"`
		FinishReason string `json:"finish_reason"` // "stop"/"length"/"tool_calls"/"content_filter"
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// applyEnvDefaults 当调用方未显式提供 APIKey/BaseURL/Model 时，
// 从环境变量注入（LLM_API_KEY / LLM_BASE_URL / LLM_MODEL）。
// 这使 SalesEngine、会话分配、统一消息等只传 Model/APIType 的调用点也能真正调通 LLM。
func applyEnvDefaults(cfg *LLMConfig) {
	if cfg == nil {
		return
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("LLM_API_KEY")
	}
	if cfg.BaseURL == "" {
		if v := os.Getenv("LLM_BASE_URL"); v != "" {
			cfg.BaseURL = v
		}
	}
	if cfg.Model == "" || cfg.Model == "gpt-3.5-turbo" {
		// 用户通过 LLM_MODEL 指定的模型优先于内置默认（gpt-3.5-turbo 在部分账号已弃用）
		if v := os.Getenv("LLM_MODEL"); v != "" {
			cfg.Model = v
		}
	}
}

// GenerateResult LLM 调用结果（智能体支持）
//
// 字段含义：
//   - Content：文本内容（finish_reason=stop/length 时为最终回复；
//     finish_reason=tool_calls 时可能为空或仅是 LLM 的中间说明）
//   - ToolCalls：LLM 决定调用的工具列表（finish_reason=tool_calls 时非空）
//   - FinishReason：stop / length / tool_calls / content_filter
//   - Usage：token 用量统计
type GenerateResult struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        TokenUsage
}

// TokenUsage 类型定义见 dispatcher.go（同包内共享）。
// 本文件不重复声明。

// Generate 生成文本回复（向后兼容）。
//
// 注意：当 config.Tools 非空时，LLM 可能返回 tool_calls 而非文本内容；
// 此时本方法只返回 content（可能为空字符串）。需要获取 tool_calls 的调用方
// 请使用 GenerateWithTools。
func (s *LLMService) Generate(ctx context.Context, config *LLMConfig, prompt string) (string, error) {
	result, err := s.GenerateWithTools(ctx, config, prompt)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// GenerateWithTools 调用 LLM 并返回完整结果（含 tool_calls）。
//
// 智能体 Agent Loop 使用：LLM 返回 tool_calls 时由调用方执行工具、
// 回灌 tool 结果到 config.Messages、再次调用本方法，直到 finish_reason=stop。
//
// 消息构造规则：
//  1. 若 config.Messages 非空（多轮含 tool 结果），优先使用（智能体循环场景）。
//  2. 否则使用 SystemPrompt + prompt 两段式（向后兼容旧调用方）。
//
// 工具构造规则：
//   - config.Tools 非空时，序列化为 OpenAI tools 数组并设置 tool_choice。
//   - tool_choice 默认 "auto"；支持 "auto"/"none"/"required" 或 {"type":"function","function":{"name":"xxx"}}。
// sanitizeToolName 将工具函数名转为 OpenAI/DeepSeek 兼容格式（仅允许 [a-zA-Z0-9_-]）。
// 本地 llama-server 对函数名较宽松，但云端 DeepSeek 严格校验正则 ^[a-zA-Z0-9_-]+$，
// 本项目工具名含点号（如 knowledge.search）会被 400 拒绝。此处统一合规化，并在响应时还原。
// 注意：若两个不同原始名合规化后发生冲突，本函数不保证可逆，但实践中工具名冲突极低。
func sanitizeToolName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (s *LLMService) GenerateWithTools(ctx context.Context, config *LLMConfig, prompt string) (*GenerateResult, error) {
	applyEnvDefaults(config)
	if err := s.ValidateConfig(config); err != nil {
		return nil, err
	}

	// 构造 messages：优先使用 config.Messages（多轮含 tool 结果）
	var messages []chatMessage
	if len(config.Messages) > 0 {
		messages = make([]chatMessage, 0, len(config.Messages))
		for _, m := range config.Messages {
			nm := m.Name
			if nm != "" {
				nm = sanitizeToolName(nm)
			}
			messages = append(messages, chatMessage{
				Role:       m.Role,
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
				Name:       nm,
				ToolCalls:  toChatToolCalls(m.ToolCalls),
			})
		}
	} else {
		messages = []chatMessage{}
		if config.SystemPrompt != "" {
			messages = append(messages, chatMessage{Role: "system", Content: config.SystemPrompt})
		}
		messages = append(messages, chatMessage{Role: "user", Content: prompt})
	}

	reqBody := chatRequest{
		Model:            config.Model,
		Messages:         messages,
		Temperature:      config.Temperature,
		MaxTokens:        config.MaxTokens,
		TopP:             config.TopP,
		FrequencyPenalty: config.FrequencyPenalty,
		PresencePenalty:  config.PresencePenalty,
		Stream:           false,
	}
	if config.ResponseFormat == "json_object" {
		reqBody.ResponseFormat = map[string]any{"type": "json_object"}
	}

	// 序列化 Tools / ToolChoice 到请求体
	// toolNameMap 记录 合规化名->原始名，用于响应时还原（云端 API 仅接受合规名）
	toolNameMap := make(map[string]string)
	if len(config.Tools) > 0 {
		reqBody.Tools = make([]map[string]any, 0, len(config.Tools))
		for _, t := range config.Tools {
			fnType := t.Type
			if fnType == "" {
				fnType = "function"
			}
			safe := sanitizeToolName(t.Function.Name)
			toolNameMap[safe] = t.Function.Name
			logger.Infof("[LLM] tool: name=%s desc_len=%d params=%v",
				t.Function.Name, len(t.Function.Description), t.Function.Parameters != nil)
			reqBody.Tools = append(reqBody.Tools, map[string]any{
				"type": fnType,
				"function": map[string]any{
					"name":        safe,
					"description": t.Function.Description,
					"parameters":  t.Function.Parameters,
				},
			})
		}
		choice := config.ToolChoice
		if choice == "" {
			choice = "auto"
		}
		// 支持字符串 "auto"/"none"/"required" 或 JSON 对象字符串
		if choice == "auto" || choice == "none" || choice == "required" {
			reqBody.ToolChoice = choice
		} else if strings.HasPrefix(choice, "{") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(choice), &obj); err == nil {
				if fn, ok := obj["function"].(map[string]any); ok {
					if nm, ok := fn["name"].(string); ok && nm != "" {
						fn["name"] = sanitizeToolName(nm)
					}
				}
				reqBody.ToolChoice = obj
			} else {
				reqBody.ToolChoice = "auto"
			}
		} else {
			reqBody.ToolChoice = "auto"
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// 调试日志：记录 tools 数量
	logger.Infof("[LLM] request: model=%s tools=%d tool_choice=%v messages=%d body_len=%d config_tools=%d config_toolchoice=%q",
		config.Model, len(reqBody.Tools), reqBody.ToolChoice, len(messages), len(bodyBytes), len(config.Tools), config.ToolChoice)
	// DEBUG: 写入带 tools 的请求
	if len(reqBody.Tools) > 0 {
		os.WriteFile("/tmp/llm_with_tools.json", bodyBytes, 0644)
	}

	resp, err := s.callProvider(ctx, config, bodyBytes)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM: model=%s", config.Model)
	}

	choice := resp.Choices[0]
	// 推理模型（sensenova-6.7-flash-lite 等）把最终答案放在 content，但链式思考放在
	// reasoning/reasoning_content；极少数被 max_tokens 截断时 content 可能为空而 reasoning 有内容。
	// 此处做兜底：content 为空时回退到 reasoning，避免上层（自学习评分 / Agent 回复）拿到空串。
	content := choice.Message.Content
	if content == "" {
		content = choice.Message.ReasoningContent
	}
	if content == "" {
		content = choice.Message.ReasoningContent2
	}
	result := &GenerateResult{
		Content:      content,
		FinishReason: choice.FinishReason,
		Usage: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	// 解析 tool_calls（finish_reason=tool_calls 时 LLM 要求调用工具）
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))
		for _, tc := range choice.Message.ToolCalls {
			// 还原合规化前的原始工具名，保证 Agent 工具分发正确
			name := tc.Function.Name
			if orig, ok := toolNameMap[name]; ok {
				name = orig
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: ToolCallFunction{
					Name:      name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	return result, nil
}

// toChatToolCalls 将公共 ToolCall 转为内部 chatToolCall（结构体序列化）
func toChatToolCalls(tcs []ToolCall) []chatToolCall {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]chatToolCall, 0, len(tcs))
	for _, tc := range tcs {
		out = append(out, chatToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: chatToolCallFunc{
				Name:      sanitizeToolName(tc.Function.Name),
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return out
}

// GenerateStructured 生成结构化回复
func (s *LLMService) GenerateStructured(ctx context.Context, config *LLMConfig, prompt string, responseSchema any) (any, error) {
	// 追加 JSON 模式指令到提示
	schemaJSON, _ := json.Marshal(responseSchema)
	structuredPrompt := fmt.Sprintf("%s\n\n请严格按照以下 JSON Schema 输出响应（仅输出合法 JSON，不要其他说明文字）：\n%s", prompt, string(schemaJSON))

	// 强制使用 JSON 输出
	cfg := *config
	if cfg.ResponseFormat != "json_object" {
		cfg.ResponseFormat = "json_object"
	}

	rawJSON, err := s.Generate(ctx, &cfg, structuredPrompt)
	if err != nil {
		return nil, err
	}

	// 提取 JSON 子串
	rawJSON = extractJSON(rawJSON)
	if rawJSON == "" {
		return nil, fmt.Errorf("no JSON content in LLM response")
	}

	// 反序列化为 responseSchema 类型
	result := responseSchema
	if result == nil {
		var generic map[string]any
		if err := json.Unmarshal([]byte(rawJSON), &generic); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
		return generic, nil
	}
	if err := json.Unmarshal([]byte(rawJSON), result); err != nil {
		return nil, fmt.Errorf("parse JSON to schema: %w", err)
	}
	return result, nil
}

// callProvider 调用 LLM 提供商
func (s *LLMService) callProvider(ctx context.Context, config *LLMConfig, body []byte) (*chatResponse, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		switch config.APIType {
		case "azure":
			baseURL = "https://api.openai.azure.com"
		case "anthropic":
			baseURL = "https://api.anthropic.com"
		default:
			baseURL = "https://api.openai.com"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	// 规范化：去掉可能已存在的 /v1 后缀，避免与环境变量（如 https://api.openai.com/v1）
	// 拼接出 /v1/v1/chat/completions 双斜杠导致 404。
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	// OpenAI 兼容接口（OpenAI/Azure/DeepSeek/通义千问/Moonshot 均使用 /v1/chat/completions）
	endpoint := baseURL + "/v1/chat/completions"
	if config.APIType == "azure" {
		// Azure OpenAI 路径不同
		endpoint = fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-01", baseURL, config.Model)
	} else if config.APIType == "anthropic" {
		// Anthropic Messages API 路径
		endpoint = baseURL + "/v1/messages"
	}

	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		switch config.APIType {
		case "anthropic":
			req.Header.Set("x-api-key", config.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		default:
			req.Header.Set("Authorization", "Bearer "+config.APIKey)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			logger.Errorf("[LLM] request error (attempt %d/%d): %v", attempt+1, maxRetries, err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: status=%d body=%s", resp.StatusCode, string(respBody))
			logger.Errorf("[LLM] server error (attempt %d/%d): %s", attempt+1, maxRetries, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("LLM API error: status=%d body=%s", resp.StatusCode, string(respBody))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return nil, fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
		}

		if chatResp.Error != nil {
			return nil, fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
		}

		return &chatResp, nil
	}

	return nil, fmt.Errorf("LLM request failed after %d attempts: %w", maxRetries, lastErr)
}

// ValidateConfig 验证配置
func (s *LLMService) ValidateConfig(config *LLMConfig) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}
	if config.APIKey == "" {
		// 本地模型（llama.cpp / mtk-llm）无需密钥；云端缺密钥由请求层返回 401。
		// 仅做提示性日志，不阻断（优化三：本地优先，默认无密钥）。
		logger.Warnf("[llm] WARN: APIKey empty for base_url=%s (local model is fine; cloud will 401)", config.BaseURL)
	}
	if config.Model == "" {
		return fmt.Errorf("model is required")
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 60
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if config.MaxTokens <= 0 {
		// 推理模型 reasoning 阶段占用较多 token，过小上限会截断到空回复；基线 2048。
		config.MaxTokens = 2048
	}
	return nil
}

// GetDefaultConfig 获取默认配置（本地优先）
//
// 优先级：config.yaml inference.llm > 环境变量 LLM_* > 内置本地默认（127.0.0.1:8207）。
// 默认即本地 mtk-llm（llama.cpp，OpenAI 兼容）；用户配置线上 base_url+api_key 即切线上。
func (s *LLMService) GetDefaultConfig() *LLMConfig {
	inf := config.GetAppConfig().Inference.LLM
	baseURL := inf.BaseURL
	model := inf.Model
	apiKey := inf.APIKey
	if baseURL == "" {
		baseURL = os.Getenv("LLM_BASE_URL")
	}
	if baseURL == "" {
		// 内置本地默认（host 部署走 127.0.0.1; docker 部署由 config-docker.yaml 显式设置 mtk-llm:8207）
		// 单一源：config.DefaultLLMBaseURLDev（user-server/internal/pkg/utils/config/ports.go）
		// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8207 | LLM（llama.cpp）
		baseURL = config.DefaultLLMBaseURLDev
	}
	if model == "" {
		if v := os.Getenv("LLM_MODEL"); v != "" {
			model = v
		}
	}
	if model == "" {
		// 单一源：config.defaultLLMModel（user-server/internal/pkg/utils/config/server.go）
		// 文档源：DEVELOPMENT.md §2.4 + config.yaml inference.llm.model
		// 行为：dev 档默认为 Qwen2.5-1.5B-Instruct（CPU 推理优化档），与 config.yaml 严格对齐
		model = config.DefaultLLMModel()
	}
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
	}
	return &LLMConfig{
		Model:            model,
		APIType:          "openai",
		BaseURL:          baseURL,
		APIKey:           apiKey,
		MaxRetries:       3,
		RequestTimeout:   60,
		Temperature:      0.7,
		MaxTokens:        2048,
		TopP:             0.9,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.5,
		ResponseFormat:   "json_object",
	}
}

// extractJSON 从 LLM 响应中提取 JSON 子串
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")

	var start int
	if startObj == -1 && startArr == -1 {
		return ""
	}
	if startObj == -1 {
		start = startArr
	} else if startArr == -1 {
		start = startObj
	} else if startObj < startArr {
		start = startObj
	} else {
		start = startArr
	}

	// 找匹配的结束位置
	openCount := 0
	openCh := byte(s[start])
	var closeCh byte
	if openCh == '{' {
		closeCh = '}'
	} else {
		closeCh = ']'
	}

	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
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
		if c == openCh {
			openCount++
		} else if c == closeCh {
			openCount--
			if openCount == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}
