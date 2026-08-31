package translation

import (
	"regexp"
	"sort"
	"strings"
)

// ValidationIssue 单条校准记录。
//
// Type 取值：
//   - "glossary_corrected"：术语被校准（Actual → Expected）
//   - "pattern_protected" ：业务令牌被保留（Term 为命中的原文片段）
//   - "pattern_redacted"  ：内部敏感信息被脱敏（Term 为命中的正则；Actual 为 [REDACTED]，不留存原文）
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
// 而非脱敏——因为邮箱/价格/URL 是客服回复中应当原样送达客户的业务内容。
//
// 排序原则：更具体的模式在前，避免被宽泛模式吞掉。
var protectPatterns = []string{
	`SKU-[A-Z0-9]{6,}`,
	`[\$€¥£₩]\d+\.?\d*`,
	`https?://\S+`,
	`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`,
}

// redactPatterns 内置内部敏感脱敏模式（v1.3 新增）。
//
// 这些模式覆盖"绝对不应随 LLM 回复外泄"的内部信息：
//   - 中国大陆身份证号（18 位）
//   - 16~19 位纯数字账号/银行卡号
//   - RFC1918 内网 IP（10./192.168./172.16~31.）
//   - API 密钥前缀（sk-/AKIA）与 Bearer token
//   - 内部成本价/供货价/利润等带金额标记
//
// 命中后整段替换为 [REDACTED]，绝不留存原文到 issues（Actual 固定为 [REDACTED]）。
//
// 与 protectPatterns 的边界：protect 命中的区间（URL/email/价格等）在脱敏时会被排除，
// 因此 email/价格/URL 不会被误脱敏，保障正常客服回复不被破坏。
//
// 关于误伤：\d{16,19} 也会命中独立的长数字订单号/资源 ID。出于"敏感优先"原则保留该模式；
// 业务侧如确有长数字业务令牌需原样送达，应通过 glossary.Patterns 加入保护或调整此处。
var redactPatterns = []string{
	`\b\d{17}[\dXx]\b`,
	`\b\d{16,19}\b`,
	`\b(?:10\.\d{1,3}|192\.168\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3})\.\d{1,3}\b`,
	`(?:sk-|AKIA)[A-Za-z0-9_\-]{12,}`,
	`Bearer\s+[A-Za-z0-9_\-\.]{12,}`,
	`(?:成本价|供货价|内部价|利润|成本)\D*\d+(?:\.\d+)?`,
}

// compiledProtectPatterns 预编译保护模式（包级单例，避免每次 Validate 重复编译）。
var compiledProtectPatterns = mustCompilePatterns(protectPatterns)

// compiledRedactPatterns 预编译脱敏模式（包级单例）。
var compiledRedactPatterns = mustCompilePatterns(redactPatterns)

// redactExcludePatterns 脱敏时需跳过的"强业务令牌"区间（仅 email/URL/SKU）。
//
// 这些模式对应客服回复中必须原样送达的强业务令牌，脱敏（如银行卡/身份证模式）
// 不得误伤其中的长数字。价格不在此列——因为"成本价/内部价/利润"等内部敏感模式
// 需要能够覆盖价格金额（内部成本优先于价格展示）。
var redactExcludePatterns = []string{
	`SKU-[A-Z0-9]{6,}`,
	`https?://\S+`,
	`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`,
}

// compiledRedactExcludePatterns 预编译脱敏排除模式（包级单例）。
var compiledRedactExcludePatterns = mustCompilePatterns(redactExcludePatterns)

// mustCompilePatterns 编译模式列表，任一失败 panic（启动期 fail-fast）。
func mustCompilePatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			panic("i18n: invalid pattern " + p + ": " + err.Error())
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
//   - targetLang ：目标语言（保留参数供未来扩展）
//   - glossary   ：术语视图（可为 nil —— 仅做正则保护与脱敏）
//
// 返回：
//   - 校准后的文本（始终非 nil，可能等于 text）
//   - 校准记录列表（无校准则长度为 0）
//
// 校准顺序：
//  1. 术语校准（glossary_corrected）
//  2. 定位业务令牌保护区间（不改文本）
//  3. 内部敏感真正脱敏（pattern_redacted），排除保护区间
//  4. 业务令牌保护记录（pattern_protected），文本不变
func (v *PostValidator) Validate(text string, targetLang string, glossary *GlossaryView) (string, []ValidationIssue) {
	if text == "" {
		return text, nil
	}

	var issues []ValidationIssue
	out := text

	if glossary != nil && len(glossary.Mappings) > 0 {
		out, issues = v.applyGlossary(out, glossary.Mappings, issues)
	}

	protected := v.findProtectSpans(out, compiledRedactExcludePatterns)

	out, issues = v.applyRedact(out, compiledRedactPatterns, protected, issues)

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

// findProtectSpans 返回所有保护模式在文本中命中的区间 [start,end)。
// 用于脱敏时排除这些区间，避免误伤 URL/email/价格中的长数字。
func (v *PostValidator) findProtectSpans(text string, patterns []*regexp.Regexp) []protectSpan {
	var spans []protectSpan
	for _, re := range patterns {
		locs := re.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			spans = append(spans, protectSpan{start: loc[0], end: loc[1]})
		}
	}
	return spans
}

// protectSpan 保护命中区间。
type protectSpan struct {
	start int
	end   int
}

// spanOverlaps 判断 [s,e) 是否与任一保护区间重叠。
func spanOverlaps(spans []protectSpan, s, e int) bool {
	for _, sp := range spans {
		if s < sp.end && e > sp.start {
			return true
		}
	}
	return false
}

// applyRedact 真正脱敏：命中 redact 模式的片段替换为 [REDACTED]，
// 但落入 protect 区间（业务令牌）的命中跳过（保留原样）。
// 返回脱敏后的文本与脱敏记录（Actual 固定 [REDACTED]，不留存原文）。
func (v *PostValidator) applyRedact(text string, patterns []*regexp.Regexp, protected []protectSpan, issues []ValidationIssue) (string, []ValidationIssue) {
	type hit struct {
		s, e  int
		reStr string
	}
	var hits []hit
	for _, re := range patterns {
		locs := re.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			s, e := loc[0], loc[1]
			if spanOverlaps(protected, s, e) {
				continue
			}
			hits = append(hits, hit{s: s, e: e, reStr: re.String()})
		}
	}
	if len(hits) == 0 {
		return text, issues
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].s < hits[j].s })

	var sb strings.Builder
	last := 0
	for _, h := range hits {
		if h.s < last {
			continue
		}
		sb.WriteString(text[last:h.s])
		sb.WriteString("[REDACTED]")
		issues = append(issues, ValidationIssue{
			Type:     "pattern_redacted",
			Term:     h.reStr,
			Expected: "[REDACTED]",
			Actual:   "[REDACTED]",
		})
		last = h.e
	}
	sb.WriteString(text[last:])
	return sb.String(), issues
}

// compileUserPatterns 编译 glossary 自定义保护模式。
//
// 失败的模式跳过（best-effort），不阻断校准流程。
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
