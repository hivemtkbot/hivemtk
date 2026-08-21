package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// ContentService 内容服务
type ContentService struct {
	articleRepo      repository.GeoArticleRepository
	optimizationRepo repository.GeoOptimizationRepository
	apiCallRepo      repository.GeoAPICallRepository
	llm              *LLMAdapter
}

// NewContentService 创建内容服务
func NewContentService(ar repository.GeoArticleRepository, or repository.GeoOptimizationRepository, acr repository.GeoAPICallRepository, adapter *LLMAdapter) *ContentService {
	return &ContentService{
		articleRepo:      ar,
		optimizationRepo: or,
		apiCallRepo:      acr,
		llm:              adapter,
	}
}

// GenerateContent 生成内容
func (s *ContentService) GenerateContent(ctx context.Context, keyword, brandName string, advantages []string, wordCount int, style string) (*model.GeoArticle, error) {
	advantagesStr := AdvantagesToString(advantages)
	wordCountStr := "800"
	if wordCount > 0 {
		wordCountStr = fmt.Sprintf("%d", wordCount)
	}
	if style == "" {
		style = "专业"
	}
	prompt := ContentGenerationPrompt(brandName, advantagesStr, keyword, wordCountStr, style)

	resp, err := s.llm.Generate(ctx, "", prompt, 0.7, 4000)
	if err != nil {
		return nil, fmt.Errorf("内容生成失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "content_generate")

	article := &model.GeoArticle{
		Keyword:   keyword,
		BrandName: brandName,
		Content:   resp.Content,
		Model:     resp.Model,
		WordCount: wordCount,
	}
	if err := s.articleRepo.Create(article); err != nil {
		return nil, fmt.Errorf("保存文章失败: %w", err)
	}
	return article, nil
}

// OptimizeContent 优化内容
func (s *ContentService) OptimizeContent(ctx context.Context, articleID, content, brandName string, advantages []string) (*model.GeoOptimization, error) {
	advantagesStr := AdvantagesToString(advantages)
	prompt := ContentOptimizePrompt(brandName, advantagesStr, content)

	resp, err := s.llm.Generate(ctx, "", prompt, 0.6, 4000)
	if err != nil {
		return nil, fmt.Errorf("内容优化失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "content_optimize")

	optimization := &model.GeoOptimization{
		ArticleID:        articleID,
		OriginalContent:   content,
		OptimizedContent: extractOptimizedContent(resp.Content),
		Model:            resp.Model,
	}
	if err := s.optimizationRepo.Create(optimization); err != nil {
		return nil, fmt.Errorf("保存优化记录失败: %w", err)
	}
	return optimization, nil
}

func extractOptimizedContent(response string) string {
	if idx := strings.Index(response, "【来源占位清单】"); idx != -1 {
		return strings.TrimSpace(response[:idx])
	}
	return strings.TrimSpace(response)
}

// ScoreContent 内容质量评分
func (s *ContentService) ScoreContent(ctx context.Context, content, brandName, keyword string) (map[string]any, error) {
	prompt := ContentScorePrompt(brandName, keyword, content)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 2000)
	if err != nil {
		return nil, fmt.Errorf("内容评分失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "content_score")

	return parseScoreResult(resp.Content), nil
}

// parseScoreResult 解析评分结果
func parseScoreResult(content string) map[string]any {
	result := map[string]any{
		"total_score": 0.0,
		"dimensions":  map[string]float64{},
		"summary":     content,
	}
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return result
	}
	if score, ok := parsed["total_score"].(float64); ok {
		result["total_score"] = score
	}
	if dims, ok := parsed["dimensions"].(map[string]any); ok {
		dimMap := map[string]float64{}
		for k, v := range dims {
			if fv, ok := v.(float64); ok {
				dimMap[k] = fv
			}
		}
		result["dimensions"] = dimMap
	}
	if summary, ok := parsed["summary"].(string); ok {
		result["summary"] = summary
	}
	return result
}

// EnhanceEEAT E-E-A-T 增强
func (s *ContentService) EnhanceEEAT(ctx context.Context, content, brandName string, advantages []string) (map[string]any, error) {
	advantagesStr := AdvantagesToString(advantages)
	prompt := EEATEnhancePrompt(brandName, advantagesStr, content)

	resp, err := s.llm.Generate(ctx, "", prompt, 0.5, 4000)
	if err != nil {
		return nil, fmt.Errorf("E-E-A-T 增强失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "eeat_enhance")

	return map[string]any{
		"original":  content,
		"enhanced":  resp.Content,
		"provider":  resp.Provider,
		"model":     resp.Model,
	}, nil
}

// GenerateSchema 生成 JSON-LD Schema
func (s *ContentService) GenerateSchema(ctx context.Context, brandName, description, schemaType string) (map[string]any, error) {
	prompt := SchemaGeneratePrompt(brandName, description, schemaType)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 2000)
	if err != nil {
		return nil, fmt.Errorf("Schema 生成失败: %w", err)
	}
	s.recordAPICall(ctx, resp, "schema_generate")

	var schema map[string]any
	jsonStr := extractJSONObject(resp.Content)
	if jsonStr != "" {
		_ = json.Unmarshal([]byte(jsonStr), &schema)
	}
	if schema == nil {
		schema = map[string]any{
			"@context": "https://schema.org",
			"@type":    schemaType,
			"name":     brandName,
			"description": description,
		}
	}
	schema["provider"] = resp.Provider
	schema["model"] = resp.Model
	return schema, nil
}

// CheckUniqueness 内容独特性检测
func (s *ContentService) CheckUniqueness(ctx context.Context, content string) (map[string]any, error) {
	words := tokenize(content)
	uniqueWords := make(map[string]bool)
	for _, w := range words {
		uniqueWords[w] = true
	}
	total := len(words)
	unique := len(uniqueWords)
	uniquenessRatio := 0.0
	if total > 0 {
		uniquenessRatio = float64(unique) / float64(total)
	}
	return map[string]any{
		"total_words":     total,
		"unique_words":    unique,
		"uniqueness_ratio": uniquenessRatio,
		"is_unique":       uniquenessRatio > 0.6,
	}, nil
}

func tokenize(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	words := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '.' ||
			r == '，' || r == '。' || r == '、' || r == '；' || r == '：'
	})
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true,
		"和": true, "就": true, "不": true, "都": true, "一": true,
	}
	result := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 1 && !stopWords[w] {
			result = append(result, strings.ToLower(w))
		}
	}
	return result
}

// GetArticleList 获取文章列表
func (s *ContentService) GetArticleList(ctx context.Context, page, limit int) ([]*model.GeoArticle, int64, error) {
	return s.articleRepo.GetList("", "", page, limit)
}

// GetArticleByID 根据 ID 获取文章
func (s *ContentService) GetArticleByID(ctx context.Context, id string) (*model.GeoArticle, error) {
	return s.articleRepo.GetByID(id)
}

// recordAPICall 记录 API 调用
func (s *ContentService) recordAPICall(ctx context.Context, resp *LLMResult, operation string) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	call := &model.GeoAPICall{
		Provider: resp.Provider,
		Model:    resp.Model,
		Purpose:  operation,
		Status:   "success",
	}
	_ = s.apiCallRepo.Create(call)
}
