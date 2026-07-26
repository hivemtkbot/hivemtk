package ragretrieval

// bge_m3_vectorizer.go bge-m3 多语言 embedding provider
//
// 五层架构归属: L4 能力层
// 设计依据: bge-m3 配置化
//
// 与 vectorizer.go 中 Vectorizer（基于 llm.EmbeddingService）的差异：
//   - Vectorizer：默认 provider（openai 兼容），通过 llm.EmbeddingService 间接调用
//   - BGEM3Vectorizer：显式 bge-m3 provider，直接走 OpenAI 兼容 /v1/embeddings，
//     支持 normalize / batch_size 等 bge-m3 专属参数；不依赖 llm 包，便于在
//     i18n 多语言路径独立配置（base_url / api_key / model 可与默认 embedding 不同）
//
// 兼容性：
//   - 实现 VectorizerInterface，可直接替换 Vectorizer 注入到 RagRetrievalService
//   - 默认维度 1024（与 pgvector vector(1024) 对齐）
//   - API 兼容 OpenAI /v1/embeddings 协议，可对接：
//     * 本地部署：FlagEmbedding + ONNX / llama.cpp / TEI
//     * API 模式：SiliconFlow / 智谱 BigModel 等 OpenAI 兼容服务

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// BGEM3ProviderName bge-m3 provider 标识
const BGEM3ProviderName = "bge-m3"

// BGEM3DefaultDimension bge-m3 默认向量维度（与 pgvector vector(1024) 一致）
const BGEM3DefaultDimension = 1024

// BGEM3DefaultModel bge-m3 默认模型名
const BGEM3DefaultModel = "BAAI/bge-m3"

// BGEM3Config bge-m3 embedding provider 配置
//
// 与 InferenceEmbeddingConfig 的差异：
//   - 本配置面向 i18n 多语言路径，可独立于默认推理栈 inference.embedding
//   - 新增 normalize / batch_size 参数（bge-m3 专属）
type BGEM3Config struct {
	// Provider provider 类型，固定 "bge-m3"
	Provider string `yaml:"provider" json:"provider"`
	// Model 模型名，默认 BAAI/bge-m3
	Model string `yaml:"model" json:"model"`
	// BaseURL OpenAI 兼容 /v1/embeddings 根路径
	// 如 http://localhost:8080/v1、https://api.siliconflow.cn/v1
	BaseURL string `yaml:"base_url" json:"base_url"`
	// APIKey 鉴权密钥（本地部署可空）
	APIKey string `yaml:"api_key" json:"api_key"`
	// Dimension 向量维度，默认 1024
	Dimension int `yaml:"dimension" json:"dimension"`
	// Normalize 是否对返回向量做 L2 归一化（bge-m3 推荐 true）
	Normalize bool `yaml:"normalize" json:"normalize"`
	// BatchSize 单次请求最大文本数，默认 32
	BatchSize int `yaml:"batch_size" json:"batch_size"`
	// RequestTimeout 单次请求超时（秒），默认 60
	RequestTimeout int `yaml:"request_timeout" json:"request_timeout"`
	// MaxRetries 失败重试次数，默认 3
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
}

// DefaultBGEM3Config 默认 bge-m3 配置
//
// 默认走本地 llama.cpp / TEI（http://localhost:8080/v1），
// 私域部署数据不出域；可通过 BaseURL/APIKey 切换到 OpenAI 兼容云端服务。
func DefaultBGEM3Config() BGEM3Config {
	return BGEM3Config{
		Provider:       BGEM3ProviderName,
		Model:          BGEM3DefaultModel,
		BaseURL:        "http://localhost:8080/v1",
		APIKey:         "",
		Dimension:      BGEM3DefaultDimension,
		Normalize:      true,
		BatchSize:      32,
		RequestTimeout: 60,
		MaxRetries:     3,
	}
}

// BGEM3Vectorizer bge-m3 多语言 embedding provider
//
// 支持本地部署（FlagEmbedding + ONNX / llama.cpp / TEI）和 API 模式
// （SiliconFlow / 智谱等 OpenAI 兼容服务）。
type BGEM3Vectorizer struct {
	baseURL    string
	apiKey     string
	model      string
	dimension  int
	normalize  bool
	batchSize  int
	maxRetries int
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewBGEM3Vectorizer 构造 bge-m3 向量化器
//
// cfg.Dimension <= 0 时使用默认 1024；cfg.BatchSize <= 0 时使用默认 32。
// cfg.BaseURL 为空时使用默认 http://localhost:8080/v1。
func NewBGEM3Vectorizer(cfg BGEM3Config) *BGEM3Vectorizer {
	if cfg.Model == "" {
		cfg.Model = BGEM3DefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080/v1"
	}
	if cfg.Dimension <= 0 {
		cfg.Dimension = BGEM3DefaultDimension
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 32
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 60
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &BGEM3Vectorizer{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimension:  cfg.Dimension,
		normalize:  cfg.Normalize,
		batchSize:  cfg.BatchSize,
		maxRetries: cfg.MaxRetries,
		httpClient: &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second},
	}
}

// EmbedText 将文本转换为向量
func (v *BGEM3Vectorizer) EmbedText(text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text cannot be empty")
	}
	vecs, err := v.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("bge-m3 embedding 返回空")
	}
	return vecs[0], nil
}

// EmbedBatch 批量向量化
//
// 超过 batchSize 时自动分片串行请求，合并结果保持原始顺序。
func (v *BGEM3Vectorizer) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("text cannot be empty")
		}
	}
	if len(texts) <= v.batchSize {
		return v.embedWithRetry(context.Background(), texts)
	}
	// 分片串行请求
	total := len(texts)
	result := make([][]float32, total)
	for i := 0; i < total; i += v.batchSize {
		end := i + v.batchSize
		if end > total {
			end = total
		}
		batch := texts[i:end]
		vectors, err := v.embedWithRetry(context.Background(), batch)
		if err != nil {
			return nil, fmt.Errorf("bge-m3 embedding 分片 %d-%d 失败: %w", i, end-1, err)
		}
		copy(result[i:end], vectors)
	}
	return result, nil
}

// GetDimension 返回向量维度
func (v *BGEM3Vectorizer) GetDimension() int { return v.dimension }

// ValidateEmbedding 验证向量有效性
func (v *BGEM3Vectorizer) ValidateEmbedding(embedding []float32) bool {
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

// Name 返回 provider 名
func (v *BGEM3Vectorizer) Name() string { return BGEM3ProviderName }

// embedWithRetry 带重试的 embedding 调用
func (v *BGEM3Vectorizer) embedWithRetry(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error
	for attempt := 0; attempt < v.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		vectors, err := v.callAPI(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		logger.Warnf("[BGEM3] embedding 调用失败 (attempt %d/%d): %v", attempt+1, v.maxRetries, err)
	}
	return nil, fmt.Errorf("bge-m3 embedding 不可达，已重试 %d 次: %w", v.maxRetries, lastErr)
}

// bgeM3Request OpenAI 兼容 /v1/embeddings 请求体
type bgeM3Request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// bgeM3Response OpenAI 兼容 /v1/embeddings 响应体
type bgeM3Response struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// callAPI 调用 OpenAI 兼容 /v1/embeddings
func (v *BGEM3Vectorizer) callAPI(ctx context.Context, texts []string) ([][]float32, error) {
	v.mu.RLock()
	baseURL := v.baseURL
	apiKey := v.apiKey
	model := v.model
	v.mu.RUnlock()

	if baseURL == "" {
		return nil, fmt.Errorf("BGEM3 BaseURL 未配置")
	}
	// 兼容 baseURL 已含 /v1 后缀的情况
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
		baseURL = baseURL + "/v1"
	}
	endpoint := baseURL + "/embeddings"

	body, err := json.Marshal(bgeM3Request{Model: model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("bge-m3 鉴权失败 status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("bge-m3 服务端错误 status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bge-m3 API error status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var er bgeM3Response
	if err := json.Unmarshal(respBody, &er); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
	}
	if er.Error != nil {
		return nil, fmt.Errorf("bge-m3 API error: %s", er.Error.Message)
	}
	if len(er.Data) != len(texts) {
		return nil, fmt.Errorf("bge-m3 返回数量不匹配: expect %d, got %d", len(texts), len(er.Data))
	}

	vectors := make([][]float32, len(texts))
	for _, d := range er.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("bge-m3 返回 index 越界: %d", d.Index)
		}
		vec := d.Embedding
		if v.normalize {
			vec = normalizeVector(vec)
		}
		if len(vec) != v.dimension {
			return nil, fmt.Errorf("bge-m3 维度不匹配: expect %d, got %d", v.dimension, len(vec))
		}
		vectors[d.Index] = vec
	}
	return vectors, nil
}

// normalizeVector L2 归一化
func normalizeVector(vec []float32) []float32 {
	if len(vec) == 0 {
		return vec
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := float32(1.0 / math.Sqrt(sum))
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = v * norm
	}
	return out
}

// Compile-time 接口断言：BGEM3Vectorizer 实现 VectorizerInterface
var _ VectorizerInterface = (*BGEM3Vectorizer)(nil)
