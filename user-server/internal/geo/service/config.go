package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"

	"gorm.io/gorm"
)

// ConfigService GEO 配置服务
type ConfigService struct {
	configRepo repository.GeoConfigRepository
	llm        *LLMAdapter
}

// NewConfigService 创建 GEO 配置服务
func NewConfigService(cr repository.GeoConfigRepository, adapter *LLMAdapter) *ConfigService {
	return &ConfigService{configRepo: cr, llm: adapter}
}

// GetConfig 获取 GEO 配置
// 首次使用（库中无配置）时返回默认配置而非 404，保证前端初始化体验
func (s *ConfigService) GetConfig(ctx context.Context) (*model.GeoConfig, error) {
	config, err := s.configRepo.Get()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.GeoConfig{Language: "zh"}, nil
		}
		return nil, err
	}
	return config, nil
}

// UpdateConfig 更新 GEO 配置（合并更新，保留未提供的字段）
func (s *ConfigService) UpdateConfig(ctx context.Context, brand, brandDescription, advantages string, competitors []string, domain, defaultModel string, verifyModels []string) error {
	existing, err := s.configRepo.Get()
	if err != nil {
		existing = &model.GeoConfig{Language: "zh"}
	}
	if brand != "" {
		existing.BrandName = brand
	}
	if brandDescription != "" {
		existing.BrandDescription = brandDescription
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
	if defaultModel != "" {
		existing.DefaultModel = defaultModel
	}
	if len(verifyModels) > 0 {
		existing.VerifyModels = strings.Join(verifyModels, "、")
	}
	if existing.Language == "" {
		existing.Language = "zh"
	}
	return s.configRepo.Update(existing)
}

// configOptimizeResult 配置优化结果结构
type configOptimizeResult struct {
	Summary             string           `json:"summary"`
	Suggestions         map[string]any   `json:"suggestions"`
	RecommendedVersions []map[string]any `json:"recommended_versions"`
	ExpectedEffects     map[string]any   `json:"expected_effects"`
}

// OptimizeConfig 优化 GEO 配置
func (s *ConfigService) OptimizeConfig(ctx context.Context, brandName, advantages string, competitors []string) (map[string]any, error) {
	competitorsStr := strings.Join(competitors, "、")
	prompt := ConfigOptimizePrompt(brandName, advantages, competitorsStr)

	resp, err := s.llm.GenerateJSON(ctx, "", prompt, 3000)
	if err != nil {
		return nil, fmt.Errorf("配置优化失败: %w", err)
	}

	parsed := &configOptimizeResult{}
	jsonStr := extractJSONObject(resp.Content)
	if jsonStr != "" {
		_ = json.Unmarshal([]byte(jsonStr), parsed)
	}
	return configOptimizeToMap(parsed, resp.Provider, resp.Model), nil
}

// configOptimizeToMap 将配置优化结果转为 map
func configOptimizeToMap(r *configOptimizeResult, provider, model string) map[string]any {
	return map[string]any{
		"provider":             provider,
		"model":                model,
		"summary":              r.Summary,
		"suggestions":          r.Suggestions,
		"recommended_versions": r.RecommendedVersions,
		"expected_effects":     r.ExpectedEffects,
	}
}
