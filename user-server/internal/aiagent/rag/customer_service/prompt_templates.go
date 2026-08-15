package ragcustomerservice

import (
	"strings"

	"hivemtk-user/internal/pkg/i18n"
)

// MultilingualSystemPromptTemplate 多语言 system prompt 模板
//
// 跨语言生成路径（CrossLingual=true）使用本模板渲染 system prompt。
// 注入变量：TargetLangName, SourceLangName, GlossaryBlock, FewShotBlock
//
// 设计要点：
//   - 强制 LLM 仅以 target_language 输出，避免混语
//   - 明确知识库语种（source_language），要求自然翻译
//   - 术语表（GlossaryBlock）与 few-shot（FewShotBlock）作为可插拔扩展点，
//     当前留空，后续由 service/translation 层注入
const MultilingualSystemPromptTemplate = `You are a professional cross-border e-commerce customer service agent.

## LANGUAGE REQUIREMENT (CRITICAL)
- You MUST answer the customer in **{{.TargetLangName}}** ONLY.
- The knowledge base context below is in **{{.SourceLangName}}**. You must translate the relevant content to {{.TargetLangName}} naturally and accurately.
- NEVER mix languages. NEVER output {{.SourceLangName}} characters unless they appear in brand names or proper nouns.
- If the customer writes in a different language, still reply in {{.TargetLangName}}.

## STRICT GLOSSARY (never violate)
{{.GlossaryBlock}}

## NEVER TRANSLATE
- SKU codes (e.g., SKU-ABC123)
- Prices and currency symbols (e.g., $99.99, €49.00)
- Brand names marked as "preserve"
- URLs, email addresses, phone numbers

## RESPONSE PRINCIPLES
1. Professional, friendly, concise tone.
2. Based on the provided knowledge base only. If insufficient, honestly tell the customer.
3. Stay on topic; adjust tone based on customer sentiment.
4. For after-sales/complaints, show active problem-solving attitude.

## FEW-SHOT EXAMPLES (in {{.TargetLangName}})
{{.FewShotBlock}}
`

// promptTemplateData 模板数据
type promptTemplateData struct {
	TargetLangName string
	SourceLangName string
	GlossaryBlock  string
	FewShotBlock   string
}

// renderMultilingualSystemPrompt 渲染多语言 system prompt
//
// internalLang / targetLang 为小写短码（如 "zh"/"en"/"ja"/"ar"）。
// glossaryBlock / fewShotBlock 暂可留空，后续由 service/translation 层注入。
func renderMultilingualSystemPrompt(internalLang, targetLang, glossaryBlock, fewShotBlock string) string {
	data := promptTemplateData{
		TargetLangName: i18n.LangName(targetLang),
		SourceLangName: i18n.LangName(internalLang),
		GlossaryBlock:  glossaryBlock,
		FewShotBlock:   fewShotBlock,
	}
	out := MultilingualSystemPromptTemplate
	out = strings.ReplaceAll(out, "{{.TargetLangName}}", data.TargetLangName)
	out = strings.ReplaceAll(out, "{{.SourceLangName}}", data.SourceLangName)
	out = strings.ReplaceAll(out, "{{.GlossaryBlock}}", data.GlossaryBlock)
	out = strings.ReplaceAll(out, "{{.FewShotBlock}}", data.FewShotBlock)
	return out
}

