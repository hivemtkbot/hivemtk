package service

import (
	"context"
	"fmt"
	"strings"

	i18npkg "marketing/internal/pkg/i18n"
)

// ============================================================================
// 回复语言链路（与 ragcustomerservice 同源，依赖倒置接入 service/i18n）
// ----------------------------------------------------------------------------
// SmartCSOrchestrator → SalesEngine 是主力 AI 客服对话路径，此前完全不感知语种。
// 本文件把「术语表 + 后置校准 + 目标语种路由」接入 SalesEngine 的两条 LLM 路径
// （一次性 Dispatch / Agent Loop），使主对话也按客户语言回复。
//
// 设计约束：
//   - 通过接口（GlossaryRenderer / OutputCalibrator）解耦，service 层不反向依赖
//     service/i18n 的具体类型（i18n.GlossaryService 以鸭子类型同时满足两接口）。
//   - 接口方法签名与 ragcustomerservice 包保持一致，便于统一实现。
//   - 全部可选注入：nil 时跳过对应环节，向后兼容（同语种零开销）。
// ============================================================================

// GlossaryRenderer 术语表渲染接口（由 service/i18n.GlossaryService 实现）。
//
// 返回的 block 追加到 system prompt，约束 LLM 在目标语种下对品牌术语的正确写法。
type GlossaryRenderer interface {
	Render(ctx context.Context, lang string) string
}

// OutputCalibrator 输出后置校准接口（由 service/i18n.GlossaryService 适配实现）。
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

// resolveTargetLang 计算最终输出语种（与 ragcustomerservice 同源逻辑）。
//
// 显式配置了与内部语种不同的 target_language 时严格遵循配置（不自动检测）；
// 否则依据客户消息自动检测语种，命中可识别语种且与内部语种不同时按客户语种回复。
// 无法识别时回退到内部语种，保证向后兼容。
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

// personaWithLang 在 persona（system prompt 基座）后追加多语言指令与术语表块。
// 仅当解析出与内部语种不同的目标语种时生效（同语种零开销，返回原 persona）。
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

// langReplyInstruction 生成强制用目标语种回复的 system prompt 指令（英文，通用且 LLM 易理解）。
func langReplyInstruction(targetLangName string) string {
	return fmt.Sprintf(
		"## LANGUAGE REQUIREMENT (CRITICAL)\n"+
			"- You MUST answer the customer in **%s** ONLY.\n"+
			"- If the customer writes in a different language, still reply in %s.\n"+
			"- NEVER mix languages. For brand names, SKUs, prices, URLs, emails and phone numbers, keep the original form (do not translate them).\n",
		targetLangName, targetLangName,
	)
}

// calibrate 对 LLM 输出做后置校准（术语 + 模式保护）。仅注入校准器时生效。
// 失败时回退原文，不影响主流程可用性。
func (e *SalesEngine) calibrate(ctx context.Context, text, targetLang string) string {
	if e.calibrator == nil || text == "" {
		return text
	}
	if out, err := e.calibrator.Calibrate(ctx, text, targetLang); err == nil {
		return out
	}
	return text
}
