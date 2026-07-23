package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/aiagent/llm"
	rag_core "marketing/internal/aiagent/rag/core"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
	rag_service "marketing/internal/aiagent/rag/service"
	"marketing/internal/pkg/utils/logger"
	"strconv"
)

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
		topK = DefaultTopK
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
				dim = EmbeddingDim
			}
			embClient = llm.NewEmbeddingServiceWithConfig(&llm.EmbeddingConfig{
				APIType:        "openai",
				BaseURL:        product.EmbeddingProviderConfig.BaseURL,
				Model:          product.EmbeddingProviderConfig.Model,
				APIKey:         product.EmbeddingProviderConfig.APIKey,
				Dimension:      dim,
				AllowFallback:  false,
				RequestTimeout: DefaultRequestTimeoutSeconds,
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
				SimilarityThreshold: DefaultSimilarityThreshold,
				VectorDimension:     EmbeddingDim,
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
