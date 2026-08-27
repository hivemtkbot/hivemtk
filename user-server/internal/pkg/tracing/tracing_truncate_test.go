package tracing

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"hivemtk-user/internal/pkg/textutil"
)

func TestTruncateTraceTextShortUnchanged(t *testing.T) {
	for _, s := range []string{"", "hello", "中文消息✅", strings.Repeat("ab", 4096)} { // 恰好 8192 字节
		if got := textutil.TruncateText(s, textutil.DefaultMaxBytes); got != s {
			t.Errorf("short text (%d bytes) must be unchanged", len(s))
		}
	}
}

func TestTruncateTraceTextLongASCII(t *testing.T) {
	orig := strings.Repeat("a", 10000)
	got := textutil.TruncateText(orig, textutil.DefaultMaxBytes)
	if !strings.HasSuffix(got, "…[truncated 1808 bytes]") {
		t.Fatalf("suffix wrong: %q", got[len(got)-30:])
	}
	// 头部保留：8192 字节的 'a' + 后缀
	wantHead := strings.Repeat("a", 8192)
	if !strings.HasPrefix(got, wantHead) {
		t.Fatal("head content lost")
	}
}

func TestTruncateTraceTextMultibyteBoundaryValidUTF8(t *testing.T) {
	// 中文每字 3 字节，构造截断点落在多字节字符中间的情况
	orig := strings.Repeat("中", 5000) // 15000 字节
	got := textutil.TruncateText(orig, textutil.DefaultMaxBytes)
	if !utf8.ValidString(got) {
		t.Fatal("truncated output is not valid UTF-8")
	}
	if !strings.HasSuffix(got, "]") {
		t.Fatalf("bad suffix: %q", got[len(got)-40:])
	}
	// 后缀中的 N 应等于被丢弃字节数：头部按 rune 边界回退
	head := got[:strings.LastIndex(got, "…")]
	if len(head)%3 != 0 || len(head) > 8192 {
		t.Fatalf("head length %d not a rune-safe cut of 3-byte chars", len(head))
	}
	dropped := len(orig) - len(head)
	want := fmt.Sprintf("…[truncated %d bytes]", dropped)
	if !strings.HasSuffix(got, want) {
		t.Fatalf("suffix = %q, want %q", got[len(got)-len(want):], want)
	}
}

func TestToModelFromPendingAppliesTruncation(t *testing.T) {
	p := pendingSpan{
		traceID: "tr-x",
		node:    NodeIngest,
		input:   map[string]any{"blob": strings.Repeat("x", 20000)},
		output:  "ok",
	}
	m := toModelFromPending(p)
	if m.Output != "ok" {
		t.Fatal("short output must be unchanged (metadata/short fields untouched)")
	}
	if len(m.Input) <= textutil.DefaultMaxBytes || !strings.Contains(m.Input, "[truncated") {
		t.Fatalf("input should be truncated, len=%d", len(m.Input))
	}
}
