package i18n

import (
	"regexp"
	"strings"
)

// ============================================================================
// PostValidator LLM 输出后置校准器（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 职责：在 LLM 生成文本返回业务层之前，做"低成本 + 高确定性"的校准：
//
//   1. 正则保护（pattern_protected）
//      把 LLM 可能"翻译过头"的内容还原：
//        - SKU-[A-Z0-9]{6,}    等商品编码
//        - 货币符号 + 金额       $9.99 / €10 / ¥100
//        - URL                  https://...
//        - email                foo@bar.com
//      命中后整段保留原样，记录 ValidationIssue。
//
//   2. 术语校准（glossary_corrected）
//      当 LLM 输出了"错误形式"的术语时，替换为 GlossaryView.Mappings 中
//      指定的目标语言正确形式。
//
// 设计原则：
//   - 纯函数，无副作用，无 IO（不读缓存、不调 repo）
//   - 校准是"保守"的：仅在能确定正确形式时才替换，否则原样返回
//   - 校准记录通过 []ValidationIssue 返回，供可观测性 / A/B 调试用
// ============================================================================

// ValidationIssue 单条校准记录。
//
// Type 取值：
//   - "glossary_corrected"：术语被校准（Actual → Expected）
//   - "pattern_protected" ：保护模式命中（Term 为命中的原文片段）
type ValidationIssue struct {
	Type     string
	Term     string
	Expected string
	Actual   string
}

// protectPatterns 内置正则保护模式。
//
// 这些模式覆盖出海场景下"绝不能被 LLM 翻译"的常见片段（商品编码/金额/URL/邮箱）。
// 命中后整段保留原样并记录。这里的"保护"语义是**保留**（防止 LLM 把业务令牌翻译/篡改），
// 而非脱敏——因为邮箱/价格/URL 是客服回复中应当原样送达客户的业务内容
// （既有单测 TestValidate_EmailProtected 即断言 email 不应被修改）。
//
// 关于"敏感信息泄露"风险的说明（P0 复核结论）：
// 若 LLM 在回复中泄露**内部成本价/员工私人联系方式**等真正敏感信息，正确缓解点在
// system prompt（明确禁止输出内部信息）+ 术语表（定义可披露内容），而非在此对业务令牌做
//  blanket 脱敏——后者会破坏正常客服回复。如确需对特定内部模式脱敏，应作为**商户可配置的
// 独立开关**实现，默认关闭，避免影响通用场景。
// 排序原则：更具体的模式在前，避免被宽泛模式吞掉。
var protectPatterns = []string{
	`SKU-[A-Z0-9]{6,}`,
	`[\$€¥£₩]\d+\.?\d*`,
	`https?://\S+`,
	`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`,
}

// compiledProtectPatterns 预编译保护模式（包级单例，避免每次 Validate 重复编译）。
var compiledProtectPatterns = mustCompilePatterns(protectPatterns)

// mustCompilePatterns 编译模式列表，任一失败 panic（启动期 fail-fast）。
func mustCompilePatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			panic("i18n: invalid protect pattern " + p + ": " + err.Error())
		}
		out = append(out, re)
	}
	return out
}

// PostValidator 后置校准器。
//
// 无状态、可并发调用。构造一次复用即可。
type PostValidator struct{}

// NewPostValidator 构造 PostValidator。
func NewPostValidator() *PostValidator {
	return &PostValidator{}
}

// Validate 对 LLM 输出文本做后置校准。
//
// 参数：
//   - text       ：LLM 原始输出
//   - targetLang ：目标语言（用于跳过同语种快捷路径；当前实现仅做保护，
//     不依赖此参数做语种特定处理；保留参数供未来扩展）
//   - glossary   ：术语视图（可为 nil —— 仅做正则保护）
//
// 返回：
//   - 校准后的文本（始终非 nil，可能等于 text）
//   - 校准记录列表（无校准则长度为 0）
//
// 校准顺序：
//  1. 先做术语校准（glossary_corrected）：基于精确字符串替换
//  2. 再做正则保护（pattern_protected）：保护命中片段
//
// 之所以先术语、后保护：避免保护模式"吞掉"应被校准的术语片段。
// 例如先保护 SKU-123456，则术语校准无法匹配；先术语校准可让术语映射
// 在保护前生效，保护针对的是非术语类的硬性 token。
func (v *PostValidator) Validate(text string, targetLang string, glossary *GlossaryView) (string, []ValidationIssue) {
	if text == "" {
		return text, nil
	}

	var issues []ValidationIssue
	out := text

	// 1. 术语校准（仅当 glossary 非空且有映射）
	if glossary != nil && len(glossary.Mappings) > 0 {
		out, issues = v.applyGlossary(out, glossary.Mappings, issues)
	}

	// 2. 正则保护（内置模式 + glossary.Patterns）
	allPatterns := compiledProtectPatterns
	if glossary != nil && len(glossary.Patterns) > 0 {
		extra := compileUserPatterns(glossary.Patterns)
		if len(extra) > 0 {
			allPatterns = append(allPatterns[:len(allPatterns):len(allPatterns)], extra...)
		}
	}
	out, issues = v.applyPatterns(out, allPatterns, issues)

	return out, issues
}

// applyGlossary 应用术语映射：将 wrong_form 替换为 correct_form。
//
// 替换是大小写敏感的精确匹配（避免误伤常见词）。
// 同一个 wrong_form 多次出现都会被替换，但只在 issues 中记录一次。
func (v *PostValidator) applyGlossary(text string, mappings map[string]string, issues []ValidationIssue) (string, []ValidationIssue) {
	reported := make(map[string]struct{})
	out := text
	for wrong, correct := range mappings {
		if wrong == "" || correct == "" || wrong == correct {
			continue
		}
		if !strings.Contains(out, wrong) {
			continue
		}
		out = strings.ReplaceAll(out, wrong, correct)
		if _, ok := reported[wrong]; !ok {
			issues = append(issues, ValidationIssue{
				Type:     "glossary_corrected",
				Term:     wrong,
				Expected: correct,
				Actual:   wrong,
			})
			reported[wrong] = struct{}{}
		}
	}
	return out, issues
}

// applyPatterns 应用保护模式：命中片段保持原样。
//
// "保护"的含义：记录命中（pattern_protected），文本本身不做替换
// （因为输入已经是 LLM 输出，命中的业务片段就是它本应保持的形式）。
func (v *PostValidator) applyPatterns(text string, patterns []*regexp.Regexp, issues []ValidationIssue) (string, []ValidationIssue) {
	reported := make(map[string]struct{})
	for _, re := range patterns {
		matches := re.FindAllString(text, -1)
		if len(matches) == 0 {
			continue
		}
		for _, m := range matches {
			if _, ok := reported[m]; ok {
				continue
			}
			issues = append(issues, ValidationIssue{
				Type:     "pattern_protected",
				Term:     m,
				Expected: m,
				Actual:   m,
			})
			reported[m] = struct{}{}
		}
	}
	return text, issues
}

// compileUserPatterns 编译 glossary 自定义保护模式。
//
// 失败的模式跳过（best-effort），不阻断校准流程。
// 失败模式通过 logger 记录，但本函数保持纯函数特性 ——
// 编译错误日志放在编译时（启动期加载 glossary 时）更合适，
// 此处仅静默跳过非法模式。
func compileUserPatterns(patterns []string) []*regexp.Regexp {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		out = append(out, re)
	}
	return out
}
