package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/dto"
	"hivemtk-user/internal/geo/llm"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// VerificationService AI 搜索验证服务
type VerificationService struct {
	verifyRepo  repository.GeoVerifyResultRepository
	apiCallRepo repository.GeoAPICallRepository
	llmFactory  *llm.LLMFactory
}

// NewVerificationService 创建 AI 搜索验证服务
func NewVerificationService(vr repository.GeoVerifyResultRepository, acr repository.GeoAPICallRepository, f *llm.LLMFactory) *VerificationService {
	return &VerificationService{
		verifyRepo:  vr,
		apiCallRepo: acr,
		llmFactory:  f,
	}
}

// verifyLLMResponse LLM 验证响应结构
type verifyLLMResponse struct {
	Response            string   `json:"response"`
	BrandMentioned      bool     `json:"brand_mentioned"`
	MentionCount       int      `json:"mention_count"`
	MentionPositions   []string `json:"mention_positions"`
	Sentiment           string   `json:"sentiment"`
	CompetitorsMentioned []string `json:"competitors_mentioned"`
}

// VerifyArticle 执行 AI 搜索验证（迁移自 ai_search_verifier.py）
func (s *VerificationService) VerifyArticle(ctx context.Context, req dto.VerifyRequest) (*model.GeoVerifyResult, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	// 优先使用请求中指定的模型（视为 provider 名）
	selectedProvider := provider
	if len(req.Models) > 0 {
		for _, m := range req.Models {
			if p, err := s.llmFactory.GetProvider(m); err == nil && p != nil {
				selectedProvider = p
				break
			}
		}
	}

	brandName := req.BrandName
	query := req.Query
	prompt := llm.VerifySearchPrompt(brandName, query)

	llmReq := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := selectedProvider.Chat(llmReq)
	if err != nil {
		return nil, fmt.Errorf("AI 搜索验证失败: %w", err)
	}

	s.recordAPICall(selectedProvider.Name(), resp, "verify_search")

	parsed := parseVerifyResponse(resp.Content)

	result := &model.GeoVerifyResult{
		ArticleID:      req.ArticleID,
		Model:          resp.Model,
		Query:          query,
		Response:       parsed.Response,
		BrandMentioned: parsed.BrandMentioned,
		MentionCount:   parsed.MentionCount,
		Sentiment:      parsed.Sentiment,
		Position:       strings.Join(parsed.MentionPositions, "; "),
	}

	if err := s.verifyRepo.Create(result); err != nil {
		return nil, fmt.Errorf("保存验证结果失败: %w", err)
	}

	return result, nil
}

// parseVerifyResponse 解析验证响应
func parseVerifyResponse(content string) *verifyLLMResponse {
	result := &verifyLLMResponse{}
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), result)
	return result
}

// negativeMonitorResult 负面监控结果
type negativeMonitorResult struct {
	Queries []struct {
		Query             string   `json:"query"`
		Response          string   `json:"response"`
		BrandMentioned    bool     `json:"brand_mentioned"`
		MentionCount      int      `json:"mention_count"`
		IsNegative        bool     `json:"is_negative"`
		NegativeScore     float64  `json:"negative_score"`
		NegativeKeywords  []string `json:"negative_keywords"`
		RiskLevel         string   `json:"risk_level"`
		RiskDescription   string   `json:"risk_description"`
	} `json:"queries"`
	Summary struct {
		TotalQueries        int      `json:"total_queries"`
		HighRiskCount       int      `json:"high_risk_count"`
		MediumRiskCount     int      `json:"medium_risk_count"`
		LowRiskCount        int      `json:"low_risk_count"`
		AverageMentionCount float64  `json:"average_mention_count"`
		Alerts              []string `json:"alerts"`
		Recommendations    []string `json:"recommendations"`
	} `json:"summary"`
}

// MonitorNegative 负面提及监控（迁移自 negative_monitor.py）
func (s *VerificationService) MonitorNegative(ctx context.Context, brandName string) (map[string]any, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	prompt := llm.NegativeMonitorPrompt(brandName)
	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.4,
		MaxTokens:   3000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("负面监控失败: %w", err)
	}

	s.recordAPICall(provider.Name(), resp, "negative_monitor")

	parsed := parseNegativeMonitorResult(resp.Content)

	// 持久化每条查询结果
	for _, q := range parsed.Queries {
		result := &model.GeoVerifyResult{
			Model:          resp.Model,
			Query:          q.Query,
			Response:       q.Response,
			BrandMentioned: q.BrandMentioned,
			MentionCount:   q.MentionCount,
			Sentiment:      classifyNegativeSentiment(q.IsNegative, q.NegativeScore),
			Position:       q.RiskLevel,
		}
		_ = s.verifyRepo.Create(result)
	}

	return negativeResultToMap(parsed, resp.Model), nil
}

// parseNegativeMonitorResult 解析负面监控结果
func parseNegativeMonitorResult(content string) *negativeMonitorResult {
	result := &negativeMonitorResult{}
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), result)
	return result
}

// classifyNegativeSentiment 根据负面标记和分数判定情感
func classifyNegativeSentiment(isNegative bool, score float64) string {
	if isNegative {
		return "negative"
	}
	if score > 0.5 {
		return "negative"
	}
	return "neutral"
}

// negativeResultToMap 将负面监控结果转为 map 返回
func negativeResultToMap(r *negativeMonitorResult, modelName string) map[string]any {
	queries := make([]map[string]any, 0, len(r.Queries))
	for _, q := range r.Queries {
		queries = append(queries, map[string]any{
			"query":             q.Query,
			"response":          q.Response,
			"brand_mentioned":   q.BrandMentioned,
			"mention_count":     q.MentionCount,
			"is_negative":       q.IsNegative,
			"negative_score":    q.NegativeScore,
			"negative_keywords": q.NegativeKeywords,
			"risk_level":        q.RiskLevel,
			"risk_description":  q.RiskDescription,
		})
	}

	summary := map[string]any{
		"total_queries":         r.Summary.TotalQueries,
		"high_risk_count":       r.Summary.HighRiskCount,
		"medium_risk_count":     r.Summary.MediumRiskCount,
		"low_risk_count":        r.Summary.LowRiskCount,
		"average_mention_count": r.Summary.AverageMentionCount,
		"alerts":                r.Summary.Alerts,
		"recommendations":       r.Summary.Recommendations,
	}

	return map[string]any{
		"model":   modelName,
		"queries": queries,
		"summary": summary,
	}
}

// GetVerifyResults 获取文章的验证结果列表
func (s *VerificationService) GetVerifyResults(ctx context.Context, articleID string) ([]*model.GeoVerifyResult, error) {
	return s.verifyRepo.GetByArticleID(articleID)
}

// recordAPICall 记录 API 调用
func (s *VerificationService) recordAPICall(providerName string, resp *llm.LLMResponse, purpose string) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	call := &model.GeoAPICall{
		Provider:     providerName,
		Model:        resp.Model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		Purpose:      purpose,
		Status:       "success",
	}
	_ = s.apiCallRepo.Create(call)
}
