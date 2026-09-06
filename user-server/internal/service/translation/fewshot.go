package translation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	i18npkg "hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/pkg/utils/logger"
)

// FewShotExample 多语言 few-shot 示例。
//
// 存储在 AssetBundle.Examples（JSONArray）中，每个元素为 JSON 对象。
// 字段说明：
//   - Query   ：用户问题（任意语言，通常为目标语言）
//   - Reply   ：标准回复（target_lang）
//   - Lang    ：回复语言短码（如 "en"/"ja"/"ar"），用于按目标语言过滤
//   - Category：分类标签（咨询/售后/投诉/物流/产品），便于后续按场景筛选
type FewShotExample struct {
	Query    string `json:"query"`
	Reply    string `json:"reply"`
	Lang     string `json:"lang"`
	Category string `json:"category"`
}

// FewShotAssetReader 资产包读取接口。
//
// 由 repository.AssetBundleRepository 或 service.AssetBundleService 实现/适配。
// assetType 用于区分资产包用途（如 "fewshot" / "prompt"），实现方可据此筛选。
type FewShotAssetReader interface {
	GetActiveAssetBundle(ctx context.Context, assetType string) (*model.AssetBundle, error)
}

const fewShotCacheTTL = time.Hour

const fewShotCacheKeyPrefix = "fewshot:lang:"

const fewShotDefaultAssetType = "fewshot"

// FewShotService few-shot 示例库服务。
type FewShotService struct {
	assetRepo FewShotAssetReader
	cache     cache.Cache
	assetType string
}

// NewFewShotService 构造 FewShotService。
//
// 参数：
//   - repo  ：资产包读取接口（必填）
//   - c     ：缓存实例，nil 时回退全局缓存（cache.GetGlobalCache）
//   - assetType：资产包类型标识，空串兜底 "fewshot"
func NewFewShotService(repo FewShotAssetReader, c cache.Cache, assetType string) *FewShotService {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	if assetType == "" {
		assetType = fewShotDefaultAssetType
	}
	return &FewShotService{assetRepo: repo, cache: c, assetType: assetType}
}

// LoadByLang 加载某语言下的所有 few-shot 示例。
//
// 流程：
//  1. 命中缓存 → 直接返回反序列化结果
//  2. 未命中 → 读资产包 → 解析 Examples → 按 lang 过滤 → 写缓存 → 返回
//
// 错误处理：
//   - repo 报错：返回 (nil, err)
//   - 缓存读写失败：仅日志，不阻断主流程
//   - 资产包无 Examples：返回空切片（不报错）
//
// lang 为空时兜底 "zh"。未归一化的语言代码会被 NormalizeLang 处理。
func (s *FewShotService) LoadByLang(ctx context.Context, lang string) ([]FewShotExample, error) {
	lang = i18npkg.NormalizeLang(lang)

	if examples, ok := s.loadFromCache(ctx, lang); ok {
		return examples, nil
	}

	bundle, err := s.assetRepo.GetActiveAssetBundle(ctx, s.assetType)
	if err != nil {
		return nil, fmt.Errorf("fewshot: load asset bundle failed: %w", err)
	}
	if bundle == nil || len(bundle.Examples) == 0 {
		s.saveToCache(ctx, lang, nil)
		return nil, nil
	}

	all := parseExamples(bundle.Examples)
	filtered := filterByLang(all, lang)

	s.saveToCache(ctx, lang, filtered)
	return filtered, nil
}

// Render 渲染为 system prompt 块。
//
// 返回空串表示无示例或加载失败（不阻断 LLM 调用）。
// 输出格式（Markdown 友好，可直接拼接到 system prompt 的 FewShotBlock 段落）：
//
//	## FEW-SHOT EXAMPLES (in en)
//	Customer: How long is the warranty?
//	Agent: Our products come with a 12-month warranty.
//	---
//	Customer: ...
func (s *FewShotService) Render(ctx context.Context, lang string) (string, error) {
	lang = i18npkg.NormalizeLang(lang)
	examples, err := s.LoadByLang(ctx, lang)
	if err != nil {
		logger.Warnf("fewshot: load by lang=%s failed: %v", lang, err)
		return "", nil
	}
	if len(examples) == 0 {
		return "", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## FEW-SHOT EXAMPLES (in %s)\n", lang)
	for _, ex := range examples {
		fmt.Fprintf(&b, "Customer: %s\n", ex.Query)
		fmt.Fprintf(&b, "Agent: %s\n", ex.Reply)
		b.WriteString("---\n")
	}
	return b.String(), nil
}

// InvalidateCache 失效指定语言的 few-shot 缓存。
//
// 使用 context.Background() 作为缓存操作的 ctx（供资产包更新后主动调用）。
// lang 为 "*" 或空时清空全部 fewshot 缓存。
func (s *FewShotService) InvalidateCache() {
	ctx := context.Background()
	lang := "*"
	s.invalidateCache(ctx, lang)
}

// InvalidateCacheLang 失效指定语言的 few-shot 缓存（ctx 感知版本）。
func (s *FewShotService) InvalidateCacheLang(ctx context.Context, lang string) {
	s.invalidateCache(ctx, lang)
}

func (s *FewShotService) invalidateCache(ctx context.Context, lang string) {
	lang = i18npkg.NormalizeLang(lang)
	if lang == "" || lang == "*" {
		if err := s.cache.Clear(ctx); err != nil {
			logger.Warnf("fewshot: clear cache failed: %v", err)
		}
		return
	}
	key := fewShotCacheKeyPrefix + lang
	if err := s.cache.Delete(ctx, key); err != nil {
		logger.Warnf("fewshot: invalidate cache key=%s failed: %v", key, err)
	}
}

func (s *FewShotService) loadFromCache(ctx context.Context, lang string) ([]FewShotExample, bool) {
	key := fewShotCacheKeyPrefix + lang
	var examples []FewShotExample
	if err := s.cache.GetJSON(ctx, key, &examples); err != nil {
		logger.Debugf("fewshot: cache get key=%s err=%v", key, err)
		return nil, false
	}
	return examples, true
}

func (s *FewShotService) saveToCache(ctx context.Context, lang string, examples []FewShotExample) {
	key := fewShotCacheKeyPrefix + lang
	if err := s.cache.SetJSON(ctx, key, examples, fewShotCacheTTL); err != nil {
		logger.Warnf("fewshot: cache set key=%s err=%v", key, err)
	}
}

func parseExamples(raw model.JSONArray) []FewShotExample {
	if len(raw) == 0 {
		return nil
	}
	out := make([]FewShotExample, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ex := FewShotExample{
			Query:    toString(m["query"]),
			Reply:    toString(m["reply"]),
			Lang:     toString(m["lang"]),
			Category: toString(m["category"]),
		}
		if ex.Query == "" || ex.Reply == "" {
			continue
		}
		out = append(out, ex)
	}
	return out
}

func filterByLang(examples []FewShotExample, lang string) []FewShotExample {
	if lang == "" {
		return examples
	}
	out := make([]FewShotExample, 0, len(examples))
	for _, ex := range examples {
		if ex.Lang == "" || ex.Lang == lang {
			out = append(out, ex)
		}
	}
	return out
}
