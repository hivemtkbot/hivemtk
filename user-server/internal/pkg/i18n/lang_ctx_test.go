package i18n

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithInternalLang_InjectAndRead(t *testing.T) {
	ctx := WithInternalLang(context.Background(), "en")
	assert.Equal(t, "en", GetInternalLang(ctx), "注入 en 后应读到 en")
}

func TestWithInternalLang_EmptyFallbackZh(t *testing.T) {
	ctx := WithInternalLang(context.Background(), "")
	assert.Equal(t, "zh", GetInternalLang(ctx), "空值应兜底 zh")
}

func TestGetInternalLang_NoInjection_DefaultZh(t *testing.T) {
	assert.Equal(t, "zh", GetInternalLang(context.Background()))
}

func TestGetInternalLang_Overwrite(t *testing.T) {
	ctx := WithInternalLang(context.Background(), "en")
	ctx = WithInternalLang(ctx, "ja")
	assert.Equal(t, "ja", GetInternalLang(ctx), "二次注入应覆盖前者")
}

func TestWithTargetLang_InjectAndRead(t *testing.T) {
	ctx := WithTargetLang(context.Background(), "ja")
	assert.Equal(t, "ja", GetTargetLang(ctx))
}

func TestWithTargetLang_EmptyFallbackZh(t *testing.T) {
	ctx := WithTargetLang(context.Background(), "")
	assert.Equal(t, "zh", GetTargetLang(ctx))
}

func TestGetTargetLang_NoInjection_DefaultZh(t *testing.T) {
	assert.Equal(t, "zh", GetTargetLang(context.Background()))
}

func TestWithCrossLingual_True(t *testing.T) {
	ctx := WithCrossLingual(context.Background(), true)
	assert.True(t, GetCrossLingual(ctx))
}

func TestWithCrossLingual_False(t *testing.T) {
	ctx := WithCrossLingual(context.Background(), false)
	assert.False(t, GetCrossLingual(ctx))
}

func TestGetCrossLingual_NoInjection_DefaultFalse(t *testing.T) {
	assert.False(t, GetCrossLingual(context.Background()))
}

func TestLangName_KnownLanguages(t *testing.T) {
	cases := map[string]string{
		"zh": "简体中文",
		"en": "English",
		"ja": "日本語",
		"de": "Deutsch",
		"fr": "Français",
		"ko": "한국어",
		"ar": "العربية",
	}
	for code, name := range cases {
		assert.Equal(t, name, LangName(code), "LangName(%q) 应返回 %q", code, name)
	}
}

func TestLangName_UnknownFallbackEnglish(t *testing.T) {
	assert.Equal(t, "English", LangName("xx"), "未知语言应兜底 English")
	assert.Equal(t, "English", LangName(""), "空字符串应兜底 English")
	assert.Equal(t, "English", LangName("中文"), "非 ISO 码应兜底 English")
}

func TestIsValidLang_ValidCodes(t *testing.T) {
	for _, code := range []string{"zh", "en", "ja", "de"} {
		assert.True(t, IsValidLang(code), "%q 应为有效语言", code)
	}
}

func TestIsValidLang_InvalidCodes(t *testing.T) {
	for _, code := range []string{"xx", "", "中文", "ZH", "EN"} {
		assert.False(t, IsValidLang(code), "%q 应为无效语言", code)
	}
}

func TestNormalizeLang_RegionStripped(t *testing.T) {
	assert.Equal(t, "en", NormalizeLang("en-US"))
	assert.Equal(t, "zh", NormalizeLang("zh-Hans"))
	assert.Equal(t, "ja", NormalizeLang("ja-JP"))
}

func TestNormalizeLang_Lowercased(t *testing.T) {
	assert.Equal(t, "zh", NormalizeLang("ZH"))
	assert.Equal(t, "en", NormalizeLang("EN"))
	assert.Equal(t, "de", NormalizeLang("DE"))
}

func TestNormalizeLang_EmptyReturnsZh(t *testing.T) {
	assert.Equal(t, "zh", NormalizeLang(""))
}

func TestNormalizeLang_InvalidReturnsZh(t *testing.T) {
	assert.Equal(t, "zh", NormalizeLang("xx"))
	assert.Equal(t, "zh", NormalizeLang("XX"))
	assert.Equal(t, "zh", NormalizeLang("中文"))
}

func TestNormalizeLang_AlreadyNormalized(t *testing.T) {
	for _, code := range []string{"zh", "en", "ja", "de"} {
		assert.Equal(t, code, NormalizeLang(code))
	}
}
