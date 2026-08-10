package llm

// fallback_tree.go 4 级智能降级决策树
//
// 设计依据: AI 智能体性能优化
//
// 目标: 在 Provider 失败时, 自动选择下一级 fallback, 直至 0 出域
// 4 级降级链:
//
//	Level 1: 本地 7B Q5 (主力模型, 质量高, 慢)
//	  ↓ 失败/超时
//	Level 2: 本地 3B Q4 (备份模型, 速度优先, 质量略低)
//	  ↓ 失败/超时
//	Level 3: 缓存命中 (同 prompt 历史结果, 0 延迟)
//	  ↓ miss
//	Level 4: 默认模板 (兜底文案, 100% 可用)
//
// 与 dispatcher.go 的 candidates 链配合使用:
//   - candidates 负责尝试 7B → 3B (LLM 层面)
//   - DecisionTree 负责 7B/3B → 缓存 → 模板 (跨层降级)
//
// 一键回滚: FF_FALLBACK_CHAIN=0 时, 降级链退化为单层 (7B only)

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/pkg/featureflag"
	"hivemtk-user/internal/pkg/utils/logger"
)

// FallbackLevel 降级级别
type FallbackLevel int

const (
	// LevelPrimary 主力 (本地 7B Q5)
	LevelPrimary FallbackLevel = iota + 1
	// LevelSecondary 备份 (本地 3B Q4)
	LevelSecondary
	// LevelCache 缓存命中
	LevelCache
	// LevelTemplate 默认模板
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
	Level       FallbackLevel // 实际使用的级别
	FromLevel   FallbackLevel // 上一个失败级别 (用于埋点)
	Reason      string        // 降级原因 (timeout/error/rate_limit/low_quality)
	Provider    string        // 选中的 Provider 名称
	CacheKey    string        // 缓存键 (Level=LevelCache 时使用)
	TemplateKey string        // 模板键 (Level=LevelTemplate 时使用)
	CanRetry    bool          // 是否可重试 (用于错误恢复)
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
	// 0. 灰度判定: 降级链关闭时, 直接返回 LevelTemplate 兜底
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
		// 7B 失败 → 3B
		return &FallbackDecision{
			Level:     LevelSecondary,
			FromLevel: LevelPrimary,
			Reason:    reason,
			Provider:  t.secondaryProvider,
			CanRetry:  true,
		}
	case LevelSecondary:
		// 3B 失败 → 缓存
		if t.cacheEnabled && hasCacheHit {
			return &FallbackDecision{
				Level:     LevelCache,
				FromLevel: LevelSecondary,
				Reason:    reason,
				CacheKey:  cacheKey,
				CanRetry:  true,
			}
		}
		// 无缓存 → 模板
		if t.templateEnabled {
			return &FallbackDecision{
				Level:     LevelTemplate,
				FromLevel: LevelSecondary,
				Reason:    reason,
				CanRetry:  true,
			}
		}
		// 无模板可用 → 仍返回模板级 (上游会使用默认文案)
		return &FallbackDecision{
			Level:     LevelTemplate,
			FromLevel: LevelSecondary,
			Reason:    "no_template_configured",
			CanRetry:  true,
		}
	case LevelCache:
		// 缓存 miss → 模板
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
		// 已是最后一级, 不再降级
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
		levelTimeoutMs = 60000 // 默认 60s
	}

	// Level 1: 主力 7B
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

	// Level 2: 备份 3B
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

	// Level 3: 缓存
	if t.cacheEnabled && queryCache != nil {
		levelStart = time.Now()
		cacheKey := fmt.Sprintf("llm_fallback:%x", prompt)
		if cached, found := queryCache(ctx, cacheKey); found {
			logger.Infof("[FallbackTree] level=%s hit, latency=%dms",
				LevelCache.String(), time.Since(levelStart).Milliseconds())
			return cached, LevelCache, nil
		}
		logger.Warnf("[FallbackTree] level=%s miss, latency=%dms",
			LevelCache.String(), time.Since(levelStart).Milliseconds())
	}

	// Level 4: 模板兜底
	if t.templateEnabled && t.templateFallback != "" {
		logger.Infof("[FallbackTree] level=%s used", LevelTemplate.String())
		return t.templateFallback, LevelTemplate, nil
	}

	// 全部失败
	return "", LevelTemplate, fmt.Errorf("fallback chain exhausted: 7B/3B/cache all failed")
}
