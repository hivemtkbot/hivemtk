// Package eval 提供多语言生成质量评估能力。
//
// 本包实现两类评估指标：
//   - ChrFEvaluator：chrF++ 字符/词 n-gram F-beta 评估（纯 Go，无外部依赖）
//   - LLMJudge      ：基于另一个 LLM 的多维质量评分（fluency/accuracy/terminology/tone）
//
// 五层架构归属：L3 aiagent 内部工具包。不依赖 service / repository，
// 可被 service/translation 等上层服务组合使用。
package eval

import "strings"

// ChrFEvaluator chrF++ 评估器。无状态、可并发调用。
type ChrFEvaluator struct {
	CharN int
	WordN int
	Beta  float64
}

// NewChrFEvaluator 创建带默认参数的 chrF++ 评估器。
func NewChrFEvaluator() *ChrFEvaluator {
	return &ChrFEvaluator{CharN: 6, WordN: 2, Beta: 2.0}
}

// Score 计算单条候选译文与参考译文的 chrF++ 分数（范围 0.0 ~ 1.0）。
//
// 当 candidate 或 reference 为空时返回 0。
func (e *ChrFEvaluator) Score(candidate, reference string) float64 {
	if candidate == "" || reference == "" {
		return 0.0
	}

	charN, wordN, beta := e.effectiveParams()

	var sum float64
	var count int

	for n := 1; n <= charN; n++ {
		candNG := e.charNgrams(candidate, n)
		refNG := e.charNgrams(reference, n)
		if len(candNG) == 0 && len(refNG) == 0 {
			continue
		}
		p, r := e.ngramPrecision(candNG, refNG)
		sum += e.fScore(p, r, beta)
		count++
	}

	for n := 1; n <= wordN; n++ {
		candNG := e.wordNgrams(candidate, n)
		refNG := e.wordNgrams(reference, n)
		if len(candNG) == 0 && len(refNG) == 0 {
			continue
		}
		p, r := e.ngramPrecision(candNG, refNG)
		sum += e.fScore(p, r, beta)
		count++
	}

	if count == 0 {
		return 0.0
	}
	return sum / float64(count)
}

// ScoreBatch 批量计算，返回平均 chrF++ 分数。
//
// candidates 与 references 等长（不等长时取较小者，避免越界）。
// 空切片返回 0。
func (e *ChrFEvaluator) ScoreBatch(candidates, references []string) float64 {
	if len(candidates) == 0 || len(references) == 0 {
		return 0.0
	}
	n := len(candidates)
	if len(references) < n {
		n = len(references)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += e.Score(candidates[i], references[i])
	}
	return sum / float64(n)
}

func (e *ChrFEvaluator) effectiveParams() (int, int, float64) {
	charN := e.CharN
	if charN < 1 {
		charN = 6
	}
	wordN := e.WordN
	if wordN < 1 {
		wordN = 2
	}
	beta := e.Beta
	if beta <= 0 {
		beta = 2.0
	}
	return charN, wordN, beta
}

func (e *ChrFEvaluator) charNgrams(text string, n int) map[string]int {
	if text == "" {
		return nil
	}
	normalized := strings.Join(strings.Fields(text), " ")
	normalized = " " + normalized + " "
	runes := []rune(normalized)
	if len(runes) < n {
		return nil
	}
	out := make(map[string]int)
	for i := 0; i+n <= len(runes); i++ {
		gram := string(runes[i : i+n])
		out[gram]++
	}
	return out
}

func (e *ChrFEvaluator) wordNgrams(text string, n int) map[string]int {
	words := strings.Fields(text)
	if len(words) < n {
		return nil
	}
	out := make(map[string]int)
	for i := 0; i+n <= len(words); i++ {
		gram := strings.Join(words[i:i+n], " ")
		out[gram]++
	}
	return out
}

func (e *ChrFEvaluator) ngramPrecision(cand, ref map[string]int) (float64, float64) {
	if len(cand) == 0 || len(ref) == 0 {
		return 0.0, 0.0
	}
	var candTotal, refTotal, overlap int
	for g, c := range cand {
		candTotal += c
		if r, ok := ref[g]; ok {
			if r < c {
				overlap += r
			} else {
				overlap += c
			}
		}
	}
	for _, r := range ref {
		refTotal += r
	}
	var p, r float64
	if candTotal > 0 {
		p = float64(overlap) / float64(candTotal)
	}
	if refTotal > 0 {
		r = float64(overlap) / float64(refTotal)
	}
	return p, r
}

func (e *ChrFEvaluator) fScore(p, r, beta float64) float64 {
	if p+r == 0 {
		return 0.0
	}
	beta2 := beta * beta
	return (1 + beta2) * (p * r) / (beta2*p + r)
}
