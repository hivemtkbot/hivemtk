package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newRecordedSpan(t *testing.T) (trace.Span, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	_, span := tp.Tracer("langfuse-test").Start(context.Background(), "op")
	return span, rec
}

// findAttr 在 span 属性切片中按键名线性查找
func findAttr(kvs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, kv := range kvs {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

// 属性写入：去空白、tags 过滤空项、键名正确
func TestApplyLangfuseAttrsWrites(t *testing.T) {
	span, rec := newRecordedSpan(t)
	ApplyLangfuseAttrs(span, " sess-1 ", "user-9", "客服", " vip ", "   ")
	span.End()

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("结束 span 数=%d, want 1", len(ended))
	}
	attrs := ended[0].Attributes()

	if v, ok := findAttr(attrs, LangfuseSessionIDKey); !ok || v.AsString() != "sess-1" {
		t.Fatalf("session_id 属性错误: ok=%v v=%v", ok, v)
	}
	if v, ok := findAttr(attrs, LangfuseUserIDKey); !ok || v.AsString() != "user-9" {
		t.Fatalf("user_id 属性错误: ok=%v v=%v", ok, v)
	}
	v, ok := findAttr(attrs, LangfuseTagsKey)
	if !ok {
		t.Fatalf("tags 属性缺失")
	}
	got := v.AsStringSlice()
	if len(got) != 2 || got[0] != "客服" || got[1] != "vip" {
		t.Fatalf("tags 应过滤空项后为 [客服 vip]: %v", got)
	}
}

// 守卫：nil span 不 panic；非记录态不写；空参数不写任何键
func TestApplyLangfuseAttrsGuards(t *testing.T) {
	ApplyLangfuseAttrs(nil, "s", "u", "t")

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	_, span := tp.Tracer("noop").Start(context.Background(), "op")
	if span.IsRecording() {
		t.Fatalf("NeverSample 下 span 不应处于记录态")
	}
	ApplyLangfuseAttrs(span, "s", "u", "t")

	span2, rec2 := newRecordedSpan(t)
	ApplyLangfuseAttrs(span2, "", "", " ", "")
	span2.End()
	ended := rec2.Ended()
	if len(ended) != 1 {
		t.Fatalf("结束 span 数=%d, want 1", len(ended))
	}
	if n := len(ended[0].Attributes()); n != 0 {
		t.Fatalf("空参数不应写任何属性, got %d 个", n)
	}
}
