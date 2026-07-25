package i18n

// post_validator_test.go PostValidator 单元测试
//
// 覆盖：
//  1. SKU 保护：原文 "Buy SKU-ABC12345 now" → 校验后 SKU 不变
//  2. 价格保护：原文 "Price: $99.99" → 校验后价格不变
//  3. URL 保护：原文 "Visit https://example.com" → 校验后 URL 不变
//  4. Email 保护：原文 "Contact support@brand.com" → 校验后 email 不变
//  5. Glossary 校准：glossary 含 {"顺丰国际": "SF International"} → "SF International 快递"
//  6. 无 Glossary：传 nil glossary → 正常返回，不 panic
//  7. 多重违规：同时含 SKU + 价格 + URL → 全部保护
//  8. 空文本 / 无命中场景
//  9. glossary 自定义保护模式（Patterns）
// 10. applyGlossary 边界（空字符串 / 同值跳过 / 多次出现只记一次）

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SKU 保护
// ============================================================================

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

// ============================================================================
// 价格保护
// ============================================================================

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
	// 多币种：€10 / ¥100 / £50 / ₩1000
	for _, p := range []string{"€10", "¥100", "£50", "₩1000"} {
		out, issues := v.Validate(p, "en", nil)
		assert.Equal(t, p, out, "%s 不应被修改", p)
		assert.NotEmpty(t, issues, "%s 应命中保护", p)
	}
}

// ============================================================================
// URL 保护
// ============================================================================

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

// ============================================================================
// Email 保护
// ============================================================================

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

// ============================================================================
// Glossary 校准
// ============================================================================

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
	// 同一 wrong_form 多次出现只记录一次
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
	// 应有 2 条 glossary_corrected
	count := 0
	for _, is := range issues {
		if is.Type == "glossary_corrected" {
			count++
		}
	}
	assert.Equal(t, 2, count)
}

// ============================================================================
// 无 Glossary（nil 参数）
// ============================================================================

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

// ============================================================================
// 多重违规：SKU + 价格 + URL 同时存在
// ============================================================================

func TestValidate_MultipleViolations_AllProtected(t *testing.T) {
	v := NewPostValidator()
	text := "Buy SKU-ABC12345 for $99.99 at https://example.com"
	out, issues := v.Validate(text, "en", nil)
	assert.Equal(t, text, out, "多重违规应全部保护，文本不变")
	// 至少 3 条 pattern_protected
	count := 0
	for _, is := range issues {
		if is.Type == "pattern_protected" {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, 3, "应记录 SKU/价格/URL 三条保护")
}

// ============================================================================
// 边界场景
// ============================================================================

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

// ============================================================================
// glossary 自定义 Patterns（保护模式）
// ============================================================================

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
		Patterns: []string{`[invalid`}, // 非法正则
	}
	text := "Hello"
	out, issues := v.Validate(text, "en", glossary)
	// 非法模式应被静默跳过，不 panic
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

// ============================================================================
// applyGlossary 边界
// ============================================================================

func TestApplyGlossary_EmptyMappingValue_Skipped(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang: "en",
		Mappings: map[string]string{
			"":        "ignored",  // 空 key 跳过
			"apple":   "",          // 空 value 跳过
			"same":    "same",      // key==value 跳过
			"missing": "iPhone",    // 文本中不存在
		},
	}
	text := "Hello world"
	out, issues := v.Validate(text, "en", glossary)
	assert.Equal(t, text, out)
	assert.Empty(t, issues)
}

// ============================================================================
// 综合：glossary 校准 + 保护共存
// ============================================================================

func TestValidate_GlossaryThenProtection_OrderMatters(t *testing.T) {
	v := NewPostValidator()
	glossary := &GlossaryView{
		Lang:     "en",
		Mappings: map[string]string{"顺丰国际": "SF International"},
	}
	// 文本同时包含术语 + SKU
	text := "顺丰国际 发货 SKU-ABC12345"
	out, issues := v.Validate(text, "en", glossary)
	// 术语先校准
	assert.Equal(t, "SF International 发货 SKU-ABC12345", out)
	// SKU 保护命中
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
