package service

import (
	"strings"
	"testing"
)

func TestClassifyIntentCategories(t *testing.T) {
	cases := map[string]string{
		"哪家公司好":   "疑问",
		"两款机型参数对比": "对比",
		"2026推荐排行":  "推荐",
		"如何安装配置":   "教程",
		"真实体验评测":    "评测",
		"行业市场规模":    "信息",
	}
	for kw, want := range cases {
		if got := classifyIntent(kw); got != want {
			t.Errorf("classifyIntent(%q)=%q want %q", kw, got, want)
		}
	}
}

func TestGetIntentStrategy_Fallback(t *testing.T) {
	st := GetIntentStrategy("不存在的意图")
	if st.Intent != "信息" {
		t.Errorf("未知意图应回退到信息策略, got %+v", st)
	}
	if len(st.SourceTypes) == 0 || st.PromptHint == "" {
		t.Error("回退策略字段不完整")
	}
}

func TestEnhancePromptWithIntent(t *testing.T) {
	base := "基础prompt"
	out := EnhancePromptWithIntent(base, "两款机型参数对比")
	if !strings.Contains(out, base) {
		t.Error("增强后应保留原始 prompt")
	}
	for _, want := range []string{"对比", "决策意图适配", "第三方评测", "对照表"} {
		if !strings.Contains(out, want) {
			t.Errorf("增强 prompt 缺少策略要素 %q", want)
		}
	}
	// 不同意图产生不同指令
	outQ := EnhancePromptWithIntent(base, "如何安装")
	if strings.Contains(outQ, "对照表") {
		t.Error("教程意图不应携带对比类策略")
	}
}
