package service

import (
	"testing"

	"hivemtk-user/internal/dto"
	confidencesvc "hivemtk-user/internal/service/confidence"
)

// TestVetoExplicit_SingleSourceWiring S-5 端到端接线回归：
// confidence.VetoExplicit 经 init 注入消费 nlp_keywords 导出匹配函数
// （Transfer ∪ Explicit 单一来源，含 N-2 否定窗口），包内不再有第二份清单。
func TestVetoExplicit_SingleSourceWiring(t *testing.T) {
	chain := confidencesvc.NewVetoChain()
	// 高置信信号隔离 Explicit 规则（避免 LowRAG/LowEntity 等兜底规则干扰断言）
	signals := &dto.FiveSignals{IntentConf: 0.9, EntityComp: 0.9, CtxRelev: 0.9, RAGQual: 0.9, LLMEntropy: 0.9}

	cases := []struct {
		msg    string
		want   bool
		reason string
	}{
		{"我要转人工", true, "veto_explicit"},
		{"please connect me to a REAL AGENT", true, "veto_explicit"}, // 大小写不敏感（nlp_keywords 统一转小写）
		{"帮我找一下你们的人工客服", true, "veto_explicit"},
		{"不用转人工，AI 就可以", false, ""}, // N-2 否定窗口：否定语境不触发
		{"这个产品多少钱", false, ""},
	}
	for _, c := range cases {
		triggered, reason := chain.Check(signals, &confidencesvc.VetoContext{CustomerMessage: c.msg})
		if triggered != c.want {
			t.Errorf("msg %q: triggered=%v, want %v", c.msg, triggered, c.want)
			continue
		}
		if c.want && reason != c.reason {
			t.Errorf("msg %q: reason=%v, want %v", c.msg, reason, c.reason)
		}
	}
}

// TestNLPKeywordsIsSingleSource 验证 nlp_keywords 词表覆盖历史两份清单的并集，
// 确保单源化后无关键词遗漏。
func TestNLPKeywordsIsSingleSource(t *testing.T) {
	union := map[string]bool{}
	for _, kw := range append(NLPKeywords.Transfer, NLPKeywords.Explicit...) {
		union[kw] = true
	}
	for _, kw := range []string{
		"转人工", "人工客服", "找人工", "真人客服", "转接人工", "找客服", "人工服务",
		"real agent", "human agent", "transfer to human",
		"真人", "找人",
	} {
		if !union[kw] {
			t.Errorf("历史 veto 关键词 %q 未被 nlp_keywords 覆盖", kw)
		}
	}
}
