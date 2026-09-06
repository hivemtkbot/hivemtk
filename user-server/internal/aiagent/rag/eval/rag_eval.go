// rag_eval.go 提供 RAG 评测的增量口径（R-4），与 lightweight.go 互补：
//   - ContextRecallAtK：仅以 top-k 召回片段为基准的软召回，衡量排序列表头部命中；
//   - AnswerRelevance：问题-答案字符二元组（bigram）Jaccard 相似度；
//   - RunRAGEval：批量评测输出「三指标均值 + 逐条明细」摘要。
//
// 均为纯函数，复用 lightweight.go 的 EvalCase / FaithfulnessLite / ContextRecallLite /
// bigramSet 实现，无外部依赖，可安全用于 CI 冒烟。
package eval

const ragEvalRecallK = 3

// RAGEvalCaseDetail 单条样例在 RAG 口径下的明细得分
type RAGEvalCaseDetail struct {
	Index            int     `json:"index"`
	Query            string  `json:"query"`
	Faithfulness     float64 `json:"faithfulness"`
	ContextRecallAtK float64 `json:"context_recall_at_k"`
	AnswerRelevance  float64 `json:"answer_relevance"`
}

// RAGEvalSummary 批量评测摘要：三指标均值 + 逐条明细
type RAGEvalSummary struct {
	Cases               []RAGEvalCaseDetail `json:"cases"`
	AvgFaithfulness     float64             `json:"avg_faithfulness"`
	AvgContextRecallAtK float64             `json:"avg_context_recall_at_k"`
	AvgAnswerRelevance  float64             `json:"avg_answer_relevance"`
}

// ContextRecallAtK 仅取前 k 条召回片段为基准计算软召回（复用 ContextRecallLite）。
// k<=0 返回 0；k 超出片段数时退化为全量召回。
func ContextRecallAtK(goldAnswer string, contexts []string, k int) float64 {
	if k <= 0 {
		return 0
	}
	if k < len(contexts) {
		contexts = contexts[:k]
	}
	return ContextRecallLite("", contexts, goldAnswer)
}

// AnswerRelevance 问题与答案的 bigram Jaccard 相似度（交集/并集）。
// 与 AnswerRelevanceLite（Dice）互补：Jaccard 对偶发重合更严格。
// 任一方不足两个有效字符（无法产生 bigram）时返回 0。
func AnswerRelevance(query string, answer string) float64 {
	q := bigramSet(query)
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
	union := len(q) + len(a) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// RunRAGEval 按 RAG 口径批量评测：忠实度 + top-k 软召回 + bigram Jaccard 相关性。
// 空用例表返回零值摘要（均值为 0，无 NaN）。
func RunRAGEval(cases []EvalCase) RAGEvalSummary {
	summary := RAGEvalSummary{Cases: make([]RAGEvalCaseDetail, 0, len(cases))}
	for i, c := range cases {
		detail := RAGEvalCaseDetail{
			Index:            i,
			Query:            c.Question,
			Faithfulness:     FaithfulnessLite(c.Answer, c.Contexts),
			ContextRecallAtK: ContextRecallAtK(c.GroundTruth, c.Contexts, ragEvalRecallK),
			AnswerRelevance:  AnswerRelevance(c.Question, c.Answer),
		}
		summary.Cases = append(summary.Cases, detail)
		summary.AvgFaithfulness += detail.Faithfulness
		summary.AvgContextRecallAtK += detail.ContextRecallAtK
		summary.AvgAnswerRelevance += detail.AnswerRelevance
	}
	if n := float64(len(cases)); n > 0 {
		summary.AvgFaithfulness /= n
		summary.AvgContextRecallAtK /= n
		summary.AvgAnswerRelevance /= n
	}
	return summary
}
