package llm

import (
	"testing"
)

func newVoteTestDispatcher(qualities map[string]float64) *Dispatcher {
	d := &Dispatcher{providers: map[string]*ProviderConfig{}}
	for name, q := range qualities {
		d.providers[name] = &ProviderConfig{Name: name, QualityScore: q, Enabled: true}
	}
	return d
}

// A7：真实多数投票——2/3 一致时多数派胜出，即使其 Provider 质量分更低
func TestMultiModelVote_MajorityWinsOverQuality(t *testing.T) {
	d := newVoteTestDispatcher(map[string]float64{
		"cheap":   0.80,
		"mid":     0.85,
		"premium": 0.95,
	})
	results := []*DispatchResult{
		{Provider: "cheap", Content: "这款产品支持7天无理由退货"},
		{Provider: "mid", Content: "这款产品支持7天无理由退货!"},
		{Provider: "premium", Content: "完全不同的另一个答案内容"},
	}
	got := d.MultiModelVote(results)
	// 多数派={cheap,mid}（premium 被否决）；派系内取质量分最高者=mid
	if got != results[0].Content && got != results[1].Content {
		t.Fatalf("应返回多数派成员的答案，实际返回了 premium 的答案: %q", got)
	}
}

// 无过半多数派 → 回退质量分最高（原行为保底）
func TestMultiModelVote_NoMajorityFallsBackToQuality(t *testing.T) {
	d := newVoteTestDispatcher(map[string]float64{
		"a": 0.80,
		"b": 0.95,
	})
	results := []*DispatchResult{
		{Provider: "a", Content: "答案甲"},
		{Provider: "b", Content: "答案乙完全不同"},
	}
	if got := d.MultiModelVote(results); got != results[1].Content {
		t.Fatalf("无多数派应回退质量分最高者")
	}
}

func TestMultiModelVote_EmptyAndSingle(t *testing.T) {
	d := newVoteTestDispatcher(nil)
	if got := d.MultiModelVote(nil); got != "" {
		t.Fatalf("空输入应返回空串")
	}
	single := []*DispatchResult{{Provider: "x", Content: "唯一"}}
	if got := d.MultiModelVote(single); got != "唯一" {
		t.Fatalf("单结果应原样返回")
	}
}

func TestNormalizeVoteText(t *testing.T) {
	a := normalizeVoteText("Hello,   World")
	b := normalizeVoteText("hello, world")
	if a != b {
		t.Fatalf("归一化后应相等（大小写+空白折叠）: %q vs %q", a, b)
	}
	c := normalizeVoteText("7天\u200b无理由") // 含零宽字符
	if c != normalizeVoteText("7天无理由") {
		t.Fatalf("零宽字符应被剥离: %q", c)
	}
}

func TestBigramJaccard(t *testing.T) {
	if bigramJaccard("退货", "退货") != 1 {
		t.Fatalf("同文本相似度应为1")
	}
	if bigramJaccard("", "") != 1 {
		t.Fatalf("双空应为1")
	}
	if bigramJaccard("", "非空") != 0 {
		t.Fatalf("单侧为空应为0")
	}
	sim := bigramJaccard("支持七天无理由退货", "支持七天无理由退换")
	if sim <= 0.5 || sim >= 1 {
		t.Fatalf("部分重叠相似度应在(0.5,1): %f", sim)
	}
}
