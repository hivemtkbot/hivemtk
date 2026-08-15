package i18n

import (
	"context"
	"strings"
)


// 多语言 ctx key（私有类型避免与其他包冲突）
type (
	ctxKeyInternalLang struct{}
	ctxKeyTargetLang   struct{}
	ctxKeyCrossLingual struct{}
)

// WithInternalLang 注入商户内部语言到 ctx。
// lang 为空时兜底 "zh"。
func WithInternalLang(ctx context.Context, lang string) context.Context {
	if lang == "" {
		lang = "zh"
	}
	return context.WithValue(ctx, ctxKeyInternalLang{}, lang)
}

// WithTargetLang 注入对外目标语言到 ctx。
// lang 为空时兜底 "zh"。
func WithTargetLang(ctx context.Context, lang string) context.Context {
	if lang == "" {
		lang = "zh"
	}
	return context.WithValue(ctx, ctxKeyTargetLang{}, lang)
}

// WithCrossLingual 注入是否跨语言生成标志。
func WithCrossLingual(ctx context.Context, cross bool) context.Context {
	return context.WithValue(ctx, ctxKeyCrossLingual{}, cross)
}

// GetInternalLang 从 ctx 读取商户内部语言（默认 "zh"）。
func GetInternalLang(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyInternalLang{}).(string); ok && v != "" {
		return v
	}
	return "zh"
}

// GetTargetLang 从 ctx 读取对外目标语言（默认 "zh"）。
func GetTargetLang(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTargetLang{}).(string); ok && v != "" {
		return v
	}
	return "zh"
}

// GetCrossLingual 从 ctx 读取是否跨语言生成。
func GetCrossLingual(ctx context.Context) bool {
	if v, ok := ctx.Value(ctxKeyCrossLingual{}).(bool); ok {
		return v
	}
	return false
}

// SupportedLanguages 支持的语言枚举（ISO 639-1 短码 → 可读名称）。
// 与 locale.go 的 Locale 常量相比，这里覆盖更广的出海语种。
var SupportedLanguages = map[string]string{
	"zh": "简体中文",
	"en": "English",
	"ja": "日本語",
	"ko": "한국어",
	"de": "Deutsch",
	"fr": "Français",
	"es": "Español",
	"pt": "Português",
	"ru": "Русский",
	"ar": "العربية",
	"it": "Italiano",
	"nl": "Nederlands",
	"th": "ไทย",
	"vi": "Tiếng Việt",
	"id": "Bahasa Indonesia",
	"tr": "Türkçe",
	"pl": "Polski",
	"hi": "हिन्दी",
}

// LangName 返回语言代码的可读名称。未知语言兜底英文。
func LangName(code string) string {
	if name, ok := SupportedLanguages[code]; ok {
		return name
	}
	return "English"
}

// IsValidLang 校验语言代码是否受支持。
func IsValidLang(code string) bool {
	_, ok := SupportedLanguages[code]
	return ok
}

// NormalizeLang 归一化语言代码：
//   - 去除 region 子标签（en-US → en, zh-Hans → zh）
//   - 转小写
//   - 不受支持时回退 "zh"
//
// 空串返回 "zh"。
func NormalizeLang(code string) string {
	if code == "" {
		return "zh"
	}
	if idx := strings.Index(code, "-"); idx > 0 {
		code = code[:idx]
	}
	code = strings.ToLower(code)
	if !IsValidLang(code) {
		return "zh"
	}
	return code
}

