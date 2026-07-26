package i18n

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"marketing/internal/cache"
	"marketing/internal/model"
	i18npkg "marketing/internal/pkg/i18n"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// ErrGlossaryNotFound 术语未找到 sentinel error
//
// 五层架构修复：controller 不应直接引用 gorm.ErrRecordNotFound，
// service 层将底层 ORM 错误转为业务 sentinel error，供 controller 用 errors.Is 检查。
// service 层引用 gorm 包仅用于错误识别，不直接持有/使用 *gorm.DB（符合五层架构）。
var ErrGlossaryNotFound = errors.New("glossary: not found")

// ============================================================================
// GlossaryService 术语表服务（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 职责：
//   1. 加载某目标语言下的术语映射（src → dst）+ 保护模式
//   2. 渲染为 system prompt 块，供 LLM 调用时注入
//   3. Redis 缓存（key: glossary:lang:{lang}，TTL 1h）
//   4. 术语更新时主动失效缓存（保持最终一致）
//
// 五层架构归属：L3 业务服务层。本服务不直接访问 db，
// 通过 GlossaryRepo 接口委托 repository 层。
// ============================================================================

// GlossaryRepo 术语表仓储接口（由 repository.GlossaryRepository 实现）。
type GlossaryRepo interface {
	ListActive(ctx context.Context) ([]*model.Glossary, error)
	GetByTermID(ctx context.Context, termID string) (*model.Glossary, error)
	Create(ctx context.Context, g *model.Glossary) error
	Update(ctx context.Context, g *model.Glossary) error
	Delete(ctx context.Context, termID string) error
	ListByCategory(ctx context.Context, category string) ([]*model.Glossary, error)
	ListAll(ctx context.Context, status, keyword string, page, pageSize int) ([]*model.Glossary, int64, error)
}

// GlossaryView 术语表视图（针对某目标语言渲染后的结果）。
//
// 字段说明：
//   - Lang     ：目标语言短码
//   - Mappings ：源语言词 → 目标语言词（用于 LLM 输出后置校准）
//   - Patterns ：正则保护模式（命中后保持原样，不翻译）
type GlossaryView struct {
	Lang     string            `json:"lang"`
	Mappings map[string]string `json:"mappings"`
	Patterns []string          `json:"patterns"`
}

// glossaryTranslation 术语翻译条目（与 model.Glossary.Translations 中
// 每个 JSON 元素的结构对齐）。
type glossaryTranslation struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

// glossaryCacheTTL 缓存默认 TTL（1 小时）。
const glossaryCacheTTL = time.Hour

// glossaryCacheKeyPrefix 缓存 key 前缀。
const glossaryCacheKeyPrefix = "glossary:lang:"

// GlossaryService 术语表服务。
type GlossaryService struct {
	repo  GlossaryRepo
	cache cache.Cache
}

// NewGlossaryService 构造 GlossaryService。
//
// cache 为 nil 时回退全局缓存（cache.GetGlobalCache），保证未注入 Redis
// 的单实例部署也能工作。
func NewGlossaryService(repo GlossaryRepo, c cache.Cache) *GlossaryService {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	return &GlossaryService{repo: repo, cache: c}
}

// LoadByLang 加载某目标语言下的术语视图。
//
// 流程：
//  1. 命中缓存 → 直接返回反序列化结果
//  2. 未命中 → 调 repo.ListActive → 构建 GlossaryView → 写缓存 → 返回
//
// 错误处理：
//   - repo 报错：返回 (nil, err)
//   - 缓存读写失败：仅记录日志，不阻断主流程（缓存只是优化）
//
// lang 为空时兜底 "zh"。未归一化的语言代码会被 NormalizeLang 处理。
func (s *GlossaryService) LoadByLang(ctx context.Context, lang string) (*GlossaryView, error) {
	lang = i18npkg.NormalizeLang(lang)

	// 1. 命中缓存直接返回
	if view, ok := s.loadFromCache(ctx, lang); ok {
		return view, nil
	}

	// 2. 加载全量 active 术语
	list, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("glossary: list active failed: %w", err)
	}

	// 3. 构建视图
	view := s.buildView(lang, list)

	// 4. 写缓存（best-effort）
	s.saveToCache(ctx, view)
	return view, nil
}

// Render 渲染术语表为 system prompt 块。
//
// 返回空串表示无术语或加载失败（不阻断 LLM 调用）。
// 输出格式（Markdown 友好，可直接拼接到 system prompt 末尾）：
//
//	# Glossary (target=zh)
//
//	## 必须使用的目标语言术语（用右侧替换左侧）：
//	- Apple → 苹果
//
//	## 不可翻译的保护模式（保持原样）：
//	- SKU-[A-Z0-9]{6,}
func (s *GlossaryService) Render(ctx context.Context, lang string) string {
	lang = i18npkg.NormalizeLang(lang)
	view, err := s.LoadByLang(ctx, lang)
	if err != nil || view == nil {
		return ""
	}
	if len(view.Mappings) == 0 && len(view.Patterns) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Glossary (target=%s)\n\n", view.Lang)

	if len(view.Mappings) > 0 {
		b.WriteString("## 必须使用的目标语言术语（用右侧替换左侧）：\n")
		for src, dst := range view.Mappings {
			fmt.Fprintf(&b, "- %s → %s\n", src, dst)
		}
		b.WriteString("\n")
	}
	if len(view.Patterns) > 0 {
		b.WriteString("## 不可翻译的保护模式（保持原样）：\n")
		for _, p := range view.Patterns {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	return b.String()
}

// InvalidateCache 失效指定语言的术语缓存。
//
// 术语 CRUD 后由调用方主动调用。lang 为 "*" 或空时清空全部 glossary 缓存
// （依赖 cache.Clear，对 Redis 后端慎用 — 会清空整个 db）。
func (s *GlossaryService) InvalidateCache(ctx context.Context, lang string) {
	lang = i18npkg.NormalizeLang(lang)
	if lang == "" || lang == "*" {
		// 仅清 glossary:lang:* —— 当前 Cache 接口未提供 SCAN/批量删除，
		// 退化到清空全部缓存，调用方需自行权衡。
		if err := s.cache.Clear(ctx); err != nil {
			logger.Warnf("glossary: clear cache failed: %v", err)
		}
		return
	}
	key := glossaryCacheKeyPrefix + lang
	if err := s.cache.Delete(ctx, key); err != nil {
		logger.Warnf("glossary: invalidate cache key=%s failed: %v", key, err)
	}
}

// InvalidateAll 失效全部 glossary 缓存（用于术语批量导入等场景）。
func (s *GlossaryService) InvalidateAll(ctx context.Context) {
	if err := s.cache.Clear(ctx); err != nil {
		logger.Warnf("glossary: clear all cache failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// CRUD 包装方法（供 controller 调用，五层架构：controller → service → repository）
// ----------------------------------------------------------------------------

// Create 创建术语。写库后失效全量缓存（保证后续 LoadByLang 拉到最新数据）。
func (s *GlossaryService) Create(ctx context.Context, g *model.Glossary) error {
	if g == nil {
		return fmt.Errorf("glossary: nil entity")
	}
	if g.Status == "" {
		g.Status = "active"
	}
	if err := s.repo.Create(ctx, g); err != nil {
		return fmt.Errorf("glossary: create failed: %w", err)
	}
	s.InvalidateAll(ctx)
	return nil
}

// Update 更新术语（按主键 ID）。写库后失效全量缓存。
func (s *GlossaryService) Update(ctx context.Context, g *model.Glossary) error {
	if g == nil {
		return fmt.Errorf("glossary: nil entity")
	}
	if err := s.repo.Update(ctx, g); err != nil {
		return fmt.Errorf("glossary: update failed: %w", err)
	}
	s.InvalidateAll(ctx)
	return nil
}

// Delete 删除术语（按 TermID）。写库后失效全量缓存。
func (s *GlossaryService) Delete(ctx context.Context, termID string) error {
	if termID == "" {
		return fmt.Errorf("glossary: empty term_id")
	}
	if err := s.repo.Delete(ctx, termID); err != nil {
		return fmt.Errorf("glossary: delete failed: %w", err)
	}
	s.InvalidateAll(ctx)
	return nil
}

// GetByTermID 按 TermID 查询单条术语。
func (s *GlossaryService) GetByTermID(ctx context.Context, termID string) (*model.Glossary, error) {
	if termID == "" {
		return nil, fmt.Errorf("glossary: empty term_id")
	}
	g, err := s.repo.GetByTermID(ctx, termID)
	if err != nil {
		// 五层架构修复：将底层 gorm.ErrRecordNotFound 转为业务 sentinel error，
		// controller 不再需要直接引用 gorm 包
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGlossaryNotFound
		}
		return nil, fmt.Errorf("glossary: get by term_id failed: %w", err)
	}
	return g, nil
}

// List 分页查询术语（支持 status / keyword 过滤）。
func (s *GlossaryService) List(ctx context.Context, status, keyword string, page, pageSize int) ([]*model.Glossary, int64, error) {
	list, total, err := s.repo.ListAll(ctx, status, keyword, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("glossary: list failed: %w", err)
	}
	return list, total, nil
}

// ListByCategory 按分类查询 active 术语（全量，不分页）。
func (s *GlossaryService) ListByCategory(ctx context.Context, category string) ([]*model.Glossary, error) {
	list, err := s.repo.ListByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("glossary: list by category failed: %w", err)
	}
	return list, nil
}

// ----------------------------------------------------------------------------
// 内部辅助
// ----------------------------------------------------------------------------

// loadFromCache 从缓存读取视图；未命中或反序列化失败时返回 (nil, false)。
//
// 兼容两种后端：
//   - RedisCache.GetJSON：未命中返回 redis.Nil 错误
//   - MemoryCache.GetJSON：未命中返回 nil（无错误），但反序列化后 view.Lang 为空
//
// 因此通过「err != nil || view.Lang == ""」双重判定。
func (s *GlossaryService) loadFromCache(ctx context.Context, lang string) (*GlossaryView, bool) {
	key := glossaryCacheKeyPrefix + lang
	var view GlossaryView
	if err := s.cache.GetJSON(ctx, key, &view); err != nil {
		// 缓存未命中是正常路径，仅 debug 日志
		logger.Debugf("glossary: cache get key=%s err=%v", key, err)
		return nil, false
	}
	if view.Lang == "" {
		return nil, false
	}
	return &view, true
}

// saveToCache 写缓存（best-effort，失败仅日志）。
func (s *GlossaryService) saveToCache(ctx context.Context, view *GlossaryView) {
	if view == nil {
		return
	}
	key := glossaryCacheKeyPrefix + view.Lang
	if err := s.cache.SetJSON(ctx, key, view, glossaryCacheTTL); err != nil {
		logger.Warnf("glossary: cache set key=%s err=%v", key, err)
	}
}

// buildView 从术语列表构建指定语言的视图。
//
// 规则：
//   - Preserve=true：仅追加 Pattern 到保护模式（不参与 Mappings）
//   - Preserve=false：解析 Translations（{lang:text}），找出目标语言文本；
//     其余语言的文本作为"错误形式"映射到目标文本（供 PostValidator 校准）
//   - Pattern 非空：始终追加到 Patterns（无论 Preserve）
func (s *GlossaryService) buildView(lang string, list []*model.Glossary) *GlossaryView {
	view := &GlossaryView{
		Lang:     lang,
		Mappings: make(map[string]string),
	}
	patternSeen := make(map[string]struct{})

	for _, g := range list {
		if g == nil {
			continue
		}
		// 保护模式：去重追加
		if g.Pattern != "" {
			if _, ok := patternSeen[g.Pattern]; !ok {
				view.Patterns = append(view.Patterns, g.Pattern)
				patternSeen[g.Pattern] = struct{}{}
			}
		}
		// Preserve=true：仅作为保护项，不参与 Mappings
		if g.Preserve {
			continue
		}

		translations := parseTranslations(g.Translations)
		if len(translations) == 0 {
			continue
		}

		// 找出目标语言文本
		dst := pickTextByLang(translations, lang)
		if dst == "" {
			// 目标语言没有翻译条目，跳过此术语
			continue
		}

		// 其余语言文本作为"错误形式"映射到目标文本
		for _, t := range translations {
			if t.Lang == lang {
				continue
			}
			if t.Text == "" || t.Text == dst {
				continue
			}
			// 首次写入优先；后续重复保留首个映射避免抖动
			if _, exists := view.Mappings[t.Text]; !exists {
				view.Mappings[t.Text] = dst
			}
		}
	}
	return view
}

// parseTranslations 解析 model.Glossary.Translations（model.JSONMap）。
//
// 存储格式：`{"en":"Apple","zh":"苹果"}` —— map[lang]text。
// 非字符串的值（如嵌套对象）跳过；空 text 跳过。
func parseTranslations(raw model.JSONMap) []glossaryTranslation {
	if len(raw) == 0 {
		return nil
	}
	out := make([]glossaryTranslation, 0, len(raw))
	for lang, val := range raw {
		if lang == "" {
			continue
		}
		text := toString(val)
		if text == "" {
			continue
		}
		out = append(out, glossaryTranslation{Lang: lang, Text: text})
	}
	return out
}

// toString 把 any 安全转为 string（数字/布尔也兼容）。
func toString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// pickTextByLang 从翻译列表中挑选指定语言的文本（无命中返回空串）。
func pickTextByLang(translations []glossaryTranslation, lang string) string {
	for _, t := range translations {
		if t.Lang == lang {
			return t.Text
		}
	}
	return ""
}
