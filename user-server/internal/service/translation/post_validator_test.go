package translation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_SKUProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Buy SKU-ABC12345 now"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "SKU 不应被修改")
	found := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && is.Term == "SKU-ABC12345" {
			found = true
		}
	}
	assert.True(t, found, "应记录 SKU pattern_protected")
}

func TestValidate_PriceProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Price: $99.99"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "价格不应被修改")
	found := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && is.Term == "$99.99" {
			found = true
		}
	}
	assert.True(t, found, "应记录价格 pattern_protected")
}

func TestValidate_PriceMultiCurrencies(t *testing.T) {
	v := NewPostValidator()
	for _, p := range []string{"€10", "¥100", "£50", "₩1000"} {
		out, issues := v.Validate(p, "en", nil)
		assert.Equal(t, p, out, "%s 不应被修改", p)
		assert.NotEmpty(t, issues, "%s 应命中保护", p)
	}
}

func TestValidate_URLProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Visit https://example.com today"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "URL 不应被修改")
	found := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && strings.Contains(is.Term, "https://example.com") {
			found = true
		}
	}
	assert.True(t, found, "应记录 URL pattern_protected")
}

func TestValidate_HTTPUrlProtected(t *testing.T) {
	v := NewPostValidator()
	text := "See http://example.org/path?q=1"
	out, _ := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "http URL 也不应被修改")
}

func TestValidate_EmailProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Contact support@brand.com for help"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "email 不应被修改")
	found := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && is.Term == "support@brand.com" {
			found = true
		}
	}
	assert.True(t, found, "应记录 email pattern_protected")
}

func TestValidate_GlossaryCorrection(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Mappings: map[string]string{"顺丰国际": "SF International"},
	}
	text := "顺丰国际 快递"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, "SF International 快递", out)
	require.Len(t, issues, 1)
	assert.Equal(t, "glossary_corrected", issues[0].Type)
	assert.Equal(t, "顺丰国际", issues[0].Term)
	assert.Equal(t, "SF International", issues[0].Expected)
	assert.Equal(t, "顺丰国际", issues[0].Actual)
}

func TestValidate_GlossaryMultipleOccurrences_RecordsOnce(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Mappings: map[string]string{"顺丰国际": "SF International"},
	}
	text := "顺丰国际 1，顺丰国际 2"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, "SF International 1，SF International 2", out)
	count := 0
	for _, is := range issues {
		if is.Type == "glossary_corrected" && is.Term == "顺丰国际" {
			count++
		}
	}
	assert.Equal(t, 1, count, "同一 wrong_form 只记录一次")
}

func TestValidate_GlossaryMultipleMappings(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang: "en",
		Mappings: map[string]string{
			"顺丰国际": "SF International",
			"苹果手机": "iPhone",
		},
	}
	text := "顺丰国际 配送 苹果手机"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, "SF International 配送 iPhone", out)
	count := 0
	for _, is := range issues {
		if is.Type == "glossary_corrected" {
			count++
		}
	}
	assert.Equal(t, 2, count)
}

func TestValidate_NilGlossary_NoPanic(t *testing.T) {
	v := NewPostValidator()
	text := "Hello world"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out)
	assert.Empty(t, issues, "无 glossary 且无保护命中时应无 issue")
}

func TestValidate_EmptyGlossary(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{Lang: "en", Mappings: map[string]string{}}
	text := "Hello world"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

func TestValidate_MultipleViolations_AllProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Buy SKU-ABC12345 for $99.99 at https://example.com"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "多重违规应全部保护，文本不变")
	count := 0
	for _, is := range issues {
		if is.Type == "pattern_protected" {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, 3, "应记录 SKU/价格/URL 三条保护")
}

func TestValidate_EmptyText(t *testing.T) {
	v := NewPostValidator()
	out, issues := v.Validate("", "en", nil)
	assert.Equal(t, "", out)
	assert.Nil(t, issues)
}

func TestValidate_NoMatches(t *testing.T) {
	v := NewPostValidator()
	text := "普通文本无任何保护模式"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

func TestValidate_GlossaryCustomPatterns(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Patterns: []string{`ORDER-\d+`},
	}
	text := "Order ORDER-12345 confirmed"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, text, out, "自定义保护模式命中后文本不变")
	found := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && is.Term == "ORDER-12345" {
			found = true
		}
	}
	assert.True(t, found, "应记录自定义保护模式命中")
}

func TestValidate_GlossaryInvalidPattern_SkippedGracefully(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Patterns: []string{`[invalid`},
	}
	text := "Hello"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

func TestApplyGlossary_EmptyMappingValue_Skipped(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang: "en",
		Mappings: map[string]string{
			"":        "ignored",
			"apple":   "",
			"same":    "same",
			"missing": "iPhone",
		},
	}
	text := "Hello world"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

func TestValidate_GlossaryThenProtection_OrderMatters(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Mappings: map[string]string{"顺丰国际": "SF International"},
	}
	text := "顺丰国际 发货 SKU-ABC12345"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, "SF International 发货 SKU-ABC12345", out)
	foundSKU := false
	foundGlossary := false
	for _, is := range issues {
		if is.Type == "pattern_protected" && is.Term == "SKU-ABC12345" {
			foundSKU = true
		}
		if is.Type == "glossary_corrected" && is.Term == "顺丰国际" {
			foundGlossary = true
		}
	}
	assert.True(t, foundSKU, "应记录 SKU 保护")
	assert.True(t, foundGlossary, "应记录术语校准")
}

// TestValidate_SensitiveRedacted 验证内部敏感信息被真正脱敏（[REDACTED]），
// 而业务令牌（email/价格）仍保留原样。
func TestValidate_SensitiveRedacted(t *testing.T) {
	v := NewPostValidator()
	text := "客户邮箱 support@brand.com 价格 $99.99 身份证 11010119900307123X 银行卡 6222021234567890123 成本价 ¥50"

	out, issues := v.Validate(text, "zh", nil)

	assert.Contains(t, out, "support@brand.com", "email 业务令牌应保留")
	assert.Contains(t, out, "$99.99", "价格业务令牌应保留")
	assert.NotContains(t, out, "11010119900307123X", "身份证应被脱敏")
	assert.NotContains(t, out, "6222021234567890123", "银行卡应被脱敏")
	assert.NotContains(t, out, "成本价 ¥50", "内部成本价应被脱敏")
	assert.Contains(t, out, "[REDACTED]", "至少一处敏感被脱敏")

	count := 0
	for _, is := range issues {
		if is.Type == "pattern_redacted" {
			assert.Equal(t, "[REDACTED]", is.Expected)
			assert.Equal(t, "[REDACTED]", is.Actual, "脱敏不应留存原文到 issue")
			count++
		}
	}
	assert.GreaterOrEqual(t, count, 3, "身份证/银行卡/成本价 至少 3 处脱敏")
}

// TestValidate_RedactExcludesProtectedURL 验证 URL 内的长数字不被误脱敏
// （保护区间排除机制生效，避免误伤 URL 资源 ID）。
func TestValidate_RedactExcludesProtectedURL(t *testing.T) {
	v := NewPostValidator()
	text := "详见 https://api.example.com/v1/order/6222021234567890123"
	out, _ := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "URL 内的长数字应受保护不被脱敏")
}
