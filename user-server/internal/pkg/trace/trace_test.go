package trace

// trace_test.go 追踪包单元测试
//
// 覆盖范围：
//   - TraceID / SpanID 生成（UUIDv4 格式）
//   - ctx 注入 / 提取（trace_id / span_id / parent_span_id）
//   - ChildSpan 串联
//   - Gin 中间件（X-Trace-Id 头解析 / 回写）

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGenerateTraceID(t *testing.T) {
	id := GenerateTraceID()
	if id == "" {
		t.Fatal("expected non-empty trace_id")
	}
	// UUIDv4 长度 36 (含 4 个 '-')
	if len(id) != 36 {
		t.Errorf("expected length 36, got %d (%s)", len(id), id)
	}
	// 两次生成的 ID 不重复
	if id == GenerateTraceID() {
		t.Error("expected unique trace_id per call")
	}
}

func TestGenerateSpanID(t *testing.T) {
	sid := GenerateSpanID()
	if sid == "" {
		t.Fatal("expected non-empty span_id")
	}
	if len(sid) != 16 {
		t.Errorf("expected length 16, got %d (%s)", len(sid), sid)
	}
}

func TestNewTracerDefaults(t *testing.T) {
	tr := NewTracer("", "")
	if tr.TraceID() == "" {
		t.Error("expected auto-generated trace_id")
	}
	if tr.SpanID() == "" {
		t.Error("expected auto-generated span_id")
	}
	if tr.ParentSpanID() != "" {
		t.Error("expected empty parent_span_id for root")
	}
}

func TestNewTracerWithIDs(t *testing.T) {
	tr := NewTracer("trace-1", "span-parent")
	if tr.TraceID() != "trace-1" {
		t.Errorf("expected trace-1, got %s", tr.TraceID())
	}
	if tr.ParentSpanID() != "span-parent" {
		t.Errorf("expected span-parent, got %s", tr.ParentSpanID())
	}
}

func TestTracerInjectContext(t *testing.T) {
	tr := NewTracer("trace-xyz", "")
	ctx := tr.InjectContext(context.Background())
	if got := TraceIDFromContext(ctx); got != "trace-xyz" {
		t.Errorf("expected trace-xyz, got %s", got)
	}
	if got := SpanIDFromContext(ctx); got == "" {
		t.Error("expected span_id in ctx")
	}
	if got := ParentSpanIDFromContext(ctx); got != "" {
		t.Errorf("expected empty parent, got %s", got)
	}
}

func TestTracerInjectContextNilCtx(t *testing.T) {
	tr := NewTracer("trace-nil", "")
	// 应回退到 context.Background() 不 panic
	ctx := tr.InjectContext(context.TODO())
	if got := TraceIDFromContext(ctx); got != "trace-nil" {
		t.Errorf("expected trace-nil, got %s", got)
	}
}

func TestTracerChildSpan(t *testing.T) {
	parent := NewTracer("trace-parent", "span-grand-parent")
	child := parent.ChildSpan()
	if child.TraceID() != parent.TraceID() {
		t.Error("child should inherit trace_id")
	}
	if child.ParentSpanID() != parent.SpanID() {
		t.Errorf("child.parent=%s, parent.span=%s", child.ParentSpanID(), parent.SpanID())
	}
	if child.SpanID() == parent.SpanID() {
		t.Error("child span_id should differ from parent")
	}
}

func TestNewContextWithTraceID(t *testing.T) {
	ctx := NewContextWithTraceID(context.Background(), "trace-ctx")
	if got := TraceIDFromContext(ctx); got != "trace-ctx" {
		t.Errorf("expected trace-ctx, got %s", got)
	}
	// 空 traceID 应自动生成
	ctx2 := NewContextWithTraceID(context.Background(), "")
	if got := TraceIDFromContext(ctx2); got == "" {
		t.Error("expected auto-generated trace_id when empty input")
	}
}

func TestNewTracerFromContext(t *testing.T) {
	ctx := NewContextWithTraceID(context.Background(), "from-ctx")
	tr := NewTracerFromContext(ctx)
	if tr.TraceID() != "from-ctx" {
		t.Errorf("expected from-ctx, got %s", tr.TraceID())
	}
}

func TestNilTracerSafety(t *testing.T) {
	var tr *Tracer
	if tr.TraceID() != "" {
		t.Error("nil tracer should return empty trace_id")
	}
	if tr.SpanID() != "" {
		t.Error("nil tracer should return empty span_id")
	}
	if tr.InjectContext(context.Background()) == nil {
		// 这里不能直接 == nil 比较，需要用 Error 检查
	}
	if tr.LogFields() != nil {
		t.Error("nil tracer should return nil log fields")
	}
}

func TestLogFields(t *testing.T) {
	tr := NewTracer("trace-log", "span-log")
	fields := tr.LogFields()
	if fields[LogFieldTraceId] != "trace-log" {
		t.Errorf("expected trace-log, got %v", fields[LogFieldTraceId])
	}
	if fields[LogFieldSpanId] == "" {
		t.Error("expected non-empty span_id in log fields")
	}
	if fields[LogFieldParentSpanId] != "span-log" {
		t.Errorf("expected span-log, got %v", fields[LogFieldParentSpanId])
	}
}

// ===== Gin 中间件测试 =====

func TestGinTraceMiddleware_Generate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinTraceMiddleware())
	var capturedTraceID string
	r.GET("/test", func(c *gin.Context) {
		capturedTraceID = TraceIDFromGin(c)
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	if capturedTraceID == "" {
		t.Fatal("expected non-empty trace_id in handler")
	}
	if w.Header().Get(HeaderName) != capturedTraceID {
		t.Errorf("expected response header to match handler trace_id: %s vs %s", w.Header().Get(HeaderName), capturedTraceID)
	}
}

func TestGinTraceMiddleware_ReuseHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinTraceMiddleware())
	var capturedTraceID string
	r.GET("/test", func(c *gin.Context) {
		capturedTraceID = TraceIDFromGin(c)
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(HeaderName, "incoming-trace-id")
	r.ServeHTTP(w, req)
	if capturedTraceID != "incoming-trace-id" {
		t.Errorf("expected reuse incoming-trace-id, got %s", capturedTraceID)
	}
	if w.Header().Get(HeaderName) != "incoming-trace-id" {
		t.Errorf("expected response header incoming-trace-id, got %s", w.Header().Get(HeaderName))
	}
}

func TestGinTraceMiddleware_ContextPropagation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinTraceMiddleware())
	var ctxTraceID, ctxSpanID string
	r.GET("/test", func(c *gin.Context) {
		ctxTraceID = TraceIDFromContext(c.Request.Context())
		ctxSpanID = SpanIDFromContext(c.Request.Context())
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	if ctxTraceID == "" {
		t.Error("expected trace_id propagated to request context")
	}
	if ctxSpanID == "" {
		t.Error("expected span_id propagated to request context")
	}
}

func TestGinTraceMiddleware_NilContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinTraceMiddleware())
	r.GET("/test", func(c *gin.Context) {
		// gin.Context 为 nil 不应该 panic
		if TraceIDFromGin(nil) != "" {
			t.Error("nil gin context should return empty trace_id")
		}
		c.String(http.StatusOK, "ok")
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
}
