package eval

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const epsEval = 1e-9

func assertNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > epsEval {
		t.Fatalf("%s = %.10f, want %.10f", name, got, want)
	}
}

// 分句边界：中英文终止标点、空串、无标点整句、纯标点过滤
func TestSplitSentencesLite(t *testing.T) {
	if got := splitSentencesLite(""); len(got) != 0 {
		t.Fatalf("空串应得 0 句: %v", got)
	}
	got := splitSentencesLite("质量不错。发货快！会回购？谢谢服务；再看下售后!")
	want := []string{"质量不错", "发货快", "会回购", "谢谢服务", "再看下售后"}
	if len(got) != len(want) {
		t.Fatalf("分句数=%d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第%d句=%q, want %q", i, got[i], want[i])
		}
	}
	if got := splitSentencesLite("没有标点的整句话"); len(got) != 1 || got[0] != "没有标点的整句话" {
		t.Fatalf("无标点应整体成句: %v", got)
	}
}

// 分词边界：大小写归一、跳过空白、滑窗不跨空格、ASCII 兼容
func TestTokenizeLiteBoundary(t *testing.T) {
	toks := tokenizeLite("AB C 你 好")
	set := map[string]int{}
	for _, tk := range toks {
		set[tk]++
	}
	if set["a"] != 1 || set["b"] != 1 || set["你"] != 1 || set["好"] != 1 {
		t.Fatalf("单字归一失败: %v", set)
	}
	if _, ok := set["b c"]; ok {
		t.Fatalf("滑窗不得跨空白: %v", set)
	}
	neighbour := tokenizeLite("你好")
	foundBigram := false
	for _, tk := range neighbour {
		if tk == "你好" {
			foundBigram = true
		}
	}
	if !foundBigram {
		t.Fatalf("相邻汉字应有双字滑窗: %v", neighbour)
	}
}

// FaithfulnessLite 手工数值验证：
// ctx="机器学习很好"，ans="机器学习很有趣。"
// 答案句 tokens：单字7个{机,器,学,习,很,有,趣}（命 中5）+ 双字6个{机器,器学,学习,习很,很有,有趣}（命中4）
// 覆盖率 = 9/13
func TestFaithfulnessLite(t *testing.T) {
	assertNear(t, "完美得分", FaithfulnessLite("杯盖漏水。检查密封圈。", []string{"杯盖漏水需检查密封圈是否老化"}), 1)
	assertNear(t, "手工值", FaithfulnessLite("机器学习很有趣。", []string{"机器学习很好"}), 9.0/13.0)
	assertNear(t, "空答案", FaithfulnessLite("", []string{"任意上下文"}), 0)
	assertNear(t, "空上下文", FaithfulnessLite("任意回答", nil), 0)
	assertNear(t, "纯空白", FaithfulnessLite("   ", []string{"任意上下文"}), 0)
}

// ContextRecallLite（软召回）：满句覆盖=1；含一句全不相干句子时=(1+0)/2=0.5；
// 两句均无交集=0；空输入回退 0。
func TestContextRecallLite(t *testing.T) {
	ctx := []string{"甲句内容原样出现"}
	assertNear(t, "完全匹配", ContextRecallLite("问题", ctx, "甲句内容原样出现。"), 1)
	assertNear(t, "半数覆盖", ContextRecallLite("问题", ctx, "甲句内容原样出现。囧異體字形有禮皆缺。"), 0.5)
	assertNear(t, "零覆盖", ContextRecallLite("问题", ctx, "囧異體字形。有禮皆缺字樣。"), 0)
	assertNear(t, "空真值", ContextRecallLite("问题", ctx, ""), 0)
	assertNear(t, "空上下文", ContextRecallLite("问题", nil, "甲句。"), 0)
}

// AnswerRelevanceLite：相同文本 Dice=1；无交集=0；短于两字=0
func TestAnswerRelevanceLite(t *testing.T) {
	assertNear(t, "全同", AnswerRelevanceLite("保温杯漏水怎么处理", "保温杯漏水怎么处理"), 1)
	assertNear(t, "零交集", AnswerRelevanceLite("苹果手机很好用", "香蕉牛奶真好喝"), 0)
	assertNear(t, "短输入问", AnswerRelevanceLite("好", "很好很好的商品呀"), 0)
	assertNear(t, "短输入答", AnswerRelevanceLite("质量问题如何退货退款", "嗯"), 0)
	assertNear(t, "空字符串", AnswerRelevanceLite("", ""), 0)
}

// RunEval 汇总：均值与明细一致，空输入报告不 panic 且均值为 0
func TestRunEval(t *testing.T) {
	empty := RunEval(nil)
	if len(empty.Cases) != 0 || empty.AvgFaithfulness != 0 || empty.AvgContextRecall != 0 || empty.AvgAnswerRelevance != 0 {
		t.Fatalf("空用例表应得零报告: %+v", empty)
	}

	shared := "质量问题的退货运费由商家承担"
	perfect := EvalCase{
		Question:    shared,
		GroundTruth: shared,
		Answer:      shared,
		Contexts:    []string{shared},
	}
	broken := EvalCase{Question: "什么也没说", GroundTruth: "", Answer: "", Contexts: nil}
	rep := RunEval([]EvalCase{perfect, broken})
	if len(rep.Cases) != 2 {
		t.Fatalf("明细数=%d, want 2", len(rep.Cases))
	}
	assertNear(t, "AvgF", rep.AvgFaithfulness, 0.5)
	assertNear(t, "AvgR", rep.AvgContextRecall, 0.5)
	assertNear(t, "AvgA", rep.AvgAnswerRelevance, 0.5)
	if rep.Cases[0].Index != 0 || rep.Cases[1].Index != 1 {
		t.Fatalf("明细序号错误: %+v", rep.Cases)
	}
}

// LoadGoldenSet 容错解析：数组形式 / 对象包装形式 / null contexts / 非法 JSON 报错
func TestLoadGoldenSetTolerance(t *testing.T) {
	dir := t.TempDir()

	arrayForm := `[{"question":"q1","contexts":null,"unknown_extra":{"x":1}},{"ground_truth":"gt2"}]`
	p1 := filepath.Join(dir, "arr.json")
	mustWrite(t, p1, arrayForm)
	cases, err := LoadGoldenSet(p1)
	if err != nil {
		t.Fatalf("数组形式解析失败: %v", err)
	}
	if len(cases) != 2 || cases[0].Question != "q1" || cases[1].GroundTruth != "gt2" {
		t.Fatalf("数组形式结果错误: %+v", cases)
	}
	if cases[0].Contexts == nil {
		t.Fatalf("null contexts 应归一化为空切片")
	}

	wrappedForm := `{"note":"wrapper","cases":[{"question":"w1","answer":"a1"}]}`
	p2 := filepath.Join(dir, "wrap.json")
	mustWrite(t, p2, wrappedForm)
	cases2, err := LoadGoldenSet(p2)
	if err != nil {
		t.Fatalf("对象包装解析失败: %v", err)
	}
	if len(cases2) != 1 || cases2[0].Answer != "a1" {
		t.Fatalf("对象包装结果错误: %+v", cases2)
	}

	p3 := filepath.Join(dir, "bad.json")
	mustWrite(t, p3, `{"broken json`)
	if _, err := LoadGoldenSet(p3); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}
	p4 := filepath.Join(dir, "plain.txt")
	mustWrite(t, p4, `纯文本不是 JSON`)
	if _, err := LoadGoldenSet(p4); err == nil {
		t.Fatalf("非 JSON 内容应报错")
	}
}

// 随包附带的黄金集骨架自检：3 条售后中文用例且字段完整可评
func TestGoldenSetSkeletonFile(t *testing.T) {
	cases, err := LoadGoldenSet("golden_set.json")
	if err != nil {
		t.Fatalf("加载随包黄金集失败: %v", err)
	}
	if len(cases) != 3 {
		t.Fatalf("黄金集用例数=%d, want 3", len(cases))
	}
	for i, c := range cases {
		if c.Question == "" || c.GroundTruth == "" || c.Answer == "" || len(c.Contexts) < 2 {
			t.Fatalf("第%d条字段不完整: %+v", i, c)
		}
	}
	rep := RunEval(cases)
	if !(rep.AvgContextRecall > 0 && rep.AvgContextRecall < 1) {
		t.Fatalf("黄金集召回率应落在 (0,1) 区间（改述文案部分覆盖属预期）: %.4f", rep.AvgContextRecall)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试文件失败: %v", err)
	}
}

var _ = json.Marshal
