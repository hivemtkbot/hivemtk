package service

import (
	"context"
	"strings"
	"testing"
)

// --- H-5/X-3 广告法极限词过滤 ---

func TestFilterExtremeClaims_Positive(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		banned  []string
		mustHave string
	}{
		{"最好", "我们的产品是最好的", []string{"最好"}, "很好"},
		{"顶级", "顶级配置", []string{"顶级"}, "高端"},
		{"绝对", "绝对超值", []string{"绝对"}, "确实"},
		{"100%", "100%正品", []string{"100%"}, ""},
		{"根治", "可以根治问题", []string{"根治"}, "有效改善"},
		{"国家级", "国家级认证", []string{"国家级"}, "高标准"},
		{"销量第一", "销量第一的品牌", []string{"销量第一"}, "销量领先"},
		{"百分百", "百分百满意", []string{"百分百"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterExtremeClaims(tt.in)
			for _, b := range tt.banned {
				if strings.Contains(got, b) {
					t.Errorf("output %q still contains banned word %q", got, b)
				}
			}
			if tt.mustHave != "" && !strings.Contains(got, tt.mustHave) {
				t.Errorf("output %q should contain compliant replacement %q", got, tt.mustHave)
			}
		})
	}
}

func TestFilterExtremeClaims_Negative(t *testing.T) {
	// 反例：正常词不应被误伤
	tests := []string{
		"最近天气不错",
		"您第一次使用可以先看教程",
		"我们最重视您的反馈，但不敢说最好",
	}
	// "最重视"不在词表 → 保留；替换后仍应包含原片段
	if got := filterExtremeClaims(tests[0]); got != tests[0] {
		t.Errorf("「最近」被误伤: %q", got)
	}
	if got := filterExtremeClaims(tests[1]); got != tests[1] {
		t.Errorf("「第一次」被误伤: %q", got)
	}
}

func TestPolish_FiltersExtremeClaims(t *testing.T) {
	p := NewHumanizePolisher()
	out, err := p.Polish(context.Background(), "这款产品是最好的，绝对让您满意！", nil)
	if err != nil {
		t.Fatalf("polish: %v", err)
	}
	if strings.Contains(out, "最好") || strings.Contains(out, "绝对") {
		t.Errorf("Polish 输出含极限词: %q", out)
	}
}
