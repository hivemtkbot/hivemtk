package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	knowledgerepo "marketing/internal/aiagent/knowledge/repository"
	"marketing/internal/aiagent/llm"
	rag_core "marketing/internal/aiagent/rag/core"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	rag_service "marketing/internal/aiagent/rag/service"
	"marketing/internal/aiagent/vector"
	"marketing/internal/etl"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"strconv"
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
		req.Temperature = 0.7
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 1000
	}
	if req.TopP == 0 {
		req.TopP = 0.9
	}
	if req.FrequencyPenalty == 0 {
		req.FrequencyPenalty = 0.5
	}
	if req.PresencePenalty == 0 {
		req.PresencePenalty = 0.5
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
		req.LLMProviderConfig.MaxRetries = 3
	}
	if req.LLMProviderConfig.RequestTimeout == 0 {
		req.LLMProviderConfig.RequestTimeout = 60
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
			req.EmbeddingProviderConfig.Dimension = 1024
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

// GetAccountConfig 获取账号配置
func (s *RagConfigService) GetAccountConfig(ctx context.Context, accountID, platform string) (*model.PlatformAccountConfig, error) {
	config, err := s.repo.GetAccountConfig(ctx, accountID, platform)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// UpdateAccountConfig 更新账号配置
func (s *RagConfigService) UpdateAccountConfig(ctx context.Context, req *model.PlatformAccountConfig) error {
	// 验证平台类型
	validPlatforms := []string{"douyin", "kuaishou", "xiaohongshu", "xianyu"}
	isValidPlatform := false
	for _, p := range validPlatforms {
		if req.Platform == p {
			isValidPlatform = true
			break
		}
	}
	if !isValidPlatform {
		return errors.New("invalid platform, must be one of: douyin, kuaishou, xiaohongshu, xianyu")
	}

	// 如果启用了RAG，验证RAG产品存在
	if req.IsRagEnabled && req.RagProductID != nil {
		_, err := s.repo.GetRagProductByID(ctx, *req.RagProductID)
		if err != nil {
			return fmt.Errorf("rag product not found: %w", err)
		}
	}

	req.UpdatedAt = time.Now()

	return s.repo.UpsertAccountConfig(ctx, req)
}

// ProcessMessage 处理消息
func (s *RagConfigService) ProcessMessage(ctx context.Context, platform, accountID, message string) (string, error) {
	// 获取账号配置
	config, err := s.GetAccountConfig(ctx, accountID, platform)
	if err != nil {
		return "", fmt.Errorf("failed to get account config: %w", err)
	}

	// 检查是否开启自动回复
	if !config.IsAutoReplyEnabled {
		return "", errors.New("auto reply is not enabled for this account")
	}

	// 如果启用了RAG，使用RAG处理
	if config.IsRagEnabled && config.RagProductID != nil {
		ragReply, err := s.processWithRag(ctx, *config.RagProductID, message, platform, accountID)
		if err != nil {
			return "", fmt.Errorf("failed to process with rag: %w", err)
		}
		return ragReply, nil
	}

	// 使用传统规则匹配回复
	ruleReply := s.generateRuleBasedReply(config.ReplyRules, message)
	if ruleReply != "" {
		return ruleReply, nil
	}

	// 默认回复
	return "感谢您的消息，我们会尽快回复您！", nil
}

// processWithRag 使用RAG处理消息
func (s *RagConfigService) processWithRag(ctx context.Context, productID, message, platform, accountID string) (string, error) {
	// 获取RAG产品配置
	product, err := s.repo.GetRagProductByID(ctx, productID)
	if err != nil {
		return "", fmt.Errorf("failed to get rag product: %w", err)
	}

	if product == nil {
		return "", errors.New("rag product not found")
	}

	// 构建LLM配置
	llmConfig := &llm.LLMConfig{
		APIKey:           product.LLMProviderConfig.APIKey,
		BaseURL:          product.LLMProviderConfig.BaseURL,
		APIType:          product.LLMProviderConfig.APIType,
		Model:            product.LLMProviderConfig.Model,
		MaxRetries:       product.LLMProviderConfig.MaxRetries,
		RequestTimeout:   product.LLMProviderConfig.RequestTimeout,
		Temperature:      product.Temperature,
		MaxTokens:        product.MaxTokens,
		TopP:             product.TopP,
		FrequencyPenalty: product.FrequencyPenalty,
		PresencePenalty:  product.PresencePenalty,
		ResponseFormat:   product.ResponseFormat,
		SystemPrompt:     product.SystemPrompt,
	}

	// 构建RAG配置
	ragConfig := &rag_core.RAGConfig{
		ChunkSize:           512,
		ChunkOverlap:        50,
		MaxChunksToRetrieve: 5,
		SimilarityThreshold: 0.5,
		VectorDimension:     1024,
	}

	// 构建查询请求
	queryReq := &rag_service.QueryRequest{
		Query:     message,
		RAGConfig: ragConfig,
		LLMConfig: llmConfig,
		Context: map[string]any{
			"platform":  platform,
			"accountID": accountID,
			"productID": productID,
		},
	}

	// 执行RAG查询
	response, err := s.ragService.Query(ctx, queryReq)
	if err != nil {
		return "", fmt.Errorf("failed to execute rag query: %w", err)
	}

	return response.Answer, nil
}

// generateRuleBasedReply 生成基于规则的回复
func (s *RagConfigService) generateRuleBasedReply(rules []model.ReplyRule, message string) string {
	lowerMsg := strings.ToLower(message)

	// 按优先级排序规则
	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}

		for _, keyword := range rule.Keywords {
			if strings.Contains(lowerMsg, strings.ToLower(keyword)) {
				return rule.ReplyTemplate
			}
		}
	}

	return ""
}

// AddKnowledgeBaseDocument 添加知识库文档
func (s *RagConfigService) AddKnowledgeBaseDocument(ctx context.Context, productID string, doc rag_core.Document) error {
	// 验证产品是否存在
	product, err := s.repo.GetRagProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to get rag product: %w", err)
	}
	if product == nil {
		return errors.New("rag product not found")
	}

	// 处理文档
	chunks, err := s.documentProcessor.ProcessDocument(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}

	// 将分片转换为向量并存储
	texts := make([]string, len(chunks))
	metadatas := make([]map[string]any, len(chunks))

	for i, chunk := range chunks {
		texts[i] = chunk.Content
		metadatas[i] = map[string]any{
			"document_id": chunk.DocumentID,
			"chunk_id":    chunk.ID,
			"product_id":  productID,
		}
	}

	// 使用向量处理器存储
	err = s.vectorProcessor.ProcessAndStore(ctx, texts, metadatas)
	if err != nil {
		return fmt.Errorf("failed to store document vectors: %w", err)
	}

	return nil
}

// QueryKnowledgeBase 查询知识库
//
// 2026-07-18 重构：直接走 pgvector + TEI bge-m3 向量检索，而不是内存 RAGEngine。
// 这样 RAG 召回与 RAG 产品实际写入的 knowledge_chunks 数据保持一致。
func (s *RagConfigService) QueryKnowledgeBase(ctx context.Context, productID, query string, topK int) (*rag_service.QueryResponse, error) {
	// 验证产品是否存在
	product, err := s.repo.GetRagProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rag product: %w", err)
	}
	if product == nil {
		return nil, errors.New("rag product not found")
	}
	if topK <= 0 {
		topK = 5
	}

	// 1) 走 pgvector + TEI 真实向量检索
	productNumericID := HashStringToInt64(productID)

	// per 知识库覆盖：若配置了 text-embedding 或 rerank，构造临时 HybridSearcher（不碰共享单例，避免竞态）
	var merchantChunks []MerchantRAGChunk
	var vecErr error
	if product.EmbeddingProviderConfig.BaseURL != "" || (product.RerankProviderConfig.BaseURL != "" && product.RerankProviderConfig.Enabled) {
		embClient := llm.NewEmbeddingService()
		if product.EmbeddingProviderConfig.BaseURL != "" {
			dim := product.EmbeddingProviderConfig.Dimension
			if dim == 0 {
				dim = 1024
			}
			embClient = llm.NewEmbeddingServiceWithConfig(&llm.EmbeddingConfig{
				APIType:        "openai",
				BaseURL:        product.EmbeddingProviderConfig.BaseURL,
				Model:          product.EmbeddingProviderConfig.Model,
				APIKey:         product.EmbeddingProviderConfig.APIKey,
				Dimension:      dim,
				AllowFallback:  false,
				RequestTimeout: 60,
				MaxRetries:     2,
			})
		}
		var reranker ragretrieval.RerankerInterface
		if product.RerankProviderConfig.BaseURL != "" && product.RerankProviderConfig.Enabled {
			reranker = ragretrieval.NewLocalRerankerWithConfig(&ragretrieval.RerankConfig{
				BaseURL:    product.RerankProviderConfig.BaseURL,
				Model:      product.RerankProviderConfig.Model,
				APIKey:     product.RerankProviderConfig.APIKey,
				Enabled:    true,
				Timeout:    30,
				MaxRetries: 2,
			})
		}
		merchantChunks, vecErr = s.ragSearcher.SearchIndexWithConfig(ctx, productNumericID, query, topK, embClient, reranker)
	} else {
		merchantChunks, vecErr = s.ragSearcher.SearchIndex(ctx, productNumericID, query, topK, nil)
	}
	if vecErr != nil {
		// 记录但不直接失败（与既有兜底语义一致）
		logger.Errorf("[QueryKnowledgeBase] vector search failed: %v", vecErr)
	}

	// 2) 把 MerchantRAGChunk 转为 rag_core.Chunk
	references := make([]rag_core.Chunk, 0, len(merchantChunks))
	var maxScore float64
	for _, mc := range merchantChunks {
		references = append(references, rag_core.Chunk{
			ID:         strconv.FormatUint(mc.ID, 10),
			DocumentID: strconv.FormatUint(mc.DocumentID, 10),
			Content:    mc.Content,
			Score:      mc.Score,
			Metadata: map[string]any{
				"product_id": productID,
				"source":     "pgvector",
			},
		})
		if mc.Score > maxScore {
			maxScore = mc.Score
		}
	}

	// 3) 构造 Answer：优先 LLM（如果配置了），否则拼装 chunk 摘要
	answer := ""
	if len(references) > 0 {
		if s.llmServiceConfigured(product) {
			// 走 LLM 生成（与既有 RAGService 行为一致）
			llmConfig := &llm.LLMConfig{
				APIKey:           product.LLMProviderConfig.APIKey,
				BaseURL:          product.LLMProviderConfig.BaseURL,
				APIType:          product.LLMProviderConfig.APIType,
				Model:            product.LLMProviderConfig.Model,
				MaxRetries:       product.LLMProviderConfig.MaxRetries,
				RequestTimeout:   product.LLMProviderConfig.RequestTimeout,
				Temperature:      product.Temperature,
				MaxTokens:        product.MaxTokens,
				TopP:             product.TopP,
				FrequencyPenalty: product.FrequencyPenalty,
				PresencePenalty:  product.PresencePenalty,
				ResponseFormat:   product.ResponseFormat,
				SystemPrompt:     product.SystemPrompt,
			}
			ragConfig := &rag_core.RAGConfig{
				ChunkSize:           512,
				ChunkOverlap:        50,
				MaxChunksToRetrieve: topK,
				SimilarityThreshold: 0.5,
				VectorDimension:     1024,
			}
			queryReq := &rag_service.QueryRequest{
				Query:     query,
				RAGConfig: ragConfig,
				LLMConfig: llmConfig,
				Context: map[string]any{
					"productID": productID,
				},
			}
			if resp, qErr := s.ragService.Query(ctx, queryReq); qErr == nil && resp != nil && resp.Answer != "" {
				answer = resp.Answer
			} else if qErr != nil {
				logger.Errorf("[QueryKnowledgeBase] LLM 调用失败: %v,使用 chunk 摘要兜底", qErr)
			}
		}
		// 兜底:用 chunk 内容拼接
		if answer == "" {
			answer = "根据知识库：\n" + references[0].Content
		}
	} else {
		answer = "未在知识库中检索到相关内容"
	}

	return &rag_service.QueryResponse{
		Answer:     answer,
		References: references,
		Metadata: map[string]any{
			"product_id":   productID,
			"product_name": product.Name,
			"query":        query,
			"top_k":        topK,
			"source_count": len(references),
			"max_score":    maxScore,
			"vector_path":  "pgvector+bge-m3",
		},
		ExecTime: 0,
	}, nil
}

// llmServiceConfigured 判断 RAG 产品是否配置了可用的 LLM
func (s *RagConfigService) llmServiceConfigured(product *model.RagProduct) bool {
	if product == nil {
		return false
	}
	cfg := product.LLMProviderConfig
	if cfg.APIKey == "" && cfg.BaseURL == "" {
		return false
	}
	return true
}
