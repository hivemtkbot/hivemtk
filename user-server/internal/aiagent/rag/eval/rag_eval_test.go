package eval

import "testing"

// ContextRecallAtK 手工数值验证：
// gold="退款到账" tokens：单字{退,款,到,账}4 + 双字{退款,款到,到账}3，共 7
// top1 片段"退款流程说明" 命中 {退,款,退款} = 3/7
// top2 追加"到账时间说明" 命中 +{到,账,到账} = 6/7
func TestContextRecallAtK(t *testing.T) {
	ctx := []string{"退款流程说明", "到账时间说明"}
	assertNear(t, "top1", ContextRecallAtK("退款到账", ctx, 1), 3.0/7.0)
	assertNear(t, "top2", ContextRecallAtK("退款到账", ctx, 2), 6.0/7.0)
	assertNear(t, "k超界退化全量", ContextRecallAtK("退款到账", ctx, 10), 6.0/7.0)
	assertNear(t, "k为0", ContextRecallAtK("退款到账", ctx, 0), 0)
	assertNear(t, "k为负", ContextRecallAtK("退款到账", ctx, -1), 0)
	assertNear(t, "空真值", ContextRecallAtK("", ctx, 1), 0)
	assertNear(t, "空片段", ContextRecallAtK("退款到账", nil, 3), 0)
}

// AnswerRelevance（bigram Jaccard）手工验证：
// query="退款" bigram={退款}；answer="退款已到账" bigram={退款,款已,已到,到账}
// 交=1 并=4 → 0.25；全同=1；无交集=0；单字无 bigram 或空串=0
func TestAnswerRelevanceJaccard(t *testing.T) {
	assertNear(t, "手算值", AnswerRelevance("退款", "退款已到账"), 0.25)
	assertNear(t, "全同", AnswerRelevance("已退款", "已退款"), 1)
	assertNear(t, "零交集", AnswerRelevance("退款流程", "发票邮寄"), 0)
	assertNear(t, "单字无bigram", AnswerRelevance("退", "退款已到账"), 0)
	assertNear(t, "空答案", AnswerRelevance("退款流程", ""), 0)
}

// RunRAGEval 汇总：均值与逐条均值一致，空输入零值不 panic
func TestRunRAGEvalSummary(t *testing.T) {
	empty := RunRAGEval(nil)
	if len(empty.Cases) != 0 || empty.AvgFaithfulness != 0 || empty.AvgContextRecallAtK != 0 || empty.AvgAnswerRelevance != 0 {
		t.Fatalf("空用例表应得零摘要: %+v", empty)
	}

	shared := "质量问题退货运费商家承担"
	ok := EvalCase{Question: shared, GroundTruth: shared, Answer: shared, Contexts: []string{shared, "备用片段"}}
	bad := EvalCase{Question: "无关问题", GroundTruth: "", Answer: "", Contexts: nil}
	sum := RunRAGEval([]EvalCase{ok, bad})
	if len(sum.Cases) != 2 || sum.Cases[0].Index != 0 || sum.Cases[1].Index != 1 {
		t.Fatalf("明细数或序号错误: %+v", sum.Cases)
	}
	assertNear(t, "AvgF", sum.AvgFaithfulness, 0.5)
	assertNear(t, "AvgR@K", sum.AvgContextRecallAtK, 0.5)
	assertNear(t, "AvgA", sum.AvgAnswerRelevance, 0.5)

	var f, r, a float64
	for _, d := range sum.Cases {
		f += d.Faithfulness
		r += d.ContextRecallAtK
		a += d.AnswerRelevance
	}
	n := float64(len(sum.Cases))
	assertNear(t, "明细均值F", f/n, sum.AvgFaithfulness)
	assertNear(t, "明细均值R", r/n, sum.AvgContextRecallAtK)
	assertNear(t, "明细均值A", a/n, sum.AvgAnswerRelevance)
}

// 随包 testdata 黄金集自检：10 条中文客服占位样例，字段完整且三指标均落在 [0,1]
func TestRAGEvalGoldenFile(t *testing.T) {
	cases, err := LoadGoldenSet("testdata/rag_golden.json")
	if err != nil {
		t.Fatalf("加载 testdata 黄金集失败: %v", err)
	}
	if len(cases) != 10 {
		t.Fatalf("黄金集用例数=%d, want 10", len(cases))
	}
	for i, c := range cases {
		if c.Question == "" || c.GroundTruth == "" || c.Answer == "" || len(c.Contexts) < 2 {
			t.Fatalf("第%d条字段不完整: %+v", i, c)
		}
	}
	sum := RunRAGEval(cases)
	for _, d := range sum.Cases {
		for name, v := range map[string]float64{
			"faithfulness":     d.Faithfulness,
			"context_recall_k": d.ContextRecallAtK,
			"answer_relevance": d.AnswerRelevance,
		} {
			if v < 0 || v > 1 {
				t.Fatalf("第%d条 %s=%.4f 越界 [0,1]", d.Index, name, v)
			}
		}
	}
	if sum.AvgContextRecallAtK <= 0 {
		t.Fatalf("占位黄金集召回率应大于 0: %.4f", sum.AvgContextRecallAtK)
	}
}
