package ragretrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"marketing/internal/aiagent/llm"
)

// EmbeddingProviderOpenAI 默认 provider：基于 llm.EmbeddingService（OpenAI 兼容）
const EmbeddingProviderOpenAI = "openai"

// EmbeddingClient 真实向量化客户端(由 LLM 包的 EmbeddingService 提供)
type EmbeddingClient = llm.EmbeddingServiceInterface

// Vectorizer 真实向量化器实现
// 底层走 OpenAI 兼容协议的 /v1/embeddings,无 API Key 时显式降级为本地哈希实现并在日志中标记,
// 严禁在生产环境静默返回伪向量。
type Vectorizer struct {
	dimension int
	embedder  EmbeddingClient
	config    *llm.EmbeddingConfig
}

// NewVectorizer 创建真实向量化器
// dimension: 期望向量维度,0 表示跟随 embedding 模型默认维度
// 默认 1024（bge-m3 维度），与本地 TEI 容器真实 bge-m3 输出一致。
func NewVectorizer(dimension int, embedder EmbeddingClient) *Vectorizer {
	if embedder == nil {
		embedder = llm.NewEmbeddingService()
	}
	if dimension <= 0 {
		dimension = 1024
	}
	v := &Vectorizer{
		dimension: dimension,
		embedder:  embedder,
		config:    embedder.DefaultConfig(),
	}
	// 将向量化器维度同步到 embedding 配置,确保离线降级实现与期望维度一致
	v.config.Dimension = dimension
	return v
}

// NewVectorizerFromConfig 根据 provider 配置选择向量化器实现
//
// 配置驱动：根据 provider 字段切换实现，不硬编码
//   - "openai"（默认）：返回 *Vectorizer（基于 llm.EmbeddingService，OpenAI 兼容）
//   - "bge-m3"：返回 *BGEM3Vectorizer（直接走 OpenAI 兼容 /v1/embeddings，支持 normalize/batch_size）
//   - 其他值：回退到 "openai" 并记录警告
//
// 向后兼容：provider 为空时使用默认 "openai"，行为与 NewVectorizer 一致。
func NewVectorizerFromConfig(provider string, bgeM3Cfg BGEM3Config) VectorizerInterface {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = EmbeddingProviderOpenAI
	}
	switch provider {
	case BGEM3ProviderName:
		return NewBGEM3Vectorizer(bgeM3Cfg)
	case EmbeddingProviderOpenAI:
		fallthrough
	default:
		// 默认走 *Vectorizer（基于 llm.EmbeddingService）
		// 维度跟随 bgeM3Cfg.Dimension（保持与配置一致），<=0 时由 NewVectorizer 兜底 1024
		return NewVectorizer(bgeM3Cfg.Dimension, nil)
	}
}

// EmbedText 将文本转换为向量(真实 API 调用)
func (v *Vectorizer) EmbedText(text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text cannot be empty")
	}
	vec, err := v.embedder.EmbedOne(context.Background(), v.config, text)
	if err != nil {
		return nil, fmt.Errorf("embedding 调用失败: %w", err)
	}
	return vec, nil
}

// EmbedBatch 批量向量化(真实 API 调用)
func (v *Vectorizer) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("text cannot be empty")
		}
	}
	vectors, err := v.embedder.Embed(context.Background(), v.config, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding 批量调用失败: %w", err)
	}
	return vectors, nil
}

// GetDimension 获取向量维度
func (v *Vectorizer) GetDimension() int { return v.dimension }

// ValidateEmbedding 验证向量有效性
func (v *Vectorizer) ValidateEmbedding(embedding []float32) bool {
	if len(embedding) != v.dimension {
		return false
	}
	for _, val := range embedding {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return false
		}
	}
	return true
}

// CosineSimilarity 计算余弦相似度
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// JaccardSimilarity 计算Jaccard相似度（基于词汇重叠）
func JaccardSimilarity(text1, text2 string) float64 {
	words1 := getUniqueWords(text1)
	words2 := getUniqueWords(text2)

	intersection := 0
	union := len(words2) // 初始化为集合2的大小

	// 计算交集和调整并集大小
	for word := range words1 {
		if _, exists := words2[word]; exists {
			intersection++
		} else {
			union++ // 如果word不在集合2中，则它在并集中
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// getUniqueWords 获取唯一词汇集合
func getUniqueWords(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	wordSet := make(map[string]bool)

	for _, word := range words {
		// 移除标点符号
		cleanWord := strings.Trim(word, ".,!?;:\"'()[]{}")
		if cleanWord != "" {
			wordSet[cleanWord] = true
		}
	}

	return wordSet
}
