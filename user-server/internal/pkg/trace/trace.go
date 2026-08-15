// Package trace 统一追踪 ID 全链路传递包
//
// 五层架构归属: L2 服务层 / 工具层
// 设计依据: PRD § 缺口修复
// 私域独立部署: 无 merchant_id 字段
//
// 职责：
//   - 生成 UUID 风格 TraceId（与 google/uuid 兼容，便于跨服务关联）
//   - 提供 ctx 注入 / 提取 / 拷贝 / 重写 API
//   - 提供 Gin 中间件（自动从 X-Trace-Id 头提取或新生成；写入响应头）
//   - 提供 JSON 日志字段辅助（logger.Ctx 透传 trace_id + span_id）
//   - 与 aiagent/llm.TraceContext 双向兼容（trace 事件、Span 树）
//
// 与 aiagent/llm/trace_context.go 的关系：
//   - aiagent/llm.TraceContext: 完整 Span 树（trace_id + span_id + parent_span_id + metadata）
//   - pkg/trace.Tracer: 轻量级 trace_id 单值传递（仅 trace_id，无 Span 树）
//   - Tracer 的 TraceID 可直接转 llm.TraceContext.NewTraceContext(traceID, "")
package trace

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"hivemtk-user/internal/pkg/utils/logger"
)

// HeaderName 入站 / 出站透传的追踪头名称
const HeaderName = "X-Trace-Id"

// LogFieldTraceId 日志 JSON 中 trace_id 字段名
const LogFieldTraceId = "trace_id"

// LogFieldSpanId 日志 JSON 中 span_id 字段名
const LogFieldSpanId = "span_id"

// LogFieldParentSpanId 日志 JSON 中 parent_span_id 字段名
const LogFieldParentSpanId = "parent_span_id"

// LogFieldOperation 日志 JSON 中 operation 字段名
const LogFieldOperation = "operation"


type ctxKey int

const (
	traceIDKey ctxKey = iota
	spanIDKey
	parentSpanIDKey
)

// Tracer 追踪器
//
// 线程安全：所有方法均无状态或使用 sync.RWMutex。
// 生命周期：可作为单例 / 全局注入到 service 层。
type Tracer struct {
	mu       sync.RWMutex
	traceID  string
	spanID   string
	parentID string
	started  time.Time
}

// NewTracer 创建新的追踪器（traceID 空时自动生成 UUIDv4）
// spanID 同样自动生成（UUIDv4 前 16 位）
func NewTracer(traceID, parentSpanID string) *Tracer {
	t := &Tracer{
		started: time.Now(),
	}
	if traceID == "" {
		t.traceID = GenerateTraceID()
	} else {
		t.traceID = traceID
	}
	t.spanID = GenerateSpanID()
	t.parentID = parentSpanID
	return t
}

// NewTracerFromContext 从 ctx 还原 Tracer（无 trace_id 时自动生成）
func NewTracerFromContext(ctx context.Context) *Tracer {
	if ctx == nil {
		return NewTracer("", "")
	}
	tid := logger.TraceIDFromContext(ctx)
	return NewTracer(tid, "")
}

// GenerateTraceID 生成分布式追踪 ID（UUID v4）。
// 使用 google/uuid 保证唯一性；按时间排序由 timestamp 字段承担。
func GenerateTraceID() string {
	return uuid.NewString()
}

// GenerateSpanID 生成 span_id（UUIDv4 前 16 位 hex）
func GenerateSpanID() string {
	id := uuid.NewString()
	if len(id) >= 16 {
		return id[:16]
	}
	return id
}

// TraceID 返回 trace_id
func (t *Tracer) TraceID() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.traceID
}

// SpanID 返回 span_id
func (t *Tracer) SpanID() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.spanID
}

// ParentSpanID 返回 parent_span_id
func (t *Tracer) ParentSpanID() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.parentID
}

// StartedAt 返回追踪器创建时间
func (t *Tracer) StartedAt() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.started
}

// InjectContext 将 trace_id 注入 context（与 logger.WithTraceID 兼容）
//
// 返回的 ctx 包含：
//   - trace_id（供 logger.Ctx 自动携带到 JSON 日志）
//   - span_id（供 Span 树串联）
//   - parent_span_id
//
// 用法：
//
//	ctx = tracer.InjectContext(parentCtx)
//	logger.Ctx(ctx).Info().Msg("...")
func (t *Tracer) InjectContext(ctx context.Context) context.Context {
	if t == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	t.mu.RLock()
	tid := t.traceID
	sid := t.spanID
	pid := t.parentID
	t.mu.RUnlock()
	ctx = logger.WithTraceID(ctx, tid)
	ctx = context.WithValue(ctx, spanIDKey, sid)
	if pid != "" {
		ctx = context.WithValue(ctx, parentSpanIDKey, pid)
	}
	return ctx
}

// SpanIDFromContext 从 ctx 取出 span_id（无则返回空串）
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(spanIDKey).(string); ok {
		return v
	}
	return ""
}

// ParentSpanIDFromContext 从 ctx 取出 parent_span_id（无则返回空串）
func ParentSpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(parentSpanIDKey).(string); ok {
		return v
	}
	return ""
}

// ChildSpan 创建子 Span（trace_id 不变，parent_span_id = 当前 span_id）
func (t *Tracer) ChildSpan() *Tracer {
	if t == nil {
		return NewTracer("", "")
	}
	t.mu.RLock()
	tid := t.traceID
	pid := t.spanID
	t.mu.RUnlock()
	return &Tracer{
		traceID:  tid,
		spanID:   GenerateSpanID(),
		parentID: pid,
		started:  time.Now(),
	}
}

// NewContextWithTraceID 便捷函数：仅注入 trace_id（不创建 Span 树）
// 适用于：service / repository 层不涉及 Span 串联，仅需要日志携带 trace_id 的场景
func NewContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	return logger.WithTraceID(ctx, traceID)
}

// TraceIDFromContext 便捷函数：从 ctx 取出 trace_id
func TraceIDFromContext(ctx context.Context) string {
	return logger.TraceIDFromContext(ctx)
}


// GinTraceMiddleware Gin 追踪中间件
//
// 契约：
//   - 入站若带 X-Trace-Id 头则复用，否则用 GenerateTraceID 生成
//   - trace_id 写入 gin.Context("trace_id") 与 c.Request.Context()
//   - 同时注入 span_id（新建）到 ctx
//   - 响应头回写 X-Trace-Id
//
// 用法（router/router.go）：
//
//	r.Use(trace.GinTraceMiddleware())
//	r.Use(trace.GinTraceLogMiddleware())  // 可选：记录访问日志
func GinTraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(HeaderName)
		if traceID == "" {
			traceID = GenerateTraceID()
		}
		spanID := GenerateSpanID()

		c.Set(LogFieldTraceId, traceID)
		c.Set(LogFieldSpanId, spanID)

		ctx := logger.WithTraceID(c.Request.Context(), traceID)
		ctx = context.WithValue(ctx, spanIDKey, spanID)
		c.Request = c.Request.WithContext(ctx)

		c.Writer.Header().Set(HeaderName, traceID)

		c.Next()
	}
}

// GinTraceLogMiddleware Gin 访问日志中间件（可选）
//
// 依赖：必须先注册 GinTraceMiddleware 以确保 trace_id 已写入 ctx。
// 行为：请求结束后输出结构化访问日志（含 trace_id / method / path / status / duration）。
func GinTraceLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)
		traceID := TraceIDFromContext(c.Request.Context())
		spanID := SpanIDFromContext(c.Request.Context())

		status := "ok"
		if c.Writer.Status() >= 500 {
			status = "error"
		}
		logger.Ctx(c.Request.Context()).Info().
			Str(LogFieldTraceId, traceID).
			Str(LogFieldSpanId, spanID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Int64("duration_ms", duration.Milliseconds()).
			Str("result", status).
			Str("client_ip", c.ClientIP()).
			Msg("http_access")
	}
}

// TraceIDFromGin 从 gin.Context 取出 trace_id（不存在返回空串）
func TraceIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(LogFieldTraceId); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}


// LogFields 返回结构化日志字段（trace_id / span_id / parent_span_id）
// 用于 service 层手动构建结构化日志：
//
//	logger.Ctx(ctx).Info().Fields(tracer.LogFields()).Msg("...")
func (t *Tracer) LogFields() map[string]any {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return map[string]any{
		LogFieldTraceId:      t.traceID,
		LogFieldSpanId:       t.spanID,
		LogFieldParentSpanId: t.parentID,
	}
}

