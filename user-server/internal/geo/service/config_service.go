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

// ConfigService GEO 配置服务
type ConfigService struct {
	configRepo repository.GeoConfigRepository
	llmFactory *llm.LLMFactory
}

// NewConfigService 创建 GEO 配置服务
func NewConfigService(cr repository.GeoConfigRepository, f *llm.LLMFactory) *ConfigService {
	return &ConfigService{
		configRepo: cr,
		llmFactory: f,
	}
}

// GetConfig 获取 GEO 配置
func (s *ConfigService) GetConfig(ctx context.Context) (*model.GeoConfig, error) {
	return s.configRepo.Get()
}

// UpdateConfig 更新 GEO 配置（合并更新，保留未提供的字段）
func (s *ConfigService) UpdateConfig(ctx context.Context, brand, advantages string, competitors []string, domain string) error {
	existing, err := s.configRepo.Get()
	if err != nil {
		// 配置不存在时初始化默认结构
		existing = &model.GeoConfig{
			Language: "zh",
		}
	}

	if brand != "" {
		existing.BrandName = brand
	}
	if advantages != "" {
		existing.Advantages = advantages
	}
	if len(competitors) > 0 {
		existing.Competitors = strings.Join(competitors, "、")
	}
	if domain != "" {
		existing.Domain = domain
	}
	if existing.Language == "" {
		existing.Language = "zh"
	}

	return s.configRepo.Update(existing)
}

// configOptimizeResult 配置优化结果结构
type configOptimizeResult struct {
	Summary             string             `json:"summary"`
	Suggestions         map[string]any     `json:"suggestions"`
	RecommendedVersions []map[string]any   `json:"recommended_versions"`
	ExpectedEffects     map[string]any     `json:"expected_effects"`
}

// OptimizeConfig 优化 GEO 配置（迁移自 config_optimizer.py）
func (s *ConfigService) OptimizeConfig(ctx context.Context, brandName, advantages string, competitors []string) (map[string]any, error) {
	provider := s.llmFactory.GetDefaultProvider()
	if provider == nil {
		return nil, fmt.Errorf("未配置 LLM 提供商")
	}

	competitorsStr := strings.Join(competitors, "、")
	prompt := llm.ConfigOptimizePrompt(brandName, advantages, competitorsStr)

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   3000,
	}

	resp, err := provider.Chat(req)
	if err != nil {
		return nil, fmt.Errorf("配置优化失败: %w", err)
	}

	parsed := parseConfigOptimizeResult(resp.Content)
	return configOptimizeToMap(parsed, provider.Name(), resp.Model), nil
}

// parseConfigOptimizeResult 解析配置优化结果
func parseConfigOptimizeResult(content string) *configOptimizeResult {
	result := &configOptimizeResult{}
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return result
	}
	_ = json.Unmarshal([]byte(jsonStr), result)
	return result
}

// configOptimizeToMap 将配置优化结果转为 map 返回
func configOptimizeToMap(r *configOptimizeResult, providerName, modelName string) map[string]any {
	return map[string]any{
		"provider":             providerName,
		"model":                modelName,
		"summary":              r.Summary,
		"suggestions":          r.Suggestions,
		"recommended_versions": r.RecommendedVersions,
		"expected_effects":     r.ExpectedEffects,
	}
}
