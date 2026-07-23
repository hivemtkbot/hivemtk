package agent_runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// 本地 LLM 客户端（Ollama / vLLM /v1/chat/completions 兼容协议）
//
// 文档依据：docs/企业级架构优化/资产包模式.md §三 运行态 Request Payload
//
// 设计：
//  - 协议完全兼容 OpenAI /v1/chat/completions
//  - Ollama 默认端口 11434，vLLM 默认端口 8000
//  - 私域部署不强制鉴权，APIKey 可空
//  - 单次请求超时 180s（与 LLMService 对齐，支持 Mac M1 跑 1.5B Q4 GGUF）
//  - 不实现流式（agent_runtime 当前是同步推理模式）
// ============================================================================

// LocalLLMClient 本地 LLM 客户端（OpenAI /v1/chat/completions 兼容）
//
// 实现 portcontract.LLMChatPort 接口
type LocalLLMClient struct {
	baseURL string   // 例如 http://localhost:11434/v1
	apiKey  string   // 可空（私域部署不鉴权）
	model   string   // 默认模型，例如 qwen2.5:7b
	httpCli *http.Client
}

// LocalLLMConfig 本地 LLM 客户端配置
type LocalLLMConfig struct {
	BaseURL        string // 不含 /v1 后缀；客户端自动追加。例如 http://localhost:11434
	APIKey         string // 可空
	DefaultModel   string // 默认模型名
	RequestTimeout time.Duration
}

// NewLocalLLMClient 创建本地 LLM 客户端
//
// baseURL 处理逻辑：
//  - 末尾 / 自动 trim
//  - 末尾 /v1 自动 trim（避免 /v1/v1/chat/completions 双斜杠）
//  - 客户端在 Chat 时自动追加 /v1/chat/completions
func NewLocalLLMClient(cfg LocalLLMConfig) *LocalLLMClient {
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	return &LocalLLMClient{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.DefaultModel,
		httpCli: &http.Client{Timeout: timeout},
	}
}

// Chat 发送一次 chat completions 请求
//
// 实现 portcontract.LLMChatPort 接口
//
// 参数：
//   - messages: OpenAI 兼容 messages 数组（来自 Weave 织布结果）
//   - model: 模型名；空字符串时使用客户端默认模型
//   - temperature: 温度（0.0-1.0）；<0 时使用 0.7
func (c *LocalLLMClient) Chat(ctx context.Context, messages []model.AssetBundleMessage, model string, temperature float64) (string, error) {
	if c == nil {
		return "", errors.New("local llm client is nil")
	}
	if len(messages) == 0 {
		return "", errors.New("messages is empty")
	}
	if model == "" {
		model = c.model
	}
	if temperature < 0 {
		temperature = 0.7
	}

	// 转换 messages 为 OpenAI 协议格式
	openaiMsgs := make([]openaiChatMessage, 0, len(messages))
	for _, m := range messages {
		openaiMsgs = append(openaiMsgs, openaiChatMessage{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		})
	}

	reqBody := openaiChatRequest{
		Model:       model,
		Messages:    openaiMsgs,
		Temperature: temperature,
		Stream:      false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm api status=%d body=%s", resp.StatusCode, string(respBytes))
	}

	var chatResp openaiChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", errors.New("llm response has no choices")
	}
	content := chatResp.Choices[0].Message.Content
	logger.Debugf("[local_llm] chat ok model=%s msgs=%d resp_len=%d duration=%s",
		model, len(messages), len(content), time.Since(start))
	return content, nil
}

// ============================================================================
// OpenAI /v1/chat/completions 协议结构
// ============================================================================

type openaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type openaiChatRequest struct {
	Model       string             `json:"model"`
	Messages    []openaiChatMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
	Stream      bool               `json:"stream"`
}

type openaiChatResponse struct {
	ID      string           `json:"id"`
	Choices []openaiChoice   `json:"choices"`
	Usage   openaiUsage      `json:"usage"`
}

type openaiChoice struct {
	Index        int               `json:"index"`
	Message      openaiChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
