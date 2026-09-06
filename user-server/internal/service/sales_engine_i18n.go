package service

import (
	"context"
	"fmt"
	"strings"

	i18npkg "hivemtk-user/internal/pkg/i18n"
)

// GlossaryRenderer 术语表渲染接口（由 service/translation.GlossaryService 实现）。
//
// 返回的 block 追加到 system prompt，约束 LLM 在目标语种下对品牌术语的正确写法。
type GlossaryRenderer interface {
	Render(ctx context.Context, lang string) string
}

// OutputCalibrator 输出后置校准接口（由 service/translation.GlossaryService 适配实现）。
//
// 在 LLM 生成文本返回前做术语校准与敏感模式保护
// （SKU/金额/URL/邮箱/电话等不被误翻译）。
type OutputCalibrator interface {
	Calibrate(ctx context.Context, text string, targetLang string) (string, error)
}

// SetGlossaryRenderer 注入术语表渲染器（可选）。
// 仅跨语言路径追加 GlossaryBlock。传入 nil 表示禁用（向后兼容）。
func (e *SalesEngine) SetGlossaryRenderer(r GlossaryRenderer) {
	e.glossary = r
}

// SetOutputCalibrator 注入输出后置校准器（可选）。
// 在 LLM 生成回复后做术语校准与敏感模式保护。传入 nil 表示禁用（向后兼容）。
func (e *SalesEngine) SetOutputCalibrator(c OutputCalibrator) {
	e.calibrator = c
}

func (e *SalesEngine) resolveTargetLang(ctx context.Context, query string) string {
	internalLang := i18npkg.GetInternalLang(ctx)
	configuredTarget := i18npkg.GetTargetLang(ctx)
	if configuredTarget != "" && configuredTarget != internalLang {
		return configuredTarget
	}
	if detected := i18npkg.DetectLangCode(query); detected != "" && detected != internalLang {
		return detected
	}
	return internalLang
}

func (e *SalesEngine) personaWithLang(ctx context.Context, persona, targetLang string) string {
	internalLang := i18npkg.GetInternalLang(ctx)
	if targetLang == internalLang {
		return persona
	}
	name := i18npkg.LangName(targetLang)
	if name == "" {
		name = targetLang
	}
	var b strings.Builder
	b.WriteString(persona)
	b.WriteString("\n\n")
	b.WriteString(langReplyInstruction(name))
	if e.glossary != nil {
		if block := e.glossary.Render(ctx, targetLang); block != "" {
			b.WriteString("\n\n")
			b.WriteString(block)
		}
	}
	return b.String()
}

func langReplyInstruction(targetLangName string) string {
	return fmt.Sprintf(
		"## LANGUAGE REQUIREMENT (CRITICAL)\n"+
			"- You MUST answer the customer in **%s** ONLY.\n"+
			"- If the customer writes in a different language, still reply in %s.\n"+
			"- NEVER mix languages. For brand names, SKUs, prices, URLs, emails and phone numbers, keep the original form (do not translate them).\n",
		targetLangName, targetLangName,
	)
}

func (e *SalesEngine) calibrate(ctx context.Context, text, targetLang string) string {
	if e.calibrator == nil || text == "" {
		return text
	}
	if out, err := e.calibrator.Calibrate(ctx, text, targetLang); err == nil {
		return out
	}
	return text
}
