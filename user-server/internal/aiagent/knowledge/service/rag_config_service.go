package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	knowledgerepo "marketing/internal/aiagent/knowledge/repository"
	rag_service "marketing/internal/aiagent/rag/service"
	"marketing/internal/aiagent/vector"
	"marketing/internal/etl"
	"marketing/internal/model"
	"strings"
	"time"
)

// RagConfigService RAG配置服务
type RagConfigService struct {
	repo              *knowledgerepo.RagConfigRepository
	ragService        *rag_service.RAGService
	documentProcessor *etl.DocumentProcessor
	vectorProcessor   *vector.VectorProcessor
	// 2026-07-18：直接挂 RagSearcher，让 QueryKnowledgeBase 走真实 pgvector 而非内存 RAGEngine
	ragSearcher *RagSearcher
}

// NewRagConfigService 创建RAG配置服务
func NewRagConfigService(
	repo *knowledgerepo.RagConfigRepository,
	ragService *rag_service.RAGService,
	documentProcessor *etl.DocumentProcessor,
	vectorProcessor *vector.VectorProcessor,
) *RagConfigService {
	return &RagConfigService{
		repo:              repo,
		ragService:        ragService,
		documentProcessor: documentProcessor,
		vectorProcessor:   vectorProcessor,
		// 2026-07-18：自动初始化向量检索器（pgvector + TEI bge-m3）
		ragSearcher: NewRagSearcher(),
	}
}

// CreateRagProduct 创建RAG产品
func (s *RagConfigService) CreateRagProduct(ctx context.Context, req *model.RagProduct) (*model.RagProduct, error) {
	if req.Name == "" {
		return nil, errors.New("product name is required")
	}

	if req.Category == "" {
		return nil, errors.New("product category is required")
	}

	// 设置默认值
	if req.Temperature == 0 {
		req.Temperature = DefaultTemperature
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}
	if req.TopP == 0 {
		req.TopP = DefaultTopP
	}
	if req.FrequencyPenalty == 0 {
		req.FrequencyPenalty = DefaultFrequencyPenalty
	}
	if req.PresencePenalty == 0 {
		req.PresencePenalty = DefaultPresencePenalty
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "json_object"
	}

	// 设置默认的LLM提供者配置
	if req.LLMProviderConfig.APIType == "" {
		req.LLMProviderConfig.APIType = "openai"
	}
	if req.LLMProviderConfig.Model == "" {
		req.LLMProviderConfig.Model = req.LLMModel
	}
	if req.LLMProviderConfig.MaxRetries == 0 {
		req.LLMProviderConfig.MaxRetries = DefaultMaxRetries
	}
	if req.LLMProviderConfig.RequestTimeout == 0 {
		req.LLMProviderConfig.RequestTimeout = DefaultRequestTimeoutSeconds
	}

	// 设置默认的 text-embedding / rerank 配置（per 知识库，可被后台覆盖）
	if req.EmbeddingProviderConfig.APIType == "" {
		req.EmbeddingProviderConfig.APIType = "openai"
	}
	if req.EmbeddingProviderConfig.Model == "" {
		req.EmbeddingProviderConfig.Model = req.EmbeddingModel
	}
	if req.EmbeddingProviderConfig.Dimension == 0 {
		req.EmbeddingProviderConfig.Dimension = req.EmbeddingDim
		if req.EmbeddingProviderConfig.Dimension == 0 {
			req.EmbeddingProviderConfig.Dimension = EmbeddingDim
		}
	}
	req.EmbeddingProviderConfig.Enabled = true
	if req.RerankProviderConfig.APIType == "" {
		req.RerankProviderConfig.APIType = "openai"
	}
	if req.RerankProviderConfig.Model == "" {
		req.RerankProviderConfig.Model = "BAAI/bge-reranker-v2-m3"
	}
	req.RerankProviderConfig.Enabled = true

	// 修复：vector_table 有 uniqueIndex，未提供时 PG 会因空串重复导致 duplicate key 错误
	// 兜底用 Name 生成 deterministic 的 table 名（kebab-case + 随机后缀）
	if req.VectorTable == "" {
		req.VectorTable = sanitizeVectorTableName(req.Name) + "_" + randomHex(4)
	}

	req.IsActive = true
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	err := s.repo.CreateRagProduct(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create rag product: %w", err)
	}

	return req, nil
}

// sanitizeVectorTableName 把人类可读 Name 转成 PG 安全的 snake_case 表名
func sanitizeVectorTableName(name string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", "（", "", "）", "", "(", "", ")", "", "，", "", "。", "", "、", "")
	cleaned := r.Replace(name)
	// 移除非字母数字下划线字符
	var b strings.Builder
	for _, c := range cleaned {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		}
	}
	out := strings.ToLower(b.String())
	if out == "" {
		out = "rag"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// randomHex 生成 n 字节随机十六进制字符串
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GetRagProduct 获取RAG产品
func (s *RagConfigService) GetRagProduct(ctx context.Context, id string) (*model.RagProduct, error) {
	return s.repo.GetRagProductByID(ctx, id)
}

// ListRagProducts 列出RAG产品
func (s *RagConfigService) ListRagProducts(ctx context.Context) ([]*model.RagProduct, error) {
	return s.repo.ListRagProducts(ctx)
}

// UpdateRagProduct 更新RAG产品
//
// 2026-07-18 修复：
//   - 数值字段（Temperature/MaxTokens/TopP/FrequencyPenalty/PresencePenalty）允许为 0，
//     表示用户没传，沿用原值。这避免 PATCH 语义下"少传一个字段就被验证拒绝"的问题。
//   - VectorTable 字段必须保留原值（不能被空串覆盖，否则 uniqueIndex 会冲突）。
//   - 实际修改采用 SELECT-then-MERGE 模式，确保未指定字段不会被清空。
func (s *RagConfigService) UpdateRagProduct(ctx context.Context, req *model.RagProduct) error {
	// 验证必填字段
	if req.ID == "" {
		return errors.New("product ID is required")
	}
	if req.Name == "" {
		return errors.New("product name is required")
	}

	// 加载原始产品，做 merge（避免 PATCH 语义下覆盖问题）
	original, err := s.repo.GetRagProductByID(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("failed to load original product: %w", err)
	}
	if original == nil {
		return errors.New("rag product not found")
	}

	// 数值字段：仅在 > 0 时覆盖（避免 0 误清空已有值）
	if req.Temperature > 0 {
		if req.Temperature < 0 || req.Temperature > 2 {
			return errors.New("temperature must be between 0 and 2")
		}
		original.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		if req.MaxTokens > 4096 {
			return errors.New("max_tokens must be between 1 and 4096")
		}
		original.MaxTokens = req.MaxTokens
	}
	if req.TopP > 0 {
		if req.TopP < 0 || req.TopP > 1 {
			return errors.New("top_p must be between 0 and 1")
		}
		original.TopP = req.TopP
	}
	if req.FrequencyPenalty != 0 {
		if req.FrequencyPenalty < -2 || req.FrequencyPenalty > 2 {
			return errors.New("frequency_penalty must be between -2 and 2")
		}
		original.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != 0 {
		if req.PresencePenalty < -2 || req.PresencePenalty > 2 {
			return errors.New("presence_penalty must be between -2 and 2")
		}
		original.PresencePenalty = req.PresencePenalty
	}

	// 文本字段：非空时覆盖
	if req.Name != "" {
		original.Name = req.Name
	}
	if req.Description != "" {
		original.Description = req.Description
	}
	if req.Category != "" {
		original.Category = req.Category
	}
	if req.EmbeddingModel != "" {
		original.EmbeddingModel = req.EmbeddingModel
	}
	if req.LLMModel != "" {
		original.LLMModel = req.LLMModel
	}
	if req.SystemPrompt != "" {
		original.SystemPrompt = req.SystemPrompt
	}
	if req.ResponseFormat != "" {
		// 验证响应格式
		validResponseFormat := map[string]bool{
			"json_object": true,
			"text":        true,
		}
		if !validResponseFormat[req.ResponseFormat] {
			return errors.New("response_format must be one of: json_object, text")
		}
		original.ResponseFormat = req.ResponseFormat
	}
	if req.LLMProviderConfig.APIType != "" {
		// 验证API类型
		validAPIType := map[string]bool{
			"openai":    true,
			"anthropic": true,
			"custom":    true,
			"azure":     true,
		}
		if !validAPIType[req.LLMProviderConfig.APIType] {
			return errors.New("api_type must be one of: openai, anthropic, custom, azure")
		}
		original.LLMProviderConfig.APIType = req.LLMProviderConfig.APIType
	}
	if req.LLMProviderConfig.Model != "" {
		original.LLMProviderConfig.Model = req.LLMProviderConfig.Model
	}
	if req.LLMProviderConfig.APIKey != "" {
		original.LLMProviderConfig.APIKey = req.LLMProviderConfig.APIKey
	}
	if req.LLMProviderConfig.BaseURL != "" {
		original.LLMProviderConfig.BaseURL = req.LLMProviderConfig.BaseURL
	}

	// 文本向量(text-embedding)供应商配置合并（per 知识库覆盖全局）
	if req.EmbeddingProviderConfig.APIType != "" {
		original.EmbeddingProviderConfig.APIType = req.EmbeddingProviderConfig.APIType
	}
	if req.EmbeddingProviderConfig.Model != "" {
		original.EmbeddingProviderConfig.Model = req.EmbeddingProviderConfig.Model
	}
	if req.EmbeddingProviderConfig.APIKey != "" {
		original.EmbeddingProviderConfig.APIKey = req.EmbeddingProviderConfig.APIKey
	}
	if req.EmbeddingProviderConfig.BaseURL != "" {
		original.EmbeddingProviderConfig.BaseURL = req.EmbeddingProviderConfig.BaseURL
	}
	if req.EmbeddingProviderConfig.Dimension > 0 {
		original.EmbeddingProviderConfig.Dimension = req.EmbeddingProviderConfig.Dimension
	}
	original.EmbeddingProviderConfig.Enabled = req.EmbeddingProviderConfig.Enabled

	// 重排(rerank)供应商配置合并（per 知识库覆盖全局）
	if req.RerankProviderConfig.APIType != "" {
		original.RerankProviderConfig.APIType = req.RerankProviderConfig.APIType
	}
	if req.RerankProviderConfig.Model != "" {
		original.RerankProviderConfig.Model = req.RerankProviderConfig.Model
	}
	if req.RerankProviderConfig.APIKey != "" {
		original.RerankProviderConfig.APIKey = req.RerankProviderConfig.APIKey
	}
	if req.RerankProviderConfig.BaseURL != "" {
		original.RerankProviderConfig.BaseURL = req.RerankProviderConfig.BaseURL
	}
	original.RerankProviderConfig.Enabled = req.RerankProviderConfig.Enabled

	// 关键：VectorTable 永远保留原值（uniqueIndex 不能被空串覆盖）
	// 这里不需要赋值，original 已有原值

	original.UpdatedAt = time.Now()

	return s.repo.UpdateRagProduct(ctx, original)
}

// DeleteRagProduct 删除RAG产品
func (s *RagConfigService) DeleteRagProduct(ctx context.Context, id string) error {
	return s.repo.DeleteRagProduct(ctx, id)
}
