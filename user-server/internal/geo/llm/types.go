package llm

// Message 对话消息
type Message struct {
	Role    string `json:"role"`    // system/user/assistant
	Content string `json:"content"`
}

// LLMRequest LLM 请求
type LLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// LLMResponse LLM 响应
type LLMResponse struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}

// LLMProvider LLM 提供商接口
type LLMProvider interface {
	Name() string
	Chat(req *LLMRequest) (*LLMResponse, error)
}

// LLMConfig LLM 配置
type LLMConfig struct {
	Provider string // deepseek/openai/tongyi/groq/moonshot/doubao/ernie
	APIKey   string
	BaseURL  string
	Model    string
}

// 支持的提供商名称常量
const (
	ProviderDeepSeek = "deepseek"
	ProviderOpenAI   = "openai"
	ProviderTongyi   = "tongyi"
	ProviderGroq     = "groq"
	ProviderMoonshot = "moonshot"
	ProviderDoubao   = "doubao"
	ProviderErnie    = "ernie"
)

// defaultModels 各提供商默认模型
var defaultModels = map[string]string{
	ProviderDeepSeek: "deepseek-chat",
	ProviderOpenAI:   "gpt-4o-mini",
	ProviderTongyi:   "qwen-max",
	ProviderGroq:     "llama3-70b-8192",
	ProviderMoonshot: "moonshot-v1-128k",
	ProviderDoubao:   "",
	ProviderErnie:    "ernie-bot-turbo",
}

// DefaultModel 返回指定提供商的默认模型名
func DefaultModel(provider string) string {
	if m, ok := defaultModels[provider]; ok {
		return m
	}
	return ""
}

// SupportedProviders 返回支持的提供商列表
func SupportedProviders() []string {
	return []string{
		ProviderDeepSeek,
		ProviderOpenAI,
		ProviderTongyi,
		ProviderGroq,
		ProviderMoonshot,
		ProviderDoubao,
		ProviderErnie,
	}
}
