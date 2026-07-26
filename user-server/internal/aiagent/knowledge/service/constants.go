// Package service 知识库子域常量
package service

import "time"

// ============================================================================
// Embedding / 向量维度
// ============================================================================

// EmbeddingDim 默认 embedding 维度（bge-m3 / TEI 1024 维）
const EmbeddingDim = 1024

// ============================================================================
// 异步处理超时
// ============================================================================

// AsyncProcessingTimeout 文档异步处理（分片/embedding/索引）总超时
const AsyncProcessingTimeout = 15 * time.Minute

// ExternalImportTimeout 外部导入（飞书/Notion）总超时
const ExternalImportTimeout = 30 * time.Minute

// SSRFCheckTimeout SSRF 防护 URL 校验超时
const SSRFCheckTimeout = 5 * time.Second

// ============================================================================
// 检索参数默认值
// ============================================================================

// DefaultTopK 默认 top-K 检索数
const DefaultTopK = 5

// ChunkContentPreview 检索结果 Content 字段截断长度
const ChunkContentPreview = 500

// MaxSearchListSize 检索扫描上限（BM25-lite 兜底）
const MaxSearchListSize = 1000

// BM25ScanLimit BM25 文本匹配扫描上限
const BM25ScanLimit = 10000

// DefaultSimilarityThreshold 默认相似度阈值
const DefaultSimilarityThreshold = 0.5

// ============================================================================
// 分页默认值
// ============================================================================

// DefaultPageSize 默认分页大小
const DefaultPageSize = 20

// ============================================================================
// LLM 默认参数
// ============================================================================

// DefaultTemperature 默认 temperature
const DefaultTemperature = 0.7

// DefaultMaxTokens 默认 max_tokens
const DefaultMaxTokens = 1000

// DefaultTopP 默认 top_p
const DefaultTopP = 0.9

// DefaultFrequencyPenalty 默认 frequency_penalty
const DefaultFrequencyPenalty = 0.5

// DefaultPresencePenalty 默认 presence_penalty
const DefaultPresencePenalty = 0.5

// DefaultRequestTimeoutSeconds 默认 LLM 请求超时（秒）
const DefaultRequestTimeoutSeconds = 60

// DefaultMaxRetries 默认 LLM 最大重试
const DefaultMaxRetries = 3
