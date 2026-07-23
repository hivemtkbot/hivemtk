package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/aiagent/llm"
	rag_core "marketing/internal/aiagent/rag/core"
	rag_service "marketing/internal/aiagent/rag/service"
	"marketing/internal/model"
	"strings"
	"time"
)

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
		MaxChunksToRetrieve: DefaultTopK,
		SimilarityThreshold: DefaultSimilarityThreshold,
		VectorDimension:     EmbeddingDim,
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
