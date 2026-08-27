package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// P1f RAGAS 四指标评测集（竞品吸收，见 AI_CORE_COMPETITIVE_ANALYSIS.md）
//
// 指标与目标线（业界 RAGAS 默认标准）：
//   - Faithfulness     忠实度：答案中的论断是否被检索上下文支持（目标 >0.9）
//   - AnswerRelevancy  答案相关性：答案与问题的对齐程度
//   - ContextPrecision 上下文精确率：相关块的排序加权精确率（目标 >0.8）
//   - ContextRecall    上下文召回率：参考答案的信息点被上下文覆盖的比例
//
// 双实现：LLM Judge 模式（结构化四指标一次产出）失败时降级为
// 无外部依赖的启发式模式（字符 bigram 覆盖），Mode 字段显式标注，
// 避免无 LLM 环境"无评测=裸奔"，同时不伪造 LLM 结论。

// RAGAS 目标线常量
const (
	FaithfulnessTarget     = 0.90
	ContextPrecisionTarget = 0.80
)

// RAGASReport 单条样本的四指标报告
type RAGASReport struct {
	Faithfulness     float64  `json:"faithfulness"`
	AnswerRelevancy  float64  `json:"answer_relevancy"`
	ContextPrecision float64  `json:"context_precision"`
	ContextRecall    float64  `json:"context_recall"` // -1 表示无参考答案未测量
	Issues           []string `json:"issues,omitempty"`
	// Mode 评分来源："llm_judge" / "heuristic"
	Mode string `json:"mode"`
}

// MeetsTargets 是否达到目标线（忠实度>0.9 且 上下文精确率>0.8）
func (r *RAGASReport) MeetsTargets() bool {
	return r.Faithfulness > FaithfulnessTarget && r.ContextPrecision > ContextPrecisionTarget
}

// RAGASEvaluator RAGAS 评估器。
// llmService 为 nil 或被禁用时直接走启发式模式。
type RAGASEvaluator struct {
	llmService LLMServiceInterface
	enabled    bool
}

// NewRAGASEvaluator 创建 RAGAS 评估器。llmService 为 nil 时仅启发式可用。
func NewRAGASEvaluator(llmService LLMServiceInterface) *RAGASEvaluator {
	return &RAGASEvaluator{llmService: llmService, enabled: true}
}

// SetEnabled 启用/禁用 LLM 评分（禁用后固定走启发式）。
func (e *RAGASEvaluator) SetEnabled(enabled bool) {
	e.enabled = enabled
}

// Evaluate 对单条 RAG 样本产出四指标报告。
//
// groundTruth 为空时 ContextRecall 无法计算，置 -1 并记入 Issues
// （区别于真实的 0 分）。LLM 失败降级启发式并记录 Mode。
func (e *RAGASEvaluator) Evaluate(ctx context.Context, query, answer string, contexts []string, groundTruth string) (*RAGASReport, error) {
	if strings.TrimSpace(query) == "" && strings.TrimSpace(answer) == "" && len(contexts) == 0 {
		return nil, fmt.Errorf("ragas: empty sample")
	}
	if e.enabled && e.llmService != nil {
		rep, err := e.evaluateWithLLM(ctx, query, answer, contexts, groundTruth)
		if err == nil {
			return rep, nil
		}
		rep = evaluateHeuristic(query, answer, contexts, groundTruth)
		rep.Mode = "heuristic"
		rep.Issues = append(rep.Issues, fmt.Sprintf("llm judge failed, degraded to heuristic: %v", err))
		return rep, nil
	}
	return evaluateHeuristic(query, answer, contexts, groundTruth), nil
}

// ragasRawScores LLM 返回的原始分数结构
type ragasRawScores struct {
	Faithfulness     float64  `json:"faithfulness"`
	AnswerRelevancy  float64  `json:"answer_relevancy"`
	ContextPrecision float64  `json:"context_precision"`
	ContextRecall    float64  `json:"context_recall"`
	Issues           []string `json:"issues"`
}

func (e *RAGASEvaluator) buildPrompt(query, answer string, contexts []string, groundTruth string) string {
	var sb strings.Builder
	sb.WriteString("You are a strict RAG quality evaluator (RAGAS framework).\n\n")
	sb.WriteString(fmt.Sprintf("Question: %s\n\nAnswer: %s\n\n", query, answer))
	sb.WriteString(fmt.Sprintf("Retrieved contexts (%d):\n", len(contexts)))
	for i, c := range contexts {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, truncateForEval(c, 300)))
	}
	if groundTruth != "" {
		sb.WriteString(fmt.Sprintf("\nGround truth reference: %s\n", groundTruth))
	}
	sb.WriteString(`
Score each metric from 0.0 to 1.0:
- faithfulness: every claim in the answer must be supported by the retrieved contexts
- answer_relevancy: how directly the answer addresses the question
- context_precision: proportion of relevant contexts and whether they are ranked early
- context_recall: coverage of ground-truth information by the contexts (use 0 if no ground truth)

Return ONLY a JSON object (no markdown):
{"faithfulness":0.0,"answer_relevancy":0.0,"context_precision":0.0,"context_recall":0.0,"issues":["..."]}`)
	return sb.String()
}

func (e *RAGASEvaluator) evaluateWithLLM(ctx context.Context, query, answer string, contexts []string, groundTruth string) (*RAGASReport, error) {
	resp, err := e.llmService.Generate(ctx, nil, e.buildPrompt(query, answer, contexts, groundTruth))
	if err != nil {
		return nil, fmt.Errorf("ragas: generate failed: %w", err)
	}
	jsonStr := extractJSON(resp)
	if jsonStr == "" {
		return nil, fmt.Errorf("ragas: no JSON object found in response")
	}
	var raw ragasRawScores
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("ragas: parse json failed: %w", err)
	}
	return &RAGASReport{
		Faithfulness:     normalizeScore(raw.Faithfulness),
		AnswerRelevancy:  normalizeScore(raw.AnswerRelevancy),
		ContextPrecision: normalizeScore(raw.ContextPrecision),
		ContextRecall:    normalizeScore(raw.ContextRecall),
		Issues:           raw.Issues,
		Mode:             "llm_judge",
	}, nil
}

// ---------- 启发式模式（零外部依赖，纯函数可测） ----------

func evaluateHeuristic(query, answer string, contexts []string, groundTruth string) *RAGASReport {
	rep := &RAGASReport{Mode: "heuristic"}

	ansGrams := contentBigrams(answer)
	qGrams := contentBigrams(query)

	// Faithfulness：答案句子中被任一上下文支持（共享内容 bigram）的比例
	sentences := splitSentences(answer)
	if len(sentences) > 0 {
		supported := 0
		for _, s := range sentences {
			grams := contentBigrams(s)
			for _, ctx := range contexts {
				if gramsOverlap(grams, contentBigrams(ctx)) {
					supported++
					break
				}
			}
		}
		rep.Faithfulness = float64(supported) / float64(len(sentences))
	} else {
		rep.Faithfulness = 1 // 无答案无从失实
	}

	// AnswerRelevancy：问题与答案的内容 bigram 重合度
	if len(qGrams) > 0 {
		inter := 0
		for g := range qGrams {
			if _, ok := ansGrams[g]; ok {
				inter++
			}
		}
		rep.AnswerRelevancy = float64(inter) / float64(len(qGrams))
	} else {
		rep.AnswerRelevancy = 1
	}

	// ContextPrecision：标准 Average Precision —— 相关块越靠前且不掺噪声分越高
	if len(contexts) > 0 {
		var hits, apSum float64
		for i, c := range contexts {
			if gramsOverlap(contentBigrams(c), qGrams) {
				hits++
				apSum += hits / float64(i+1)
			}
		}
		if hits > 0 {
			rep.ContextPrecision = apSum / float64(hits)
		}
	} else {
		rep.Issues = append(rep.Issues, "no contexts provided")
	}

	// ContextRecall：参考答案句子被上下文覆盖的比例；无参考答案记 -1（未测量）
	if strings.TrimSpace(groundTruth) == "" {
		rep.ContextRecall = -1
		rep.Issues = append(rep.Issues, "context_recall not measurable without ground truth")
	} else {
		gtSentences := splitSentences(groundTruth)
		if len(gtSentences) > 0 {
			covered := 0
			for _, s := range gtSentences {
				grams := contentBigrams(s)
				for _, ctx := range contexts {
					if gramsOverlap(grams, contentBigrams(ctx)) {
						covered++
						break
					}
				}
			}
			rep.ContextRecall = float64(covered) / float64(len(gtSentences))
		}
	}
	return rep
}

// normalizeScore 钳位到 [0,1]，兼容 0-100 打分的模型输出
func normalizeScore(v float64) float64 {
	if v > 1 {
		v = v / 100
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// contentBigrams 提取内容字符 bigram（过滤空白与常见标点，中英文通用）
func contentBigrams(s string) map[string]struct{} {
	runes := make([]rune, 0, len(s))
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '\t', '\n', '\r', '，', '。', '！', '？', ',', '.', '!', '?',
			';', ':', '；', '：', '"', '\'', '(', ')', '（', '）':
			continue
		}
		runes = append(runes, r)
	}
	out := make(map[string]struct{})
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = struct{}{}
	}
	return out
}

func gramsOverlap(a, b map[string]struct{}) bool {
	small, large := a, b
	if len(large) < len(small) {
		small, large = large, small
	}
	for g := range small {
		if _, ok := large[g]; ok {
			return true
		}
	}
	return false
}

// splitSentences 按中英文句读切分句子
func splitSentences(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '。', '！', '？', '；', '.', '!', '?', ';', '\n':
			return true
		}
		return false
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func truncateForEval(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}
