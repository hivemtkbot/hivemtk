package ragcustomerservice

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/i18n"
)

func TestDetectLangCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"你好，我的订单在哪里", "zh"},
		{"こんにちは、注文を確認したい", "ja"},
		{"안녕하세요 배송 조회", "ko"},
		{"مرحبا أين طلبي", "ar"},
		{"สวัสดีค่ะ สอบถามออเดอร์", "th"},
		{"Привет, где мой заказ", "ru"},
		{"Xin chào, đơn hàng của tôi đâu", "vi"},
		{"Hello, where is my order", ""}, // 纯拉丁 → 不切换
		{"ok", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := i18n.DetectLangCode(c.in); got != c.want {
			t.Errorf("DetectLangCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveTargetLang(t *testing.T) {
	g := &ResponseGeneratorImpl{}
	ctx := context.Background()

	// 内部 zh，未显式配置目标 → 自动检测日语 → ja
	if got := g.resolveTargetLang(ctx, "zh", "zh", "こんにちは"); got != "ja" {
		t.Errorf("resolveTargetLang = %q, want ja", got)
	}
	// 显式配置 en → 尊重配置，不做自动检测
	if got := g.resolveTargetLang(ctx, "zh", "en", "こんにちは"); got != "en" {
		t.Errorf("resolveTargetLang = %q, want en", got)
	}
	// 内部 zh，客户讲中文 → 保持 zh
	if got := g.resolveTargetLang(ctx, "zh", "zh", "你好"); got != "zh" {
		t.Errorf("resolveTargetLang = %q, want zh", got)
	}
	// 内部 zh，英文查询 → 保守不切换（避免中文坐席场景误判）
	if got := g.resolveTargetLang(ctx, "zh", "zh", "hello"); got != "zh" {
		t.Errorf("resolveTargetLang = %q, want zh", got)
	}
}
