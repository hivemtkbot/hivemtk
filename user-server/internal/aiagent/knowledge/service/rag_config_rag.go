package service

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	rag_service "hivemtk-user/internal/aiagent/rag/service"
	"hivemtk-user/internal/pkg/utils/logger"
	"strconv"
)

// AddKnowledgeBaseDocument 添加知识库文档
func (s *RagConfigService) AddKnowledgeBaseDocument(ctx context.Context, productID string, doc rag_core.Document) error {
	product, err := s.repo.GetRagProductByID(ctx, productID)
	if err != nil {
		return fmt.Errorf("failed to get rag product: %w", err)
	}
	if product == nil {
		return errors.New("rag product not found")
	}

	chunks, err := s.documentProcessor.ProcessDocument(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}

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

	err = s.vectorProcessor.ProcessAndStore(ctx, texts, metadatas)
	if err != nil {
		return fmt.Errorf("failed to store document vectors: %w", err)
	}

	return nil
}

// QueryKnowledgeBase 查询知识库
//
// 直接走 pgvector + TEI bge-m3 向量检索。
// 这样 RAG 召回与 RAG 产品实际写入的 knowledge_chunks 数据保持一致。
func (s *RagConfigService) QueryKnowledgeBase(ctx context.Context, productID, query string, topK int) (*rag_service.QueryResponse, error) {
	product, err := s.repo.GetRagProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rag product: %w", err)
	}
	if product == nil {
		return nil, errors.New("rag product not found")
	}
	if topK <= 0 {
		topK = DefaultTopK()
	}

	productNumericID := productID

	// per 知识库覆盖：若配置了 text-embedding 或 rerank，构造临时 HybridSearcher（不碰共享单例，避免竞态）
	var merchantChunks []MerchantRAGChunk
	var vecErr error
	if product.EmbeddingProviderConfig.BaseURL != "" || (product.RerankProviderConfig.BaseURL != "" && product.RerankProviderConfig.Enabled) {
		embClient := llm.NewEmbeddingService()
		if product.EmbeddingProviderConfig.BaseURL != "" {
			dim := product.EmbeddingProviderConfig.Dimension
			if dim == 0 {
				dim = EmbeddingDim()			}
			embClient = llm.NewEmbeddingServiceWithConfig(&llm.EmbeddingConfig{
				APIType:        "openai",
				BaseURL:        product.EmbeddingProviderConfig.BaseURL,
				Model:          product.EmbeddingProviderConfig.Model,
				APIKey:         product.EmbeddingProviderConfig.APIKey,
				Dimension:      dim,
				AllowFallback:  false,
				RequestTimeout: DefaultRequestTimeoutSeconds(),
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
		logger.Errorf("[QueryKnowledgeBase] vector search failed: %v", vecErr)
	}

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

	answer := ""
	if len(references) > 0 {
		if s.llmServiceConfigured(product) {
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
				SimilarityThreshold: DefaultSimilarityThreshold(),
				VectorDimension: EmbeddingDim(),
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

