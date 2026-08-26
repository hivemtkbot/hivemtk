package confidence

import (
	"os"
	"strings"
	"testing"

	"hivemtk-user/internal/dto"
)

// S-5 单源化（2026-08-26）：本包不再维护关键词清单，VetoExplicit 的匹配函数
// 由 service 包注入（nlp_keywords.Transfer ∪ Explicit，含否定窗口过滤）。
// 本包单测通过 TestMain 注入等价 fixture，验证注入机制与触发语义；
// 词表本身的一致性由 service 侧 nlp_keywords 测试与接线测试保证。
var testExplicitKeywords = []string{
	"转人工", "人工客服", "找人工", "真人客服", "转接人工", "找客服", "人工服务",
	"real agent", "human agent", "transfer to human",
	"真人", "找人",
}

// TestMain 注入显式关键词匹配 fixture（替代历史包内清单）
func TestMain(m *testing.M) {
	SetExplicitKeywordMatcher(func(content string) bool {
		lower := strings.ToLower(content)
		for _, kw := range testExplicitKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
		return false
	})
	os.Exit(m.Run())
}

// TestVetoExplicit_MatcherNotInjectedNilSafe 未注入时不应 panic（防御性）
func TestVetoExplicit_MatcherNotInjectedNilSafe(t *testing.T) {
	prev := explicitKeywordMatcher
	explicitKeywordMatcher = nil
	defer func() { explicitKeywordMatcher = prev }()

	r := &VetoExplicit{}
	ctx := &VetoContext{CustomerMessage: "我要转人工"}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Error("未注入匹配函数时 VetoExplicit 不应触发")
	}
}

// TestSetExplicitKeywordMatcher_NilIgnored nil 注入必须被忽略（防止误清空）
func TestSetExplicitKeywordMatcher_NilIgnored(t *testing.T) {
	prev := explicitKeywordMatcher
	defer func() { explicitKeywordMatcher = prev }()

	SetExplicitKeywordMatcher(nil)
	if explicitKeywordMatcher == nil {
		t.Error("nil 注入应被忽略，匹配函数不应被置空")
	}
}
