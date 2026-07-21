package ragretrieval

// hyde_generator.go HyDE（Hypothetical Document Embeddings）假设文档生成器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.7
// 论文: "Precise Zero-Shot Dense Retrieval without Relevance Labels" (Gao et al., 2022)
//
// 思想: LLM 生成假设答案文档 → embed 假设文档 → 用假设文档向量检索
//       （因为假设文档与真实文档都是"文档风格"，比 question-document 匹配更准）
// TREC RAG 2025 UTokyo-HitU 冠军方案采用 HyDE + 150 词假设文档
//
// 设计原则:
//   - LLM 调用失败时返回错误，由 QueryRewriter 决定是否降级（不在此处静默吞错）
//   - 假设文档过短（<20 字符）视为 LLM 输出无效，返回错误
//   - 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"strings"
)

// HyDEGenerator HyDE 假设文档生成器
type HyDEGenerator struct {
	chatClient   LLMChatClient
	enabled      bool
	maxDocTokens int // 默认 150 词
	minDocLength int // 默认 20 字符（过短视为无效）
}

// HyDEGeneratorConfig HyDE 配置
type HyDEGeneratorConfig struct {
	Enabled      bool
	MaxDocTokens int
	MinDocLength int
}

// DefaultHyDEGeneratorConfig 默认配置
func DefaultHyDEGeneratorConfig() *HyDEGeneratorConfig {
	return &HyDEGeneratorConfig{
		Enabled:      true,
		MaxDocTokens: 150,
		MinDocLength: 20,
	}
}

// NewHyDEGenerator 创建 HyDE 生成器
//
// chatClient 为 nil 时自动 disabled（保持构造不报错，调用方按需启用）
func NewHyDEGenerator(chatClient LLMChatClient, cfg *HyDEGeneratorConfig) *HyDEGenerator {
	if cfg == nil {
		cfg = DefaultHyDEGeneratorConfig()
	}
	enabled := cfg.Enabled
	if chatClient == nil {
		enabled = false
	}
	return &HyDEGenerator{
		chatClient:   chatClient,
		enabled:      enabled,
		maxDocTokens: cfg.MaxDocTokens,
		minDocLength: cfg.MinDocLength,
	}
}

// IsEnabled 是否启用
func (g *HyDEGenerator) IsEnabled() bool {
	return g != nil && g.enabled && g.chatClient != nil
}

// Generate 生成 HyDE 假设文档
//
// Prompt 关键:
//   - 强调"用文档风格写一段 ~150 词的答案"
//   - 强调"答案可能不准确，但词汇/句式要与真实文档相似"
//   - 严禁提及"我是 AI"等元话语
//
// 失败场景:
//   - 未启用 → 返回 ("", nil)（QueryRewriter 据此降级）
//   - LLM 调用失败 → 返回 error
//   - 输出过短 → 返回 error
func (g *HyDEGenerator) Generate(ctx context.Context, query string) (string, error) {
	if !g.IsEnabled() {
		return "", nil
	}
	prompt := fmt.Sprintf(`请基于以下用户问题，写一段约 %d 词的假设性答案文档。
要求：
1. 用正式的文档风格写作（陈述句、领域术语、事实描述），不要用对话口吻
2. 假设性答案的内容不需要事实准确，但词汇、句式、领域术语应与真实知识库文档相似
3. 不要使用"我是 AI"、"根据资料"等元话语
4. 输出纯文本，不要 markdown 格式

用户问题: %s

假设性答案文档:`, g.maxDocTokens, query)

	resp, err := g.chatClient.Chat(ctx, prompt, LLMChatOptions{
		Temperature: 0.3, // 低温度保证稳定
		MaxTokens:   g.maxDocTokens * 2,
	})
	if err != nil {
		return "", fmt.Errorf("HyDE LLM 调用失败: %w", err)
	}
	hydeDoc := strings.TrimSpace(resp)
	if len([]rune(hydeDoc)) < g.minDocLength {
		return "", fmt.Errorf("HyDE 文档过短: %d 字符（最小 %d）", len([]rune(hydeDoc)), g.minDocLength)
	}
	return hydeDoc, nil
}
