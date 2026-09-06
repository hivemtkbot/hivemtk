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
		h.Write([]byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)})
		u := h.Sum64()
		vec[i] = float32(int64(u%2000001)-1000000) / 1000000.0
	}

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
