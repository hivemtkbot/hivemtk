package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/llm"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// ContentService 内容服务
type ContentService struct {
	articleRepo      repository.GeoArticleRepository
	optimizationRepo repository.GeoOptimizationRepository
	apiCallRepo      repository.GeoAPICallRepository
	llmFactory       *llm.LLMFactory
}

// NewContentService 创建内容服务
func NewContentService(ar repository.GeoArticleRepository, or repository.GeoOptimizationRepository, acr repository.GeoAPICallRepository, f *llm.LLMFactory) *ContentService {
	return &ContentService{
		articleRepo:      ar,
		optimizationRepo: or,
		apiCallRepo:      acr,
		llmFactory:       f,
	}
}

// GenerateContent 生成内容
func (s *ContentService) GenerateContent(ctx context.Context, keyword, brandName string, advantages []string, providerName string, wordCount int, style string) (*model.GeoArticle, error) {
	provider, err := s.llmFactory.GetProvider(providerName)
	if err != nil {
		provider = s.llmFactory.GetDefaultProvider()
		if provider == nil {
			return nil, fmt.Errorf("未配置 LLM 提供商")
		}
	}

	advantagesStr := llm.AdvantagesToString(advantages)
	wordCountStr := "800"
	if wordCount > 0 {
		wordCountStr = fmt.Sprintf("%d", wordCount)
	}
	if style == "" {
		style = "专业"
	}

	prompt := llm.ContentGenerationPrompt(brandName, advantagesStr, keyword, wordCountStr, style)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   4000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("内容生成失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp, "content_generate", keyword)

	// 持久化文章
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
func (s *ContentService) OptimizeContent(ctx context.Context, articleID, content, brandName string, advantages []string, providerName string) (*model.GeoOptimization, error) {
	provider, err := s.llmFactory.GetProvider(providerName)
	if err != nil {
		provider = s.llmFactory.GetDefaultProvider()
		if provider == nil {
			return nil, fmt.Errorf("未配置 LLM 提供商")
		}
	}

	advantagesStr := llm.AdvantagesToString(advantages)
	prompt := llm.ContentOptimizePrompt(brandName, advantagesStr, content)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.6,
		MaxTokens:   4000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("内容优化失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp, "content_optimize", "")

	// 提取优化后内容（去除来源占位清单部分）
	optimizedContent := extractOptimizedContent(resp.Content)

	optimization := &model.GeoOptimization{
		ArticleID:        articleID,
		OriginalContent:   content,
		OptimizedContent: optimizedContent,
		Model:            resp.Model,
	}
	if err := s.optimizationRepo.Create(optimization); err != nil {
		return nil, fmt.Errorf("保存优化记录失败: %w", err)
	}

	return optimization, nil
}

// extractOptimizedContent 从 LLM 响应中提取优化后的内容（去除来源占位清单部分）
func extractOptimizedContent(response string) string {
	if idx := strings.Index(response, "【来源占位清单】"); idx != -1 {
		return strings.TrimSpace(response[:idx])
	}
	return strings.TrimSpace(response)
}

// ScoreContent 内容质量评分
func (s *ContentService) ScoreContent(ctx context.Context, content, brandName, keyword string) (map[string]any, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	prompt := llm.ContentScorePrompt(brandName, keyword, content)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("内容评分失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp, "content_score", keyword)

	// 解析评分结果
	scores := parseScoreResult(resp.Content)

	// 添加内容指标（基于规则计算）
	metrics := calculateContentMetrics(content, brandName)
	for k, v := range metrics {
		scores[k] = v
	}

	return scores, nil
}

// parseScoreResult 解析评分结果
func parseScoreResult(content string) map[string]any {
	result := map[string]any{
		"scores": map[string]any{
			"structure":     0,
			"brand_mention": 0,
			"authority":     0,
			"citations":     0,
			"total":         0,
		},
	}

	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}

	var scoreData map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &scoreData); err != nil {
		return result
	}

	if scores, ok := scoreData["scores"].(map[string]any); ok {
		result["scores"] = scores
	}
	if details, ok := scoreData["details"].(map[string]any); ok {
		result["details"] = details
	}
	if improvements, ok := scoreData["improvements"].([]any); ok {
		result["improvements"] = improvements
	}
	if strengths, ok := scoreData["strengths"].([]any); ok {
		result["strengths"] = strengths
	}

	return result
}

// calculateContentMetrics 计算内容指标（迁移自 content_metrics.py）
func calculateContentMetrics(content, brand string) map[string]any {
	metrics := map[string]any{}

	if content == "" {
		return metrics
	}

	// 统计品牌提及次数
	brandMentions := 0
	if brand != "" {
		brandMentions = strings.Count(strings.ToLower(content), strings.ToLower(brand))
	}

	// 统计结构化元素
	lines := strings.Split(content, "\n")
	headings := 0
	lists := 0
	faqPairs := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		runes := []rune(trimmed)
		if strings.HasPrefix(trimmed, "#") {
			headings++
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			lists++
		} else if len(runes) > 2 && runes[0] >= '0' && runes[0] <= '9' &&
			(runes[1] == '.' || runes[1] == '、') {
			lists++
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "q:") || strings.Contains(trimmed, "问：") {
			faqPairs++
		}
	}

	// 文本长度（去除空白）
	textLength := len(strings.Join(strings.Fields(content), ""))

	metrics["brand_mentions"] = brandMentions
	metrics["headings"] = headings
	metrics["lists"] = lists
	metrics["faq_pairs"] = faqPairs
	metrics["text_length"] = textLength

	// 信任密度（简化版：每 100 字的信任信号数）
	trustSignals := countTrustSignals(content)
	if textLength > 0 {
		metrics["trust_density"] = float64(trustSignals) / float64(textLength) * 100
	} else {
		metrics["trust_density"] = 0.0
	}

	return metrics
}

// countTrustSignals 统计信任信号数量（简化版）
func countTrustSignals(content string) int {
	count := 0
	trustPatterns := []string{
		"根据", "参考", "报告", "研究", "数据", "统计", "调查", "分析",
		"标准", "规范", "显示", "表明", "案例", "例如", "实际",
	}
	lower := strings.ToLower(content)
	for _, pattern := range trustPatterns {
		count += strings.Count(lower, strings.ToLower(pattern))
	}
	return count
}

// EnhanceEEAT E-E-A-T 强化
func (s *ContentService) EnhanceEEAT(ctx context.Context, content, brandName string, advantages []string) (string, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return "", fmt.Errorf("未配置 LLM 提供商")
	}

	advantagesStr := llm.AdvantagesToString(advantages)
	prompt := llm.EEATEnhancePrompt(brandName, advantagesStr, content)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.6,
		MaxTokens:   4000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return "", fmt.Errorf("E-E-A-T 强化失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp, "eeat_enhance", "")

	// 提取强化后内容
	return extractOptimizedContent(resp.Content), nil
}

// GenerateSchema 生成 Schema.org JSON-LD
func (s *ContentService) GenerateSchema(ctx context.Context, brandName, description, domain string) (string, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		// 无 LLM 时使用规则生成
		return generateSchemaByRule(brandName, description, domain), nil
	}

	prompt := llm.SchemaGeneratePrompt(brandName, description, domain)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		// LLM 失败时使用规则生成
		return generateSchemaByRule(brandName, description, domain), nil
	}

	s.recordAPICall(ctx, provider.Name(), resp, "schema_generate", "")

	// 提取 JSON
	schemaJSON := extractJSONArray(resp.Content)
	if schemaJSON == "" {
		schemaJSON = extractJSONObject(resp.Content)
	}
	if schemaJSON == "" {
		return generateSchemaByRule(brandName, description, domain), nil
	}

	// 格式化输出
	var schema any
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err == nil {
		formatted, _ := json.MarshalIndent(schema, "", "  ")
		return string(formatted), nil
	}

	return schemaJSON, nil
}

// generateSchemaByRule 基于规则生成 Schema.org JSON-LD（迁移自 schema_generator.py）
func generateSchemaByRule(brandName, description, domain string) string {
	schema := []map[string]any{
		{
			"@context":    "https://schema.org",
			"@type":       "Organization",
			"name":        brandName,
			"description": description,
			"url":         domain,
		},
		{
			"@context":            "https://schema.org",
			"@type":               "SoftwareApplication",
			"name":                brandName,
			"applicationCategory":  "BusinessApplication",
			"description":          description,
			"url":                 domain,
			"operatingSystem":      "Web",
			"publisher": map[string]any{
				"@type": "Organization",
				"name":  brandName,
			},
		},
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	return string(b)
}

// CheckUniqueness 内容独特性检测（迁移自 content_uniqueness.py）
func (s *ContentService) CheckUniqueness(ctx context.Context, contents []string) ([]map[string]any, error) {
	results := make([]map[string]any, 0)

	if len(contents) < 2 {
		results = append(results, map[string]any{
			"is_unique":       true,
			"message":         "内容数量不足，无需检查",
			"total_contents":  len(contents),
		})
		return results, nil
	}

	// 计算两两相似度
	highSimilarityPairs := make([]map[string]any, 0)
	totalSimilarity := 0.0
	pairCount := 0

	for i := 0; i < len(contents); i++ {
		for j := i + 1; j < len(contents); j++ {
			similarity := calculateSimilarity(contents[i], contents[j])
			totalSimilarity += similarity
			pairCount++

			if similarity > 0.7 {
				highSimilarityPairs = append(highSimilarityPairs, map[string]any{
					"content_index_1": i,
					"content_index_2": j,
					"similarity":      similarity,
				})
			}
		}
	}

	avgSimilarity := 0.0
	if pairCount > 0 {
		avgSimilarity = totalSimilarity / float64(pairCount)
	}
	uniquenessScore := (1.0 - avgSimilarity) * 100

	result := map[string]any{
		"is_unique":            len(highSimilarityPairs) == 0,
		"total_contents":       len(contents),
		"high_similarity_pairs": highSimilarityPairs,
		"avg_similarity":       avgSimilarity,
		"uniqueness_score":     uniquenessScore,
	}

	results = append(results, result)
	return results, nil
}

// calculateSimilarity 计算两段文本的相似度（Jaccard 相似度，简化版）
func calculateSimilarity(text1, text2 string) float64 {
	words1 := tokenize(text1)
	words2 := tokenize(text2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0
	}

	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}
	set2 := make(map[string]bool)
	for _, w := range words2 {
		set2[w] = true
	}

	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenize 分词（简化版）
func tokenize(text string) []string {
	// 移除标点符号
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// 按空格和标点分割
	words := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '.' ||
			r == '，' || r == '。' || r == '、' || r == '；' || r == '：'
	})

	// 过滤停用词和短词
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
func (s *ContentService) recordAPICall(ctx context.Context, providerName string, resp *llm.LLMResponse, operation, keyword string) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	call := &model.GeoAPICall{
		Provider:     providerName,
		Model:        resp.Model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		Purpose:      operation,
		Status:       "success",
	}
	_ = s.apiCallRepo.Create(call)
}
