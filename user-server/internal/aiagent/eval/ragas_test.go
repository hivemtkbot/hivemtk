package eval

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// P1f RAGAS 四指标测试

type fakeLLM struct {
	resp string
	err  error
}

func (f *fakeLLM) Generate(ctx context.Context, config any, prompt string) (string, error) {
	return f.resp, f.err
}

func TestRAGAS_LLMJudgeMode(t *testing.T) {
	ev := NewRAGASEvaluator(&fakeLLM{resp: `{"faithfulness":0.95,"answer_relevancy":0.8,"context_precision":85,"context_recall":0.7,"issues":["minor"]}`})
	rep, err := ev.Evaluate(context.Background(), "Q", "A", []string{"C1"}, "GT")
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if rep.Mode != "llm_judge" {
		t.Errorf("mode = %s, want llm_judge", rep.Mode)
	}
	if rep.Faithfulness != 0.95 || rep.AnswerRelevancy != 0.8 || rep.ContextRecall != 0.7 {
		t.Errorf("scores passthrough wrong: %+v", rep)
	}
	// 0-100 输出自动归一化
	if rep.ContextPrecision < 0.84 || rep.ContextPrecision > 0.86 {
		t.Errorf("context_precision 应归一化为 0.85, got %v", rep.ContextPrecision)
	}
}

func TestRAGAS_LLMFailureDegradesToHeuristic(t *testing.T) {
	ev := NewRAGASEvaluator(&fakeLLM{err: errors.New("boom")})
	rep, err := ev.Evaluate(context.Background(), "怎么退款", "支持7天无理由退款。联系客服即可。", []string{"我们的退款政策是7天无理由退款"}, "")
	if err != nil {
		t.Fatalf("降级路径不应报错: %v", err)
	}
	if rep.Mode != "heuristic" {
		t.Errorf("mode = %s, want heuristic", rep.Mode)
	}
	found := false
	for _, iss := range rep.Issues {
		if strings.Contains(iss, "degraded to heuristic") {
			found = true
		}
	}
	if !found {
		t.Error("降级必须记录 issue 留痕")
	}
}

func TestRAGAS_HeuristicScoringBoundsAndSemantics(t *testing.T) {
	ev := NewRAGASEvaluator(nil) // 无 LLM → 固定启发式

	// 支持性答案：忠实度高、召回可测
	good, err := ev.Evaluate(context.Background(),
		"如何申请退款",
		"您可以申请退款。流程是在订单页点击申请。",
		[]string{"退款政策：订单页点击申请，7天内可退款"},
		"在订单页点击申请，7天有效")
	if err != nil {
		t.Fatal(err)
	}
	if good.Faithfulness <= 0 || good.Faithfulness > 1 {
		t.Errorf("faithfulness 越界: %v", good.Faithfulness)
	}
	if good.ContextRecall <= 0 {
		t.Errorf("有参考答案时 recall 应 >0: %v", good.ContextRecall)
	}
	if !good.MeetsTargets() && good.Faithfulness <= FaithfulnessTarget {
		t.Logf("info: faithful=%.2f precision=%.2f（未达线仅提示不阻断）", good.Faithfulness, good.ContextPrecision)
	}

	// 幻觉答案：上下文完全无关 → 忠实度低
	bad, _ := ev.Evaluate(context.Background(),
		"公司什么时候成立",
		"我们公司成立于1998年，是全球500强。",
		[]string{"本店主营生鲜水果配送"},
		"")
	if bad.Faithfulness >= FaithfulnessTarget {
		t.Errorf("幻觉内容忠实度应低于目标线: %v", bad.Faithfulness)
	}

	// 无参考答案 → recall=-1 显式标注未测量
	nogt, _ := ev.Evaluate(context.Background(), "q", "a", []string{"c"}, "")
	if nogt.ContextRecall != -1 {
		t.Errorf("无 ground truth 时 recall 应为 -1, got %v", nogt.ContextRecall)
	}
	if len(nogt.Issues) == 0 {
		t.Error("未测量必须写 issues")
	}

	// 无上下文 → precision=0
	noCtx, _ := ev.Evaluate(context.Background(), "q", "a", nil, "")
	if noCtx.ContextPrecision != 0 {
		t.Errorf("无上下文 precision 应为 0, got %v", noCtx.ContextPrecision)
	}

	// 排序质量：相关块在前 AP 高于相关块在后
	q := "退货"
	ans := "可以退货"
	relFirst, _ := ev.Evaluate(context.Background(), q, ans, []string{"退货政策说明", "本店水果很甜"}, "")
	relLast, _ := ev.Evaluate(context.Background(), q, ans, []string{"本店水果很甜", "退货政策说明"}, "")
	if relFirst.ContextPrecision <= relLast.ContextPrecision {
		t.Errorf("相关块靠前应得更高 precision: first=%v last=%v", relFirst.ContextPrecision, relLast.ContextPrecision)
	}
}

func TestRAGAS_EmptySampleRejected(t *testing.T) {
	ev := NewRAGASEvaluator(nil)
	if _, err := ev.Evaluate(context.Background(), "", "", nil, ""); err == nil {
		t.Error("空样本应拒绝评估")
	}
}
