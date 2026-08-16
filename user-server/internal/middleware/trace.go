package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/utils/logger"
)

// traceHeader 是入站/出站透传的追踪头名称。
// 若上游（网关/前端/调用方）已带该头，则复用其 trace_id 以贯通跨服务链路；否则本层生成。
const traceHeader = "X-Trace-Id"

// TraceMiddleware 为每个请求分配（或复用）trace_id 并绑定到请求 context。
//
// 契约（务必最先注册，位于脱敏/审计之前）：
//   - 入站若带 X-Trace-Id 则复用，否则用 logger.GenerateTraceID() 生成，保证链路必有追踪标识。
//   - trace_id 同时写入 gin.Context("trace_id") 与 c.Request.Context()（经 logger.WithTraceID），
//     后续 handler / service / 编排 / 触达全链路通过 logger.Ctx(ctx) 自动携带同一 trace_id。
//   - 响应头回写 X-Trace-Id，便于客户端与线上日志按 trace_id 关联定位。
//   - 同时构造根 TraceContext 注入 gin.Context("trace_ctx")，
//     供 LLM/工具/DB/RAG/WebSocket 等子 span 通过 ChildSpan 串联全链路。
//   - 发布根 span 事件到 TraceBus，便于 trace 服务持久化与查询。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(traceHeader)
		if traceID == "" {
			traceID = logger.GenerateTraceID()
		}

		c.Set("trace_id", traceID)

		tc := llm.NewTraceContext(traceID, "")
		c.Set("trace_ctx", tc)

		c.Request = c.Request.WithContext(logger.WithTraceID(c.Request.Context(), traceID))

		c.Writer.Header().Set(traceHeader, traceID)

		start := time.Now()
		c.Next()

		duration := time.Since(start).Milliseconds()
		status := "ok"
		if c.Writer.Status() >= 500 {
			status = "error"
		}
		// v3 审计 P1-12 修复：异步发布防止 trace 慢订阅污染 P99
		// 原：sync PublishTraceEvent → 订阅者慢/满会阻塞请求
		// 新：go 异步
		go func(evt llm.TraceEvent) {
			llm.PublishTraceEvent(evt)
		}(llm.TraceEvent{
			TraceID:    traceID,
			SpanID:     tc.SpanID(),
			Kind:       llm.TraceSpanKindLog,
			Service:    "http",
			Operation:  c.Request.Method + " " + c.Request.URL.Path,
			DurationMs: duration,
			Status:     status,
			Metadata: map[string]any{
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"status":     c.Writer.Status(),
				"client_ip":  c.ClientIP(),
				"user_agent": c.Request.UserAgent(),
			},
		})
	}
}

// TraceContextFromGin 从 gin.Context 取出 TraceContext（不存在返回 nil）
func TraceContextFromGin(c *gin.Context) *llm.TraceContext {
	if c == nil {
		return nil
	}
	if v, ok := c.Get("trace_ctx"); ok {
		if tc, ok := v.(*llm.TraceContext); ok {
			return tc
		}
	}
	return nil
}

