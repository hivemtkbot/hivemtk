package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"hivemtk-user/internal/geo/dto"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// KeywordEnhanceService 关键词数据增强服务
// 从历史验证数据中提取高价值关键词，反哺关键词生成
type KeywordEnhanceService struct {
	keywordRepo repository.GeoKeywordRepository
	verifyRepo  repository.GeoVerifyResultRepository
	llm         *LLMAdapter
}

// NewKeywordEnhanceService 创建关键词数据增强服务
func NewKeywordEnhanceService(kr repository.GeoKeywordRepository, vr repository.GeoVerifyResultRepository, adapter *LLMAdapter) *KeywordEnhanceService {
	return &KeywordEnhanceService{
		keywordRepo: kr,
		verifyRepo:  vr,
		llm:         adapter,
	}
}

// AnalyzeHistoricalPerformance 分析历史验证数据中的关键词表现
func (s *KeywordEnhanceService) AnalyzeHistoricalPerformance(ctx context.Context, brandName string) (*dto.KeywordEnhanceResponse, error) {
	// 获取该品牌所有验证结果
	results, err := s.verifyRepo.GetByBrandName(brandName)
	if err != nil || len(results) == 0 {
		return &dto.KeywordEnhanceResponse{
			HasData: false,
			Message: "暂无历史验证数据",
		}, nil
	}

	// 按关键词聚合表现数据
	perfMap := map[string]*dto.KeywordPerformance{}
	for _, r := range results {
		query := r.Query
		if query == "" {
			continue
		}
		if _, ok := perfMap[query]; !ok {
			perfMap[query] = &dto.KeywordPerformance{Keyword: query}
		}
		p := perfMap[query]
		p.QueryCount++
		if r.BrandMentioned {
			p.MentionCount++
		}
	}

	// 计算提及率和价值分
	perfs := make([]*dto.KeywordPerformance, 0, len(perfMap))
	for _, p := range perfMap {
		if p.QueryCount > 0 {
			p.MentionRate = float64(p.MentionCount) / float64(p.QueryCount)
		}
		// 价值分 = 提及率 * log(查询次数+1) * 10
		p.HighValueScore = p.MentionRate * float64(p.QueryCount) * 10
		perfs = append(perfs, p)
	}

	// 按价值分排序
	sort.Slice(perfs, func(i, j int) bool {
		return perfs[i].HighValueScore > perfs[j].HighValueScore
	})

	// 提取高价值关键词（Top 20）
	highValue := make([]*dto.KeywordPerformance, 0, 20)
	for i, p := range perfs {
		if i >= 20 {
			break
		}
		if p.MentionRate > 0.3 {
			highValue = append(highValue, p)
		}
	}

	// 生成增强建议
	suggestions, err := s.generateEnhancementSuggestions(ctx, brandName, highValue)

	return &dto.KeywordEnhanceResponse{
		HasData:            true,
		TotalKeywords:      len(perfMap),
		HighValueKeywords:  highValue,
		Suggestions:        suggestions,
		IntentDistribution: s.calculateIntentDistribution(perfs),
	}, nil
}

// generateEnhancementSuggestions 基于高价值关键词生成增强建议
func (s *KeywordEnhanceService) generateEnhancementSuggestions(ctx context.Context, brandName string, highValue []*dto.KeywordPerformance) ([]string, error) {
	if len(highValue) == 0 || s.llm == nil {
		return []string{"暂无足够数据生成建议"}, nil
	}

	// 构建高价值关键词摘要
	kwSummary := make([]string, 0, len(highValue))
	for _, p := range highValue {
		kwSummary = append(kwSummary, fmt.Sprintf("%s (提及率:%.0f%%)", p.Keyword, p.MentionRate*100))
	}

	prompt := fmt.Sprintf(`基于以下高价值关键词表现数据，为品牌"%s"生成 5 条关键词策略建议。

高价值关键词（按价值排序）：
%s

要求：
1. 每条建议具体可行，针对关键词策略优化
2. 结合提及率数据给出针对性建议
3. 输出 JSON 数组格式：["建议1", "建议2", ...]`, brandName, strings.Join(kwSummary, "\n"))

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 1000)
	if err != nil {
		return []string{"生成建议失败: " + err.Error()}, nil
	}

	var suggestions []string
	jsonStr := extractJSONArray(resp.Content)
	if jsonStr != "" {
		_ = json.Unmarshal([]byte(jsonStr), &suggestions)
	}
	if len(suggestions) == 0 {
		suggestions = []string{"基于高价值关键词持续优化内容覆盖"}
	}
	return suggestions, nil
}

// calculateIntentDistribution 计算意图分布（简化版）
func (s *KeywordEnhanceService) calculateIntentDistribution(perfs []*dto.KeywordPerformance) map[string]int {
	distribution := map[string]int{}
	for _, p := range perfs {
		intent := classifyIntent(p.Keyword)
		distribution[intent]++
	}
	return distribution
}

func classifyIntent(keyword string) string {
	switch {
	case strings.Contains(keyword, "哪家") || strings.Contains(keyword, "哪个") || strings.Contains(keyword, "哪个好"):
		return "疑问"
	case strings.Contains(keyword, "对比") || strings.Contains(keyword, "比较") || strings.Contains("keyword", "vs"):
		return "对比"
	case strings.Contains(keyword, "推荐") || strings.Contains(keyword, "排行"):
		return "推荐"
	case strings.Contains(keyword, "教程") || strings.Contains(keyword, "如何") || strings.Contains(keyword, "怎么"):
		return "教程"
	case strings.Contains(keyword, "评测") || strings.Contains(keyword, "体验"):
		return "评测"
	default:
		return "信息"
	}
}

// EnhanceKeywordWithData 用历史数据增强现有关键词
func (s *KeywordEnhanceService) EnhanceKeywordWithData(ctx context.Context, keywords []string, brandName string) ([]*model.GeoKeyword, error) {
	results, err := s.verifyRepo.GetByBrandName(brandName)
	if err != nil || len(results) == 0 {
		return nil, nil
	}

	// 构建查询->提及率映射
	mentionMap := map[string]float64{}
	queryCount := map[string]int{}
	for _, r := range results {
		if r.Query == "" {
			continue
		}
		queryCount[r.Query]++
		if r.BrandMentioned {
			mentionMap[r.Query]++
		}
	}
	for q := range mentionMap {
		mentionMap[q] = mentionMap[q] / float64(queryCount[q])
	}

	// 为现有关键词添加数据增强标记
	enhanced := make([]*model.GeoKeyword, 0, len(keywords))
	for _, kw := range keywords {
		geoKW := &model.GeoKeyword{
			Keyword:  kw,
			Source:   "data_enhanced",
			Category: "数据增强",
		}
		if rate, ok := mentionMap[kw]; ok {
			geoKW.SearchVolume = int(rate * 100)
		}
		if err := s.keywordRepo.Create(geoKW); err != nil {
			continue
		}
		enhanced = append(enhanced, geoKW)
	}
	return enhanced, nil
}
