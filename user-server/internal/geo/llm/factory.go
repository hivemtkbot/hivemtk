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
)

// LLMFactory LLM 工厂，管理多个 LLM 提供商
type LLMFactory struct {
	configs  map[string]LLMConfig
	provider map[string]LLMProvider
	client   *http.Client
}

// NewLLMFactory 创建 LLM 工厂
func NewLLMFactory(configs map[string]LLMConfig) *LLMFactory {
	f := &LLMFactory{
		configs:  configs,
		provider: make(map[string]LLMProvider),
		client: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
	f.registerProviders()
	return f
}

// NewLLMFactoryFromEnv 从环境变量创建 LLM 工厂
// 每个提供商通过以下环境变量配置（GEO_ 前缀）：
//   GEO_<PROVIDER>_API_KEY  - API Key（必填，未设置则跳过该提供商）
//   GEO_<PROVIDER>_MODEL    - 模型名称（可选，未设置使用默认模型）
//   GEO_<PROVIDER>_BASE_URL - 自定义 Base URL（可选）
// <PROVIDER> 为大写提供商名：DEEPSEEK / OPENAI / TONGYI / GROQ / MOONSHOT / DOUBAO / ERNIE
func NewLLMFactoryFromEnv() *LLMFactory {
	configs := make(map[string]LLMConfig)
	for _, provider := range SupportedProviders() {
		envPrefix := "GEO_" + strings.ToUpper(provider)
		apiKey := os.Getenv(envPrefix + "_API_KEY")
		if apiKey == "" {
			continue
		}
		cfg := LLMConfig{
			Provider: provider,
			APIKey:   apiKey,
		}
		if model := os.Getenv(envPrefix + "_MODEL"); model != "" {
			cfg.Model = model
		}
		if baseURL := os.Getenv(envPrefix + "_BASE_URL"); baseURL != "" {
			cfg.BaseURL = baseURL
		}
		configs[provider] = cfg
	}
	return NewLLMFactory(configs)
}

// registerProviders 注册所有已配置的提供商
func (f *LLMFactory) registerProviders() {
	for name, cfg := range f.configs {
		p := f.createProvider(name, cfg)
		if p != nil {
			f.provider[name] = p
		}
	}
}

// createProvider 根据提供商名称创建对应的 provider 实例
func (f *LLMFactory) createProvider(name string, cfg LLMConfig) LLMProvider {
	base := openaiCompatibleProvider{
		apiKey:   cfg.APIKey,
		model:    cfg.Model,
		httpCli:  f.client,
		provider: name,
	}
	switch name {
	case ProviderDeepSeek:
		base.baseURL = "https://api.deepseek.com/v1/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderDeepSeek)
		}
		return &deepseekProvider{openaiCompatibleProvider: base}
	case ProviderOpenAI:
		base.baseURL = "https://api.openai.com/v1/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderOpenAI)
		}
		return &openaiProvider{openaiCompatibleProvider: base}
	case ProviderTongyi:
		base.baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderTongyi)
		}
		return &tongyiProvider{openaiCompatibleProvider: base}
	case ProviderGroq:
		base.baseURL = "https://api.groq.com/openai/v1/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderGroq)
		}
		return &groqProvider{openaiCompatibleProvider: base}
	case ProviderMoonshot:
		base.baseURL = "https://api.moonshot.cn/v1/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderMoonshot)
		}
		return &moonshotProvider{openaiCompatibleProvider: base}
	case ProviderDoubao:
		base.baseURL = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
		if cfg.BaseURL != "" {
			base.baseURL = cfg.BaseURL
		}
		if base.model == "" {
			base.model = DefaultModel(ProviderDoubao)
		}
		return &doubaoProvider{openaiCompatibleProvider: base}
	case ProviderErnie:
		e := &ernieProvider{
			apiKey:  cfg.APIKey,
			model:   cfg.Model,
			httpCli: f.client,
		}
		if e.model == "" {
			e.model = DefaultModel(ProviderErnie)
		}
		e.baseURL = cfg.BaseURL
		return e
	}
	return nil
}

// GetProvider 根据提供商名称获取 provider
func (f *LLMFactory) GetProvider(providerName string) (LLMProvider, error) {
	p, ok := f.provider[providerName]
	if !ok {
		return nil, fmt.Errorf("不支持的 LLM 提供商: %s", providerName)
	}
	return p, nil
}

// GetDefaultProvider 获取默认 provider（按优先级返回第一个已配置的）
func (f *LLMFactory) GetDefaultProvider() LLMProvider {
	for _, name := range SupportedProviders() {
		if p, ok := f.provider[name]; ok {
			return p
		}
	}
	return nil
}

// GetProviderNames 返回已注册的提供商名称列表
func (f *LLMFactory) GetProviderNames() []string {
	names := make([]string, 0, len(f.provider))
	for name := range f.provider {
		names = append(names, name)
	}
	return names
}

// --- OpenAI 兼容协议基础实现 ---

// openaiCompatibleProvider OpenAI 兼容协议基础结构
// DeepSeek/OpenAI/Tongyi/Groq/Moonshot/Doubao 均使用此基础结构
type openaiCompatibleProvider struct {
	apiKey   string
	baseURL  string
	model    string
	httpCli  *http.Client
	provider string
}

// chatRequest OpenAI 兼容的聊天请求体
type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// chatResponse OpenAI 兼容的聊天响应
type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
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

// chat 调用 OpenAI 兼容协议的 chat completions 接口
func (p *openaiCompatibleProvider) chat(req *LLMRequest) (*LLMResponse, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("%s: APIKey 为空", p.provider)
	}
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, fmt.Errorf("%s: model 为空", p.provider)
	}

	body := chatRequest{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if body.Temperature == 0 {
		body.Temperature = 0.7
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: 序列化请求失败: %w", p.provider, err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", p.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: 创建请求失败: %w", p.provider, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpCli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: HTTP 请求失败: %w", p.provider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: 读取响应失败: %w", p.provider, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: API 错误 status=%d body=%s", p.provider, resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("%s: 解析响应失败: %w (body=%s)", p.provider, err, string(respBody))
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("%s: API 错误: %s", p.provider, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("%s: 响应为空", p.provider)
	}

	return &LLMResponse{
		Content:      chatResp.Choices[0].Message.Content,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Model:        chatResp.Model,
		Provider:     p.provider,
	}, nil
}

// --- DeepSeek ---

type deepseekProvider struct {
	openaiCompatibleProvider
}

func (p *deepseekProvider) Name() string { return ProviderDeepSeek }

func (p *deepseekProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	return p.chat(req)
}

// --- OpenAI ---

type openaiProvider struct {
	openaiCompatibleProvider
}

func (p *openaiProvider) Name() string { return ProviderOpenAI }

func (p *openaiProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	return p.chat(req)
}

// --- Tongyi (通义千问) ---

type tongyiProvider struct {
	openaiCompatibleProvider
}

func (p *tongyiProvider) Name() string { return ProviderTongyi }

func (p *tongyiProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	return p.chat(req)
}

// --- Groq ---

type groqProvider struct {
	openaiCompatibleProvider
}

func (p *groqProvider) Name() string { return ProviderGroq }

func (p *groqProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	return p.chat(req)
}

// --- Moonshot (Kimi) ---

type moonshotProvider struct {
	openaiCompatibleProvider
}

func (p *moonshotProvider) Name() string { return ProviderMoonshot }

func (p *moonshotProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	return p.chat(req)
}

// --- Doubao (豆包) ---

type doubaoProvider struct {
	openaiCompatibleProvider
}

func (p *doubaoProvider) Name() string { return ProviderDoubao }

func (p *doubaoProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	// 豆包 API Key 格式为 access_key:secret_key:endpoint_id
	parts := strings.Split(p.openaiCompatibleProvider.apiKey, ":")
	if len(parts) >= 3 && p.openaiCompatibleProvider.model == "" {
		// endpoint_id 作为 model
		p.openaiCompatibleProvider.model = parts[2]
	}
	return p.chat(req)
}

// --- Ernie (文心一言/百度千帆) ---

// ernieProvider 百度文心一言 provider（使用 OAuth token，非 OpenAI 兼容）
type ernieProvider struct {
	apiKey  string // 格式: app_key:app_secret
	model   string
	baseURL string
	httpCli *http.Client
	// 缓存的 access token
	accessToken string
	tokenExpiry time.Time
}

func (p *ernieProvider) Name() string { return ProviderErnie }

// ernieTokenResponse 百度 OAuth token 响应
type ernieTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Error       string `json:"error,omitempty"`
}

// getAccessToken 获取百度 OAuth access token
func (p *ernieProvider) getAccessToken() (string, error) {
	// 使用缓存的 token
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	parts := strings.Split(p.apiKey, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("ernie: APIKey 格式错误，应为 app_key:app_secret")
	}
	appKey, appSecret := parts[0], parts[1]

	tokenURL := "https://aip.baidubce.com/oauth/2.0/token"
	reqURL := fmt.Sprintf("%s?grant_type=client_credentials&client_id=%s&client_secret=%s",
		tokenURL, appKey, appSecret)

	req, err := http.NewRequestWithContext(context.Background(), "POST", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("ernie: 创建 token 请求失败: %w", err)
	}

	resp, err := p.httpCli.Do(req)
	if err != nil {
		return "", fmt.Errorf("ernie: 获取 token 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ernie: 读取 token 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ernie: token 请求错误 status=%d body=%s", resp.StatusCode, string(body))
	}

	var tokenResp ernieTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("ernie: 解析 token 响应失败: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("ernie: token 错误: %s", tokenResp.Error)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("ernie: 返回的 access_token 为空")
	}

	p.accessToken = tokenResp.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return p.accessToken, nil
}

// ernieChatRequest 百度千帆聊天请求体
type ernieChatRequest struct {
	Model       string    `json:"model,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_output_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// ernieChatResponse 百度千帆聊天响应
type ernieChatResponse struct {
	ID               string `json:"id"`
	Model            string `json:"model"`
	Result           string `json:"result"`
	Usage            struct {
		PromptTokens    int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens     int `json:"total_tokens"`
	} `json:"usage"`
	ErrorCode int    `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`
}

func (p *ernieProvider) Chat(req *LLMRequest) (*LLMResponse, error) {
	token, err := p.getAccessToken()
	if err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = DefaultModel(ProviderErnie)
	}

	// 百度千帆 chat completions 接口（OpenAI 兼容模式）
	endpoint := p.baseURL
	if endpoint == "" {
		endpoint = "https://qianfan.baidubce.com/wenxinworkshop/chat/completions"
	}
	endpoint = endpoint + "?access_token=" + token

	body := ernieChatRequest{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if body.Temperature == 0 {
		body.Temperature = 0.7
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ernie: 序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ernie: 创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpCli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ernie: HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ernie: 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ernie: API 错误 status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp ernieChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("ernie: 解析响应失败: %w (body=%s)", err, string(respBody))
	}

	if chatResp.ErrorCode != 0 {
		return nil, fmt.Errorf("ernie: API 错误 code=%d msg=%s", chatResp.ErrorCode, chatResp.ErrorMsg)
	}

	if chatResp.Result == "" {
		return nil, fmt.Errorf("ernie: 响应为空")
	}

	return &LLMResponse{
		Content:      chatResp.Result,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Model:        chatResp.Model,
		Provider:     ProviderErnie,
	}, nil
}
