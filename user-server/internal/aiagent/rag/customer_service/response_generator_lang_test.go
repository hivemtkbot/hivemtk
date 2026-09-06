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
		{"Hello, where is my order", ""},
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

	if got := g.resolveTargetLang(ctx, "zh", "zh", "こんにちは"); got != "ja" {
		t.Errorf("resolveTargetLang = %q, want ja", got)
	}
	if got := g.resolveTargetLang(ctx, "zh", "en", "こんにちは"); got != "en" {
		t.Errorf("resolveTargetLang = %q, want en", got)
	}
	if got := g.resolveTargetLang(ctx, "zh", "zh", "你好"); got != "zh" {
		t.Errorf("resolveTargetLang = %q, want zh", got)
	}
	if got := g.resolveTargetLang(ctx, "zh", "zh", "hello"); got != "zh" {
		t.Errorf("resolveTargetLang = %q, want zh", got)
	}
}
