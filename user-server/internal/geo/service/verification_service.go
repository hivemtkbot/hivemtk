package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/dto"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// VerificationService AI 搜索验证服务
type VerificationService struct {
	verifyRepo  repository.GeoVerifyResultRepository
	apiCallRepo repository.GeoAPICallRepository
	llm         *LLMAdapter
}

// NewVerificationService 创建 AI 搜索验证服务
func NewVerificationService(vr repository.GeoVerifyResultRepository, acr repository.GeoAPICallRepository, adapter *LLMAdapter) *VerificationService {
	return &VerificationService{
		verifyRepo:  vr,
		apiCallRepo: acr,
		llm:         adapter,
	}
}

// verifyLLMResponse LLM 验证响应结构
type verifyLLMResponse struct {
	Response             string   `json:"response"`
	BrandMentioned      bool     `json:"brand_mentioned"`
	MentionCount        int      `json:"mention_count"`
	MentionPositions    []string `json:"mention_positions"`
	Sentiment           string   `json:"sentiment"`
	CompetitorsMentioned []string `json:"competitors_mentioned"`
}

// VerifyArticle 执行 AI 搜索验证
func (s *VerificationService) VerifyArticle(ctx context.Context, req dto.VerifyRequest) (*model.GeoVerifyResult, error) {
	prompt := VerifySearchPrompt(req.BrandName, req.Query)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 2000)
	if err != nil {
		return nil, fmt.Errorf("AI 搜索验证失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "verify_search")

	parsed := parseVerifyResponse(resp.Content)
	result := &model.GeoVerifyResult{
		ArticleID:      req.ArticleID,
		BrandName:      req.BrandName,
		Model:          resp.Model,
		Query:          req.Query,
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
		Query            string   `json:"query"`
		Response         string   `json:"response"`
		BrandMentioned   bool     `json:"brand_mentioned"`
		MentionCount     int      `json:"mention_count"`
		IsNegative       bool     `json:"is_negative"`
		NegativeScore    float64  `json:"negative_score"`
		NegativeKeywords []string `json:"negative_keywords"`
		RiskLevel        string   `json:"risk_level"`
		RiskDescription  string   `json:"risk_description"`
	} `json:"queries"`
	Summary struct {
		TotalQueries        int      `json:"total_queries"`
		HighRiskCount       int      `json:"high_risk_count"`
		MediumRiskCount     int      `json:"medium_risk_count"`
		LowRiskCount        int      `json:"low_risk_count"`
		AverageMentionCount float64  `json:"average_mention_count"`
		Alerts              []string `json:"alerts"`
		Recommendations     []string `json:"recommendations"`
	} `json:"summary"`
}

// MonitorNegative 负面提及监控
func (s *VerificationService) MonitorNegative(ctx context.Context, brandName string) (map[string]any, error) {
	prompt := NegativeMonitorPrompt(brandName)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 3000)
	if err != nil {
		return nil, fmt.Errorf("负面监控失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "negative_monitor")

	parsed := parseNegativeMonitorResult(resp.Content)
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

func parseNegativeMonitorResult(content string) *negativeMonitorResult {
	result := &negativeMonitorResult{}
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), result)
	return result
}

func classifyNegativeSentiment(isNegative bool, score float64) string {
	if isNegative || score > 0.5 {
		return "negative"
	}
	return "neutral"
}

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
	return map[string]any{
		"model":   modelName,
		"queries": queries,
		"summary": map[string]any{
			"total_queries":         r.Summary.TotalQueries,
			"high_risk_count":       r.Summary.HighRiskCount,
			"medium_risk_count":     r.Summary.MediumRiskCount,
			"low_risk_count":        r.Summary.LowRiskCount,
			"average_mention_count": r.Summary.AverageMentionCount,
			"alerts":                r.Summary.Alerts,
			"recommendations":       r.Summary.Recommendations,
		},
	}
}

// GetVerifyResults 获取文章的验证结果列表
func (s *VerificationService) GetVerifyResults(ctx context.Context, articleID string) ([]*model.GeoVerifyResult, error) {
	return s.verifyRepo.GetByArticleID(articleID)
}

// recordAPICall 记录 API 调用
func (s *VerificationService) recordAPICall(ctx context.Context, resp *LLMResult, purpose string) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	call := &model.GeoAPICall{
		Provider: resp.Provider,
		Model:    resp.Model,
		Purpose:  purpose,
		Status:   "success",
	}
	_ = s.apiCallRepo.Create(call)
}
