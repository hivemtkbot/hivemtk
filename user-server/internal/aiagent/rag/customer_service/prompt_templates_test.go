package ragcustomerservice


import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRenderMultilingualSystemPrompt_ZhToEn 中文 → 英文
func TestRenderMultilingualSystemPrompt_ZhToEn(t *testing.T) {
	out := renderMultilingualSystemPrompt("zh", "en", "GLOSSARY", "FEW_SHOT")
	assert.Contains(t, out, "English", "应包含 target 名称 English")
	assert.Contains(t, out, "简体中文", "应包含 source 名称 简体中文")
}

// TestRenderMultilingualSystemPrompt_EnToZh 英文 → 中文
func TestRenderMultilingualSystemPrompt_EnToZh(t *testing.T) {
	out := renderMultilingualSystemPrompt("en", "zh", "GLOSSARY", "FEW_SHOT")
	assert.Contains(t, out, "简体中文", "应包含 target 名称 简体中文")
	assert.Contains(t, out, "English", "应包含 source 名称 English")
}

// TestRenderMultilingualSystemPrompt_KnowledgeBaseHint 包含知识库语种提示
func TestRenderMultilingualSystemPrompt_KnowledgeBaseHint(t *testing.T) {
	out := renderMultilingualSystemPrompt("zh", "en", "", "")
	assert.Contains(t, out, "knowledge base context below is in")
}

// TestRenderMultilingualSystemPrompt_GlossaryBlock 注入 glossary 内容
func TestRenderMultilingualSystemPrompt_GlossaryBlock(t *testing.T) {
	glossary := "# Glossary\n- 顺丰国际 → SF International"
	out := renderMultilingualSystemPrompt("zh", "en", glossary, "")
	assert.Contains(t, out, glossary, "应包含 GlossaryBlock 原文")
	assert.NotContains(t, out, "{{.GlossaryBlock}}")
}

// TestRenderMultilingualSystemPrompt_FewShotBlock 注入 few-shot 内容
func TestRenderMultilingualSystemPrompt_FewShotBlock(t *testing.T) {
	fewShot := "Q: Hello\nA: Hi there"
	out := renderMultilingualSystemPrompt("zh", "en", "", fewShot)
	assert.Contains(t, out, fewShot, "应包含 FewShotBlock 原文")
	assert.NotContains(t, out, "{{.FewShotBlock}}")
}

// TestRenderMultilingualSystemPrompt_AllPlaceholdersReplaced 所有占位符应被替换
func TestRenderMultilingualSystemPrompt_AllPlaceholdersReplaced(t *testing.T) {
	out := renderMultilingualSystemPrompt("zh", "en", "G", "F")
	assert.NotContains(t, out, "{{.TargetLangName}}")
	assert.NotContains(t, out, "{{.SourceLangName}}")
	assert.NotContains(t, out, "{{.GlossaryBlock}}")
	assert.NotContains(t, out, "{{.FewShotBlock}}")
}

// TestRenderMultilingualSystemPrompt_EmptyBlocks 空块也允许
func TestRenderMultilingualSystemPrompt_EmptyBlocks(t *testing.T) {
	out := renderMultilingualSystemPrompt("zh", "en", "", "")
	assert.Contains(t, out, "English")
	assert.Contains(t, out, "简体中文")
}

// TestRenderMultilingualSystemPrompt_OtherLanguages 其他语种也能正确映射
func TestRenderMultilingualSystemPrompt_OtherLanguages(t *testing.T) {
	out := renderMultilingualSystemPrompt("zh", "ja", "", "")
	assert.Contains(t, out, "日本語")
	assert.Contains(t, out, "简体中文")
}

// TestRenderMultilingualSystemPrompt_UnknownLang_FallbackEnglish 未知语种应兜底 English
func TestRenderMultilingualSystemPrompt_UnknownLang_FallbackEnglish(t *testing.T) {
	out := renderMultilingualSystemPrompt("xx", "yy", "", "")
	count := strings.Count(out, "English")
	assert.GreaterOrEqual(t, count, 1, "未知语种应兜底 English")
}

// TestMultilingualSystemPromptTemplate_StringConstant 模板常量本身合规
func TestMultilingualSystemPromptTemplate_StringConstant(t *testing.T) {
	assert.Contains(t, MultilingualSystemPromptTemplate, "{{.TargetLangName}}")
	assert.Contains(t, MultilingualSystemPromptTemplate, "{{.SourceLangName}}")
	assert.Contains(t, MultilingualSystemPromptTemplate, "{{.GlossaryBlock}}")
	assert.Contains(t, MultilingualSystemPromptTemplate, "{{.FewShotBlock}}")
}

