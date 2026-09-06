// Package service 知识库子域 —— 运行时可配置参数（DB 驱动读取）
//
// 设计：本包不 import internal/service（避免循环依赖），
// 而是通过 SetConfigReader 由装配层在启动时注入 DB 驱动的读取器。
// 未注入时 fallback 到硬编码默认值。
package service

import (
	"context"
	"time"
)

// ConfigReader 配置参数读取接口（由 internal/service 在启动时注入）
type ConfigReader interface {
	GetInt(ctx context.Context, group, key string, fallback int) int
	GetFloat(ctx context.Context, group, key string, fallback float64) float64
	GetBool(ctx context.Context, group, key string, fallback bool) bool
	GetDuration(ctx context.Context, group, key string, fallback time.Duration) time.Duration
}

var globalReader ConfigReader

// SetConfigReader 装配层注入 DB 驱动的 ConfigParamService
// （internal/service 在 SeedConfigParams 后调用）
func SetConfigReader(r ConfigReader) {
	globalReader = r
}

// EmbeddingDim 默认 embedding 维度（bge-m3 / TEI 1024 维）
// seed: knowledge.embedding_dimension
func EmbeddingDim() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "knowledge", "embedding_dimension", 1024)
	}
	return 1024
}

// AsyncProcessingTimeout 文档异步处理（分片/embedding/索引）总超时
// seed: knowledge.async_processing_timeout
func AsyncProcessingTimeout() time.Duration {
	if globalReader != nil {
		return globalReader.GetDuration(context.Background(), "knowledge", "async_processing_timeout", 15*time.Minute)
	}
	return 15 * time.Minute
}

// ExternalImportTimeout 外部导入（飞书/Notion）总超时
// seed: knowledge.external_import_timeout
func ExternalImportTimeout() time.Duration {
	if globalReader != nil {
		return globalReader.GetDuration(context.Background(), "knowledge", "external_import_timeout", 30*time.Minute)
	}
	return 30 * time.Minute
}

// SSRFCheckTimeout SSRF 防护 URL 校验超时
// seed: knowledge.ssrf_check_timeout
func SSRFCheckTimeout() time.Duration {
	if globalReader != nil {
		return globalReader.GetDuration(context.Background(), "knowledge", "ssrf_check_timeout", 5*time.Second)
	}
	return 5 * time.Second
}

// DefaultTopK 默认 top-K 检索数
// seed: knowledge.default_top_k
func DefaultTopK() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "knowledge", "default_top_k", 5)
	}
	return 5
}

// ChunkContentPreview 检索结果 Content 字段截断长度
// seed: knowledge.chunk_preview_max_len
func ChunkContentPreview() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "knowledge", "chunk_preview_max_len", 500)
	}
	return 500
}

// MaxSearchListSize 检索扫描上限（BM25-lite 兜底）
// seed: knowledge.max_search_list_size
func MaxSearchListSize() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "knowledge", "max_search_list_size", 1000)
	}
	return 1000
}

// BM25ScanLimit BM25 文本匹配扫描上限
// seed: knowledge.bm25_scan_limit
func BM25ScanLimit() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "knowledge", "bm25_scan_limit", 10000)
	}
	return 10000
}

// DefaultSimilarityThreshold 默认相似度阈值
// seed: knowledge.similarity_threshold
func DefaultSimilarityThreshold() float64 {
	if globalReader != nil {
		return globalReader.GetFloat(context.Background(), "knowledge", "similarity_threshold", 0.5)
	}
	return 0.5
}

// DefaultTemperature 默认 temperature
// seed: agent_llm.temperature
func DefaultTemperature() float64 {
	if globalReader != nil {
		return globalReader.GetFloat(context.Background(), "agent_llm", "temperature", 0.7)
	}
	return 0.7
}

// DefaultMaxTokens 默认 max_tokens
// seed: agent_llm.max_tokens
func DefaultMaxTokens() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "agent_llm", "max_tokens", 1000)
	}
	return 1000
}

// DefaultTopP 默认 top_p
// seed: agent_llm.top_p
func DefaultTopP() float64 {
	if globalReader != nil {
		return globalReader.GetFloat(context.Background(), "agent_llm", "top_p", 0.9)
	}
	return 0.9
}

// DefaultRequestTimeoutSeconds 默认 LLM 请求超时（秒）
// seed: agent_llm.request_timeout
func DefaultRequestTimeoutSeconds() int {
	if globalReader != nil {
		return int(globalReader.GetDuration(context.Background(), "agent_llm", "request_timeout", 60*time.Second).Seconds())
	}
	return 60
}

// DefaultMaxRetries 默认 LLM 最大重试
// seed: agent_llm.max_retries
func DefaultMaxRetries() int {
	if globalReader != nil {
		return globalReader.GetInt(context.Background(), "agent_llm", "max_retries", 3)
	}
	return 3
}

// DefaultPageSize 默认分页大小
const DefaultPageSize = 20

// DefaultFrequencyPenalty 默认 frequency_penalty
const DefaultFrequencyPenalty = 0.5

// DefaultPresencePenalty 默认 presence_penalty
const DefaultPresencePenalty = 0.5
