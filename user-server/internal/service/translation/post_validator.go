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

var protectPatterns = []string{
	`SKU-[A-Z0-9]{6,}`,
	`[\$€¥£₩]\d+\.?\d*`,
	`https?://\S+`,
	`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`,
}

var redactPatterns = []string{
	`\b\d{17}[\dXx]\b`,
	`\b\d{16,19}\b`,
	`\b(?:10\.\d{1,3}|192\.168\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3})\.\d{1,3}\b`,
	`(?:sk-|AKIA)[A-Za-z0-9_\-]{12,}`,
	`Bearer\s+[A-Za-z0-9_\-\.]{12,}`,
	`(?:成本价|供货价|内部价|利润|成本)\D*\d+(?:\.\d+)?`,
}

var compiledProtectPatterns = mustCompilePatterns(protectPatterns)

var compiledRedactPatterns = mustCompilePatterns(redactPatterns)

var redactExcludePatterns = []string{
	`SKU-[A-Z0-9]{6,}`,
	`https?://\S+`,
	`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`,
}

var compiledRedactExcludePatterns = mustCompilePatterns(redactExcludePatterns)

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

type protectSpan struct {
	start int
	end   int
}

func spanOverlaps(spans []protectSpan, s, e int) bool {
	for _, sp := range spans {
		if s < sp.end && e > sp.start {
			return true
		}
	}
	return false
}

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
