package ragretrieval


import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MultiQueryGenerator Multi-Query 变体生成器
type MultiQueryGenerator struct {
	chatClient LLMChatClient
	enabled    bool
	variantN   int 
}

// MultiQueryGeneratorConfig Multi-Query 配置
type MultiQueryGeneratorConfig struct {
	Enabled  bool
	VariantN int
}

// DefaultMultiQueryGeneratorConfig 默认配置
func DefaultMultiQueryGeneratorConfig() *MultiQueryGeneratorConfig {
	return &MultiQueryGeneratorConfig{
		Enabled:  true,
		VariantN: 3,
	}
}

// NewMultiQueryGenerator 创建 Multi-Query 生成器
//
// chatClient 为 nil 时自动 disabled
func NewMultiQueryGenerator(chatClient LLMChatClient, cfg *MultiQueryGeneratorConfig) *MultiQueryGenerator {
	if cfg == nil {
		cfg = DefaultMultiQueryGeneratorConfig()
	}
	enabled := cfg.Enabled
	if chatClient == nil {
		enabled = false
	}
	n := cfg.VariantN
	if n <= 0 {
		n = 3
	}
	return &MultiQueryGenerator{
		chatClient: chatClient,
		enabled:    enabled,
		variantN:   n,
	}
}

// IsEnabled 是否启用
func (g *MultiQueryGenerator) IsEnabled() bool {
	return g != nil && g.enabled && g.chatClient != nil
}

// Generate 生成 Multi-Query 变体
//
// 输出: []string 变体列表（已去空白、去重）
// 失败场景:
//   - 未启用 → 返回 (nil, nil)
//   - LLM 调用失败 → 返回 error
//   - JSON 解析失败 → 返回 error
//   - 空数组 → 返回 error
func (g *MultiQueryGenerator) Generate(ctx context.Context, query string) ([]string, error) {
	if !g.IsEnabled() {
		return nil, nil
	}
	prompt := fmt.Sprintf(`你是查询改写助手。请基于以下用户原始查询，生成 %d 个不同视角的查询变体。

要求：
1. 每个变体从不同角度表达相同的检索意图（同义词替换、句式变换、视角切换）
2. 每个变体应能独立用于检索（不要相互引用）
3. 输出 JSON 数组格式，如 ["变体1", "变体2", "变体3"]
4. 不要输出任何其他内容

用户原始查询: %s

%d 个查询变体:`, g.variantN, query, g.variantN)

	resp, err := g.chatClient.Chat(ctx, prompt, LLMChatOptions{
		Temperature: 0.5, 
		MaxTokens:   200,
	})
	if err != nil {
		return nil, fmt.Errorf("Multi-Query LLM 调用失败: %w", err)
	}

	jsonStr := extractJSONArray(resp)
	if jsonStr == "" {
		return nil, fmt.Errorf("Multi-Query 响应无 JSON 数组: %s", resp)
	}

	var variants []string
	if err := json.Unmarshal([]byte(jsonStr), &variants); err != nil {
		return nil, fmt.Errorf("Multi-Query 解析失败: %w (raw=%s)", err, jsonStr)
	}

	seen := make(map[string]bool, len(variants))
	out := make([]string, 0, len(variants))
	for _, v := range variants {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Multi-Query 返回空（去重后）")
	}
	return out, nil
}

// extractJSONArray 从 LLM 输出中提取 JSON 数组（兼容 markdown 代码块包裹）
//
// 与 llm_service.go 中的 extractJSON 不同，本函数仅识别数组（[ 开头）
func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	s = strings.TrimSpace(s)

	start := strings.Index(s, "[")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(s, "]")
	if end == -1 || end <= start {
		return ""
	}
	return s[start : end+1]
}

