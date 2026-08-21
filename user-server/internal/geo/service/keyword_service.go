package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"hivemtk-user/internal/geo/llm"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// KeywordService 关键词服务
type KeywordService struct {
	keywordRepo repository.GeoKeywordRepository
	apiCallRepo repository.GeoAPICallRepository
	llmFactory  *llm.LLMFactory
}

// NewKeywordService 创建关键词服务
func NewKeywordService(repo repository.GeoKeywordRepository, acr repository.GeoAPICallRepository, factory *llm.LLMFactory) *KeywordService {
	return &KeywordService{
		keywordRepo: repo,
		apiCallRepo: acr,
		llmFactory:  factory,
	}
}

// minedKeyword LLM 返回的挖掘关键词结构
type minedKeyword struct {
	Keyword        string `json:"keyword"`
	Category       string `json:"category"`
	Intent         string `json:"intent"`
	EstimatedValue int    `json:"estimated_value"`
}

// MineKeywords 挖掘关键词（结合关键词组合工具 + LLM 精炼）
// mode: "industry"=行业挖掘, "longtail"=长尾词挖掘
func (s *KeywordService) MineKeywords(ctx context.Context, seedWords []string, mode string, brandName string, advantages []string) ([]*model.GeoKeyword, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	advantagesStr := llm.AdvantagesToString(advantages)
	seedWordsJSON := llm.KeywordsToJSON(seedWords)

	// 第一步：使用关键词组合工具生成基础关键词
	baseKeywords := generateKeywordCombinations(seedWords)

	// 第二步：LLM 挖掘 + 润色
	var prompt string
	if mode == "longtail" {
		prompt = llm.KeywordPolishPrompt(brandName, llm.KeywordsToJSON(baseKeywords))
	} else {
		prompt = llm.KeywordMiningPrompt(brandName, advantagesStr, seedWordsJSON)
	}

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   2000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("关键词挖掘失败: %w", err)
	}

	// 记录 API 调用
	s.recordAPICall(ctx, provider.Name(), resp)

	// 解析 LLM 响应
	keywords := s.parseMinedKeywords(resp.Content, baseKeywords)

	// 持久化到数据库
	result := make([]*model.GeoKeyword, 0, len(keywords))
	for _, kw := range keywords {
		geoKW := &model.GeoKeyword{
			Keyword:      kw.Keyword,
			Category:      kw.Category,
			Intent:        kw.Intent,
			Source:        mode,
			SearchVolume: kw.EstimatedValue,
		}
		if err := s.keywordRepo.Create(geoKW); err != nil {
			continue
		}
		result = append(result, geoKW)
	}

	return result, nil
}

// parseMinedKeywords 解析 LLM 返回的关键词挖掘结果
func (s *KeywordService) parseMinedKeywords(content string, baseKeywords []string) []*minedKeyword {
	result := make([]*minedKeyword, 0)

	// 尝试解析 JSON 数组
	jsonStr := extractJSONArray(content)
	if jsonStr != "" {
		var mined []minedKeyword
		if err := json.Unmarshal([]byte(jsonStr), &mined); err == nil && len(mined) > 0 {
			for i := range mined {
				if mined[i].Keyword != "" {
					result = append(result, &mined[i])
				}
			}
			return result
		}
	}

	// 解析失败，使用基础关键词
	for _, kw := range baseKeywords {
		result = append(result, &minedKeyword{
			Keyword:        kw,
			Category:       "其他",
			Intent:         "未知",
			EstimatedValue: 5,
		})
	}
	return result
}

// generateKeywordCombinations 关键词组合工具（迁移自 keyword_tool.py）
// 基于词库的组合模式生成关键词
func generateKeywordCombinations(seedWords []string) []string {
	// 默认词库
	wordbanks := map[string][]string{
		"A前缀1": {"行业上", "市场上", "市面上", "目前", "国内", "市场"},
		"B前缀2": {"口碑好的", "比较好的", "靠谱的", "有实力的", "可靠的", "诚信的", "正规的", "专业的", "热门的", "知名的"},
		"C主词":  {"软件", "管理系统", "工具"},
		"D通义词": {"品牌", "公司", "工厂", "厂商", "生产厂家", "供应商"},
		"E推荐词": {"推荐", "排行", "推荐榜", "排行榜", "推荐榜单", "推荐排行", "推荐排行榜", "口碑排行"},
		"F疑问词": {"哪家好", "哪家强", "哪家靠谱", "哪家权威", "哪个好", "有哪些", "找哪家", "选哪家", "为什么"},
	}

	// 如果有种子词，加入 C 主词库
	if len(seedWords) > 0 {
		wordbanks["C主词"] = append(wordbanks["C主词"], seedWords...)
	}

	// 组合模式（字母对应词库首字母）
	patterns := [][]string{
		{"C", "D"},
		{"A", "C", "D"},
		{"B", "C", "D"},
		{"A", "B", "C", "D"},
		{"C", "D", "E"},
		{"C", "D", "F"},
		{"A", "C", "D", "E"},
		{"B", "C", "D", "E"},
		{"A", "B", "C", "D", "E"},
		{"A", "B", "C", "D", "F"},
	}

	// 模式字母到词库 key 的映射
	patternToBank := make(map[string]string)
	for bankKey := range wordbanks {
		if len(bankKey) > 0 {
			patternToBank[string(bankKey[0])] = bankKey
		}
	}

	seen := make(map[string]bool)
	allKeywords := make([]string, 0)
	maxResults := 100

	for _, pattern := range patterns {
		requiredBanks := make([]string, 0)
		for _, letter := range pattern {
			if bankKey, ok := patternToBank[letter]; ok {
				if _, exists := wordbanks[bankKey]; exists {
					requiredBanks = append(requiredBanks, bankKey)
				}
			}
		}

		if len(requiredBanks) == 0 {
			continue
		}

		// 生成笛卡尔积
		wordLists := make([][]string, 0, len(requiredBanks))
		for _, bank := range requiredBanks {
			wordLists = append(wordLists, wordbanks[bank])
		}

		for _, combo := range cartesianProduct(wordLists) {
			keyword := strings.Join(combo, "")
			kwLower := strings.ToLower(keyword)
			if seen[kwLower] {
				continue
			}
			seen[kwLower] = true
			allKeywords = append(allKeywords, keyword)
			if len(allKeywords) >= maxResults {
				return allKeywords
			}
		}
	}

	return allKeywords
}

// cartesianProduct 计算多个列表的笛卡尔积
func cartesianProduct(lists [][]string) [][]string {
	if len(lists) == 0 {
		return nil
	}
	if len(lists) == 1 {
		result := make([][]string, 0, len(lists[0]))
		for _, item := range lists[0] {
			result = append(result, []string{item})
		}
		return result
	}

	sub := cartesianProduct(lists[1:])
	result := make([][]string, 0, len(lists[0])*len(sub))
	for _, item := range lists[0] {
		for _, s := range sub {
			combined := make([]string, 0, len(s)+1)
			combined = append(combined, item)
			combined = append(combined, s...)
			result = append(result, combined)
		}
	}
	return result
}

// expandedKeywordResult 语义扩展结果
type expandedKeywordResult struct {
	ExpandedKeywords []string `json:"expanded_keywords"`
}

// SemanticExpand 语义足迹扩展
func (s *KeywordService) SemanticExpand(ctx context.Context, keywords []string, brandName string) ([]*model.GeoKeyword, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	if len(keywords) == 0 {
		return nil, nil
	}

	// 限制输入关键词数量
	keywordsToExpand := keywords
	if len(keywordsToExpand) > 50 {
		keywordsToExpand = keywordsToExpand[:50]
	}

	keywordsJSON := llm.KeywordsToJSON(keywordsToExpand)
	prompt := llm.SemanticExpandPrompt(brandName, keywordsJSON)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.8,
		MaxTokens:   2000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("语义扩展失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp)

	// 解析结果
	expanded := s.parseExpandedKeywords(resp.Content, keywordsToExpand)

	// 持久化
	result := make([]*model.GeoKeyword, 0, len(expanded))
	originalSet := make(map[string]bool)
	for _, kw := range keywordsToExpand {
		originalSet[strings.ToLower(kw)] = true
	}

	for _, kw := range expanded {
		if originalSet[strings.ToLower(kw)] {
			continue
		}
		geoKW := &model.GeoKeyword{
			Keyword:  kw,
			Category: "扩展",
			Intent:   "语义扩展",
			Source:   "semantic_expand",
		}
		if err := s.keywordRepo.Create(geoKW); err != nil {
			continue
		}
		result = append(result, geoKW)
	}

	return result, nil
}

// parseExpandedKeywords 解析语义扩展结果
func (s *KeywordService) parseExpandedKeywords(content string, originalKeywords []string) []string {
	// 尝试提取 JSON 对象
	jsonStr := extractJSONObject(content)
	if jsonStr != "" {
		var result expandedKeywordResult
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && len(result.ExpandedKeywords) > 0 {
			return deduplicateKeywords(result.ExpandedKeywords, originalKeywords)
		}
	}

	// 尝试提取 JSON 数组
	jsonArrStr := extractJSONArray(content)
	if jsonArrStr != "" {
		var keywords []string
		if err := json.Unmarshal([]byte(jsonArrStr), &keywords); err == nil && len(keywords) > 0 {
			return deduplicateKeywords(keywords, originalKeywords)
		}
	}

	return nil
}

// deduplicateKeywords 去重（与原始关键词比较）
func deduplicateKeywords(expanded []string, original []string) []string {
	seen := make(map[string]bool)
	for _, kw := range original {
		seen[strings.ToLower(kw)] = true
	}

	result := make([]string, 0, len(expanded))
	for _, kw := range expanded {
		kw = strings.TrimSpace(kw)
		if len(kw) < 3 {
			continue
		}
		lower := strings.ToLower(kw)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		result = append(result, kw)
	}
	return result
}

// clusterResult 话题聚类结果
type clusterResult struct {
	Clusters []struct {
		ID          int      `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
		Priority    string   `json:"priority"`
	} `json:"clusters"`
}

// TopicCluster 话题聚类
func (s *KeywordService) TopicCluster(ctx context.Context, keywords []string, brandName string) (map[string][]string, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	if len(keywords) == 0 {
		return map[string][]string{}, nil
	}

	// 限制关键词数量
	keywordsToCluster := keywords
	if len(keywordsToCluster) > 100 {
		keywordsToCluster = keywordsToCluster[:100]
	}

	keywordsJSON := llm.KeywordsToJSON(keywordsToCluster)
	prompt := llm.TopicClusterPrompt(brandName, keywordsJSON)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   3000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("话题聚类失败: %w", err)
	}

	s.recordAPICall(ctx, provider.Name(), resp)

	// 解析结果
	return s.parseClusterResult(resp.Content, keywordsToCluster), nil
}

// parseClusterResult 解析聚类结果
func (s *KeywordService) parseClusterResult(content string, originalKeywords []string) map[string][]string {
	clusters := make(map[string][]string)

	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		// 规则聚类兜底
		return ruleBasedCluster(originalKeywords)
	}

	var result clusterResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil || len(result.Clusters) == 0 {
		return ruleBasedCluster(originalKeywords)
	}

	for _, c := range result.Clusters {
		if c.Name != "" && len(c.Keywords) > 0 {
			clusters[c.Name] = c.Keywords
		}
	}

	if len(clusters) == 0 {
		return ruleBasedCluster(originalKeywords)
	}
	return clusters
}

// ruleBasedCluster 基于规则的简单聚类（备用方案）
func ruleBasedCluster(keywords []string) map[string][]string {
	clusters := make(map[string][]string)
	if len(keywords) == 0 {
		return clusters
	}

	// 按关键词类别分组
	for _, kw := range keywords {
		category := "其他"
		if strings.Contains(kw, "对比") || strings.Contains(kw, "比较") {
			category = "对比类"
		} else if strings.Contains(kw, "推荐") || strings.Contains(kw, "排行") {
			category = "推荐类"
		} else if strings.Contains(kw, "哪家") || strings.Contains(kw, "哪个") {
			category = "疑问类"
		} else if strings.Contains(kw, "教程") || strings.Contains(kw, "如何") {
			category = "教程类"
		}
		clusters[category] = append(clusters[category], kw)
	}
	return clusters
}

// GetKeywordList 获取关键词列表（分页）
func (s *KeywordService) GetKeywordList(ctx context.Context, page, limit int, search, source string) ([]*model.GeoKeyword, int64, error) {
	return s.keywordRepo.GetList(search, "", source, "", "", page, limit)
}

// DeleteKeyword 删除关键词
func (s *KeywordService) DeleteKeyword(ctx context.Context, id string) error {
	return s.keywordRepo.Delete(id)
}

// recordAPICall 记录 API 调用
func (s *KeywordService) recordAPICall(ctx context.Context, providerName string, resp *llm.LLMResponse) {
	if s.apiCallRepo == nil || resp == nil {
		return
	}
	call := &model.GeoAPICall{
		Provider:     providerName,
		Model:        resp.Model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		Purpose:      "keyword_mining",
		Status:       "success",
	}
	_ = s.apiCallRepo.Create(call)
}

// --- JSON 提取辅助函数 ---

// extractJSONArray 从文本中提取 JSON 数组字符串
var jsonArrayRegex = regexp.MustCompile(`\[[\s\S]*\]`)

func extractJSONArray(s string) string {
	match := jsonArrayRegex.FindString(s)
	return match
}

// extractJSONObject 从文本中提取 JSON 对象字符串
var jsonObjectRegex = regexp.MustCompile(`\{[\s\S]*\}`)

func extractJSONObject(s string) string {
	match := jsonObjectRegex.FindString(s)
	return match
}
