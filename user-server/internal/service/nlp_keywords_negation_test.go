package service

import (
	"strings"
	"testing"
)

// --- N-2 否定窗口 ---

func TestMatchTransferKeywords_NegationWindow(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantHit bool
	}{
		{"直接转人工命中", "请帮我转人工", true},
		{"不用转人工不命中", "不用转人工了，我自己能搞定", false},
		{"别找人工不命中", "别找人工了，太麻烦", false},
		{"无需人工客服不命中", "无需人工客服介入", false},
		{"不想转接人工不命中", "我不想转接人工，先自己试试", false},
		{"正常语境仍命中", "这个问题解决不了，我要转人工", true},
		{"否定词超出6字窗口仍命中", "之前说了不需要别的推荐，现在正式要求：转人工", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchTransferKeywords(tt.text); got != tt.wantHit {
				t.Errorf("MatchTransferKeywords(%q) = %v, want %v", tt.text, got, tt.wantHit)
			}
		})
	}
}

func TestMatchExplicitKeywords_NegationWindow(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantHit bool
	}{
		{"显式关键词命中", "给我转人工客服", true},
		{"不用转人工不命中(veto链)", "不用转人工", false},
		{"英文不受中文否定影响", "I don't need transfer to human now, actually transfer to human please", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchExplicitKeywords(tt.text); got != tt.wantHit {
				t.Errorf("MatchExplicitKeywords(%q) = %v, want %v", tt.text, got, tt.wantHit)
			}
		})
	}
}

func TestMatchUrgentKeywords_NegationWindow(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantHit bool
	}{
		{"退款投诉命中", "我要投诉，马上退款", true},
		{"不用退款不命中", "算了，不用退款了", false},
		{"不想退钱不命中", "不想退钱，就是说说而已", false},
		{"紧急正常命中", "这个很紧急", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchUrgentKeywords(tt.text); got != tt.wantHit {
				t.Errorf("MatchUrgentKeywords(%q) = %v, want %v", tt.text, got, tt.wantHit)
			}
		})
	}
}

func TestHasNegationBefore_EdgeCases(t *testing.T) {
	lower := "不用转人工"
	idx := strings.Index(lower, "转")
	if !hasNegationBefore(lower, idx) {
		t.Error("「转」前 6 字符含『不用』应判定否定")
	}
	if hasNegationBefore(lower, 0) {
		t.Error("idx=0 无前缀不应判定否定")
	}
}
