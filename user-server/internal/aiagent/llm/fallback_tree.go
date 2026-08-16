package llm


import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"hivemtk-user/internal/pkg/featureflag"
	"hivemtk-user/internal/pkg/utils/logger"
)

// FallbackLevel 降级级别
type FallbackLevel int

const (
	LevelPrimary FallbackLevel = iota + 1
	LevelSecondary
	LevelCache
	LevelTemplate
)

// String 降级级别名称
func (l FallbackLevel) String() string {
	switch l {
	case LevelPrimary:
		return "primary_7b"
	case LevelSecondary:
		return "secondary_3b"
	case LevelCache:
		return "cache"
	case LevelTemplate:
		return "template"
	}
	return "unknown"
}

// FallbackDecision 降级决策
type FallbackDecision struct {
	Level       FallbackLevel 
	FromLevel   FallbackLevel 
	Reason      string        
	Provider    string        
	CacheKey    string        
	TemplateKey string        
	CanRetry    bool          
}

// DecisionTree 智能降级决策树
//
// 配置示例:
//
//	tree := NewDecisionTree(DecisionTreeConfig{
//	    PrimaryProvider:   "local-llama-7b-q5",
//	    SecondaryProvider: "local-llama-3b-q4",
//	    CacheEnabled:      true,
//	    TemplateEnabled:   true,
//	    TemplateFallback:  "抱歉, 服务暂时不可用, 请稍后重试。",
//	})
type DecisionTree struct {
	primaryProvider   string
	secondaryProvider string
	cacheEnabled      bool
	templateEnabled   bool
	templateFallback  string
}

// DecisionTreeConfig 决策树配置
type DecisionTreeConfig struct {
	PrimaryProvider   string
	SecondaryProvider string
	CacheEnabled      bool
	TemplateEnabled   bool
	TemplateFallback  string
}

// NewDecisionTree 构造决策树
func NewDecisionTree(cfg DecisionTreeConfig) *DecisionTree {
	return &DecisionTree{
		primaryProvider:   cfg.PrimaryProvider,
		secondaryProvider: cfg.SecondaryProvider,
		cacheEnabled:      cfg.CacheEnabled,
		templateEnabled:   cfg.TemplateEnabled,
		templateFallback:  cfg.TemplateFallback,
	}
}

// Decide 决策: 在某一级别失败时, 决定下一级别
//
// 参数:
//   - currentLevel 当前失败的级别
//   - reason 失败原因
//   - cacheKey 缓存键 (供 LevelCache 查询)
//   - hasCacheHit 缓存是否命中
//
// 返回: FallbackDecision
func (t *DecisionTree) Decide(
	ctx context.Context,
	currentLevel FallbackLevel,
	reason string,
	cacheKey string,
	hasCacheHit bool,
) *FallbackDecision {
	if !featureflag.Get("fallback_chain").Bool() {
		if currentLevel != LevelTemplate {
			logger.Warnf("[FallbackTree] fallback_chain disabled, jumping to template from level=%s reason=%s",
				currentLevel.String(), reason)
		}
		return &FallbackDecision{
			Level:     LevelTemplate,
			FromLevel: currentLevel,
			Reason:    reason,
			CanRetry:  true,
		}
	}

	switch currentLevel {
	case LevelPrimary:
		return &FallbackDecision{
			Level:     LevelSecondary,
			FromLevel: LevelPrimary,
			Reason:    reason,
			Provider:  t.secondaryProvider,
			CanRetry:  true,
		}
	case LevelSecondary:
		if t.cacheEnabled && hasCacheHit {
			return &FallbackDecision{
				Level:     LevelCache,
				FromLevel: LevelSecondary,
				Reason:    reason,
				CacheKey:  cacheKey,
				CanRetry:  true,
			}
		}
		if t.templateEnabled {
			return &FallbackDecision{
				Level:     LevelTemplate,
				FromLevel: LevelSecondary,
				Reason:    reason,
				CanRetry:  true,
			}
		}
		return &FallbackDecision{
			Level:     LevelTemplate,
			FromLevel: LevelSecondary,
			Reason:    "no_template_configured",
			CanRetry:  true,
		}
	case LevelCache:
		if t.templateEnabled {
			return &FallbackDecision{
				Level:     LevelTemplate,
				FromLevel: LevelCache,
				Reason:    reason,
				CanRetry:  true,
			}
		}
		return &FallbackDecision{
			Level:     LevelTemplate,
			FromLevel: LevelCache,
			Reason:    "no_template_configured",
			CanRetry:  true,
		}
	case LevelTemplate:
		return &FallbackDecision{
			Level:     LevelTemplate,
			FromLevel: LevelTemplate,
			Reason:    "end_of_chain",
			CanRetry:  false,
		}
	}
	return &FallbackDecision{
		Level:    LevelTemplate,
		Reason:   fmt.Sprintf("unknown_level_%d", currentLevel),
		CanRetry: false,
	}
}

// ExecuteWithFallback 4 级降级链执行 (集成入口)
//
// 流程:
//  1. 尝试 LevelPrimary (7B)
//  2. 失败 → 尝试 LevelSecondary (3B)
//  3. 失败 → 查询缓存 (LevelCache)
//  4. miss → 返回模板 (LevelTemplate)
//
// 参数:
//   - ctx 上下文
//   - prompt 提示词
//   - callProvider 调用 LLM Provider (返回 content + error)
//   - queryCache 查询缓存 (返回 content + found)
//   - levelTimeoutMs 单级超时
//
// 返回: 4 级降级链中第一个成功的结果
func (t *DecisionTree) ExecuteWithFallback(
	ctx context.Context,
	prompt string,
	callProvider func(ctx context.Context, provider string) (string, error),
	queryCache func(ctx context.Context, key string) (string, bool),
	levelTimeoutMs int,
) (string, FallbackLevel, error) {
	if levelTimeoutMs <= 0 {
		levelTimeoutMs = 60000 
	}

	levelStart := time.Now()
	cctx, cancel := context.WithTimeout(ctx, time.Duration(levelTimeoutMs)*time.Millisecond)
	defer cancel()
	content, err := callProvider(cctx, t.primaryProvider)
	if err == nil {
		logger.Infof("[FallbackTree] level=%s ok, latency=%dms",
			LevelPrimary.String(), time.Since(levelStart).Milliseconds())
		return content, LevelPrimary, nil
	}
	logger.Warnf("[FallbackTree] level=%s failed: %v (latency=%dms)",
		LevelPrimary.String(), err, time.Since(levelStart).Milliseconds())

	levelStart = time.Now()
	cctx2, cancel2 := context.WithTimeout(ctx, time.Duration(levelTimeoutMs)*time.Millisecond)
	defer cancel2()
	content, err = callProvider(cctx2, t.secondaryProvider)
	if err == nil {
		logger.Infof("[FallbackTree] level=%s ok, latency=%dms",
			LevelSecondary.String(), time.Since(levelStart).Milliseconds())
		return content, LevelSecondary, nil
	}
	logger.Warnf("[FallbackTree] level=%s failed: %v (latency=%dms)",
		LevelSecondary.String(), err, time.Since(levelStart).Milliseconds())

	if t.cacheEnabled && queryCache != nil {
		levelStart = time.Now()
		// v3 审计 P1-39 修复：缓存键加 provider + 提示前 32 字符 fingerprint
		// 原：fmt.Sprintf("llm_fallback:%x", prompt) → 跨 scenario/模型串台
		// 新：cacheKey 包含 provider + prompt 哈希 + 短前缀
		hash := sha256.Sum256([]byte(prompt))
		prefix := prompt
		if len(prefix) > 32 {
			prefix = prefix[:32]
		}
		cacheKey := fmt.Sprintf("llm_fallback:%s:%x:%s", t.primaryProvider, hash, prefix)
		if cached, found := queryCache(ctx, cacheKey); found {
			logger.Infof("[FallbackTree] level=%s hit, latency=%dms",
				LevelCache.String(), time.Since(levelStart).Milliseconds())
			return cached, LevelCache, nil
		}
		logger.Warnf("[FallbackTree] level=%s miss, latency=%dms",
			LevelCache.String(), time.Since(levelStart).Milliseconds())
	}

	if t.templateEnabled && t.templateFallback != "" {
		logger.Infof("[FallbackTree] level=%s used", LevelTemplate.String())
		return t.templateFallback, LevelTemplate, nil
	}

	return "", LevelTemplate, fmt.Errorf("fallback chain exhausted: 7B/3B/cache all failed")
}

