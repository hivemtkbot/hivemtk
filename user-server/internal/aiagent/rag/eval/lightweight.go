// Package eval 提供无向量依赖的轻量 RAG 评测指标（RAGAS-lite）。
//
// 适用场景：无法稳定接入 embedding 服务时的回归评测与 CI 冒烟。
// 所有指标均为启发式近似值：
//   - 中文分词采用「单字 + 相邻双字滑窗」的 bag-of-tokens 近似；
//   - 分句按中英文常见终止标点切分；
//   - 结果只用于相对比较（版本间回归观察），不可视为绝对语义质量分。
package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// EvalCase 单条评测样例
type EvalCase struct {
	Question    string   `json:"question"`     // 用户问题
	GroundTruth string   `json:"ground_truth"` // 标准答案（参考真值）
	Answer      string   `json:"answer"`       // 待评系统回答
	Contexts    []string `json:"contexts"`     // 召回上下文片段
}

// CaseResult 单条样例的明细得分
type CaseResult struct {
	Index           int     `json:"index"`
	Question        string  `json:"question"`
	Faithfulness    float64 `json:"faithfulness"`
	ContextRecall   float64 `json:"context_recall"`
	AnswerRelevance float64 `json:"answer_relevance"`
}

// EvalReport 全量评测报告：三指标均值 + 每条明细
type EvalReport struct {
	Cases              []CaseResult `json:"cases"`
	AvgFaithfulness    float64      `json:"avg_faithfulness"`
	AvgContextRecall   float64      `json:"avg_context_recall"`
	AvgAnswerRelevance float64      `json:"avg_answer_relevance"`
}

// splitSentencesLite 轻量分句：按中英文终止标点与换行切分。
// 已知近似：数字小数点（如 "3.5"）会被切开，评测场景可接受。
func splitSentencesLite(s string) []string {
	f := func(r rune) bool {
		switch r {
		case '。', '！', '？', '；', '、', '!', '?', ';', '.', '\n', '\r':
			return true
		}
		return false
	}
	parts := strings.FieldsFunc(s, f)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// tokenizeLite 中文轻量分词：小写归一后的「单字 + 相邻双字滑窗」，跳过空白字符。
// 返回原始 token 序列（上层以集合语义使用）。
func tokenizeLite(s string) []string {
	runes := []rune(strings.ToLower(s))
	tokens := make([]string, 0, len(runes)*2)
	for _, r := range runes {
		if !unicode.IsSpace(r) {
			tokens = append(tokens, string(r))
		}
	}
	for i := 0; i+1 < len(runes); i++ {
		if !unicode.IsSpace(runes[i]) && !unicode.IsSpace(runes[i+1]) {
			tokens = append(tokens, string(runes[i:i+2]))
		}
	}
	return tokens
}

func tokenSet(ss ...string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, s := range ss {
		for _, t := range tokenizeLite(s) {
			set[t] = struct{}{}
		}
	}
	return set
}

// coverageRatio 计算 target token 相对基准集合的覆盖率
func coverageRatio(targetTokens []string, base map[string]struct{}) float64 {
	var hit, total int
	for _, t := range targetTokens {
		total++
		if _, ok := base[t]; ok {
			hit++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// FaithfulnessLite 忠实度：答案逐分句后，计算每个分句的 token 被 contexts 并集覆盖率，取均值。
// 空答案或空 contexts 视为无从验证，返回 0。
func FaithfulnessLite(answer string, contexts []string) float64 {
	if strings.TrimSpace(answer) == "" || len(contexts) == 0 {
		return 0
	}
	base := tokenSet(contexts...)
	sentences := splitSentencesLite(answer)
	if len(sentences) == 0 {
		return 0
	}
	sum := 0.0
	for _, sent := range sentences {
		sum += coverageRatio(tokenizeLite(sent), base)
	}
	return sum / float64(len(sentences))
}

// ContextRecallLite 上下文召回率：groundTruth 分句后，逐句计算其 token 被 contexts
// 并集覆盖的比例，再对全部句子取算术平均（软召回；句子被完整复述时得满分 1，
// 与上下文完全无交集的句子计 0 分）。采用软语义的原因：黄金集真值为人工撰写
// 的改述文案，与召回片段极少逐字一致，硬性「整句全覆盖」判据会把正常情况
// 全部误杀为 0 分。question 参数保留以对齐 RAGAS 接口形态，当前不参与计算。
func ContextRecallLite(question string, contexts []string, groundTruth string) float64 {
	_ = question
	if strings.TrimSpace(groundTruth) == "" || len(contexts) == 0 {
		return 0
	}
	base := tokenSet(contexts...)
	sentences := splitSentencesLite(groundTruth)
	if len(sentences) == 0 {
		return 0
	}
	sum := 0.0
	for _, sent := range sentences {
		sum += coverageRatio(tokenizeLite(sent), base)
	}
	return sum / float64(len(sentences))
}

// AnswerRelevanceLite 回答相关性：问题与答案的字符二元组（bigram）集合 Dice 系数。
// 任一方不足两个有效字符（无法产生 bigram）时返回 0。
func AnswerRelevanceLite(question string, answer string) float64 {
	q := bigramSet(question)
	a := bigramSet(answer)
	if len(q) == 0 || len(a) == 0 {
		return 0
	}
	inter := 0
	for t := range q {
		if _, ok := a[t]; ok {
			inter++
		}
	}
	den := len(q) + len(a)
	if den == 0 {
		return 0
	}
	return 2 * float64(inter) / float64(den)
}

// bigramSet 仅提取相邻双字滑窗（忽略空白），用于 Dice 计算
func bigramSet(s string) map[string]struct{} {
	runes := []rune(strings.ToLower(s))
	set := make(map[string]struct{})
	for i := 0; i+1 < len(runes); i++ {
		if unicode.IsSpace(runes[i]) || unicode.IsSpace(runes[i+1]) {
			continue
		}
		set[string(runes[i:i+2])] = struct{}{}
	}
	return set
}

// RunEval 批量执行评测并汇总均值与每条明细
func RunEval(cases []EvalCase) EvalReport {
	report := EvalReport{Cases: make([]CaseResult, 0, len(cases))}
	if len(cases) == 0 {
		return report
	}
	for i, c := range cases {
		cr := CaseResult{
			Index:           i,
			Question:        c.Question,
			Faithfulness:    FaithfulnessLite(c.Answer, c.Contexts),
			ContextRecall:   ContextRecallLite(c.Question, c.Contexts, c.GroundTruth),
			AnswerRelevance: AnswerRelevanceLite(c.Question, c.Answer),
		}
		report.Cases = append(report.Cases, cr)
		report.AvgFaithfulness += cr.Faithfulness
		report.AvgContextRecall += cr.ContextRecall
		report.AvgAnswerRelevance += cr.AnswerRelevance
	}
	n := float64(len(cases))
	report.AvgFaithfulness /= n
	report.AvgContextRecall /= n
	report.AvgAnswerRelevance /= n
	return report
}

// LoadGoldenSet 从 JSON 文件加载黄金评测集，容错解析：
//   - 支持顶层数组 [...] 与对象包装 {"cases":[...]} 两种格式；
//   - 缺失字段安全降级为空串；contexts 为 null 时转为空切片；
//   - 无法解析时报错且不产生部分状态。
func LoadGoldenSet(path string) ([]EvalCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取黄金集文件失败: %w", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	type wrapper struct {
		Cases []EvalCase `json:"cases"`
	}
	switch {
	case strings.HasPrefix(trimmed, "["):
		var cases []EvalCase
		if err := json.Unmarshal([]byte(trimmed), &cases); err != nil {
			return nil, fmt.Errorf("解析黄金集数组失败: %w", err)
		}
		return normalizeCases(cases), nil
	case strings.HasPrefix(trimmed, "{"):
		var w wrapper
		if err := json.Unmarshal([]byte(trimmed), &w); err != nil {
			return nil, fmt.Errorf("解析黄金集对象失败: %w", err)
		}
		return normalizeCases(w.Cases), nil
	default:
		return nil, fmt.Errorf("黄金集文件既非 JSON 数组也非 JSON 对象")
	}
}

// normalizeCases 保证 Contexts 非 nil，便于调用方直接 range 使用
func normalizeCases(cases []EvalCase) []EvalCase {
	out := make([]EvalCase, 0, len(cases))
	for _, c := range cases {
		if c.Contexts == nil {
			c.Contexts = []string{}
		}
		out = append(out, c)
	}
	return out
}
