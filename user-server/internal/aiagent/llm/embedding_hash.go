package llm

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
)

// HashEmbeddingService 本地哈希 Embedding 降级实现
// 严格说仍是简化实现,只在 API Key 未配置 / 网络不可达时使用,
// 调用方应在日志中明确标记为"降级",禁止在生产环境默默使用。
type HashEmbeddingService struct {
	defaultDim int
}

// NewHashEmbeddingService 构造 HashEmbeddingService
// dim <= 0 时使用默认 1024（跟随 bge-m3）
// 用于测试注入或显式降级场景，生产环境不应直接使用
func NewHashEmbeddingService(dim int) *HashEmbeddingService {
	if dim <= 0 {
		dim = 1024
	}
	return &HashEmbeddingService{defaultDim: dim}
}

// Embed 批量 Embedding
func (h *HashEmbeddingService) Embed(ctx context.Context, cfg *EmbeddingConfig, texts []string) ([][]float32, error) {
	dim := h.defaultDim
	if cfg != nil && cfg.Dimension > 0 {
		dim = cfg.Dimension
	}
	results := make([][]float32, len(texts))
	for i, t := range texts {
		results[i] = hashVector(t, dim)
	}
	return results, nil
}

// EmbedOne 单条 Embedding
func (h *HashEmbeddingService) EmbedOne(ctx context.Context, cfg *EmbeddingConfig, text string) ([]float32, error) {
	dim := h.defaultDim
	if cfg != nil && cfg.Dimension > 0 {
		dim = cfg.Dimension
	}
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	return hashVector(text, dim), nil
}

// DefaultConfig 默认配置
func (h *HashEmbeddingService) DefaultConfig() *EmbeddingConfig {
	return &EmbeddingConfig{Dimension: h.defaultDim, Model: "hash-fallback"}
}

// hashVector 基于文本 + 维度位置哈希生成稳定伪向量
// 保证相同输入得到相同向量,不同输入得到不同向量,且返回单位向量。
// 私域部署基线（2026-07-18）：仅在 EMBEDDING_ALLOW_FALLBACK=true 时使用，
// 默认 dim=1024 跟随 bge-m3 维度。
func hashVector(text string, dim int) []float32 {
	text = strings.ToLower(strings.TrimSpace(text))
	if dim <= 0 {
		dim = 1024
	}
	vec := make([]float32, dim)
	if text == "" {
		return vec
	}

	h := fnv.New64a()
	for i := 0; i < dim; i++ {
		h.Reset()
		h.Write([]byte(text))
		// 将维度索引作为盐值混入,确保各维度值独立
		h.Write([]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)})
		u := h.Sum64()
		// 映射到 [-1, 1] 区间
		vec[i] = float32(int64(u%2000001)-1000000) / 1000000.0
	}

	// 单位化
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec
}
