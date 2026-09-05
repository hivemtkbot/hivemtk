package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/traceparent"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

const traceHeader = "X-Trace-Id"

// TraceMiddleware 为每个请求分配（或复用）trace_id 并绑定到请求 context。
//
// 解析顺序（按业界惯例）：
//  1. W3C `traceparent`（最高优先级，跨服务标准）。
//  2. legacy `X-Trace-Id`（项目历史兼容）。
//  3. 本层生成 UUIDv4。
//
// 行为：
//   - 透传上游 trace_id → 跨服务链路贯通。
//   - 自动构造子 span_id（基于 W3C parent span_id 派生；无父 span 时生成新 span_id）。
//   - trace_id 写入 gin.Context("trace_id") 与 c.Request.Context()（logger.WithTraceID），
//     业务代码通过 logger.Ctx(ctx) 自动携带。
//   - 响应头回写 X-Trace-Id（兼容）与 W3C traceparent（标准）。
//   - 异步发布根 span 事件到 TraceBus（防止慢订阅污染 P99）。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := extractTraceID(c)
		spanID := llm.GenerateSpanIDStatic()

		if tp, err := traceparent.Parse(c.GetHeader(traceparent.HeaderName)); err == nil && !tp.IsZeroContext() {
			logger.GetLogger().Info().Str("parsed_trace_id", tp.TraceID).Msg("[TraceMW] W3C parse success")
			traceID = tp.TraceID
		} else {
			logger.GetLogger().Debug().Err(err).Str("raw_header", c.GetHeader(traceparent.HeaderName)).Msg("[TraceMW] W3C parse fail or empty")
		}

		if traceID == "" {
			traceID = llm.GenerateTraceIDStatic()
		}

		c.Set("trace_id", traceID)
		c.Set("span_id", spanID)

		tc := llm.NewTraceContext(traceID, "")
		tc.SetSpanIDFor(spanID)
		c.Set("trace_ctx", tc)

		ctx := logger.WithTraceID(c.Request.Context(), traceID)

		carrier := &tracing.Carrier{TraceID: traceID}
		ctx = tracing.WithCarrier(ctx, carrier)
		c.Request = c.Request.WithContext(ctx)

		c.Writer.Header().Set(traceHeader, traceID)
		logger.GetLogger().Info().Str("traceID_len", fmt.Sprintf("%d", len(traceID))).Str("traceID", traceID).Str("spanID", spanID).Msg("[TraceMW] DEBUG args")
		if tpHeader, err := traceparent.Build(traceID, spanID, true); err == nil {
			c.Writer.Header().Set(traceparent.HeaderName, tpHeader)
			logger.GetLogger().Info().Str("traceparent", tpHeader).Msg("[TraceMW] set traceparent")
		} else {
			logger.GetLogger().Error().Err(err).Msg("[TraceMW] build failed")
		}

		start := time.Now()
		c.Next()

		duration := time.Since(start).Milliseconds()
		status := "ok"
		if c.Writer.Status() >= 500 {
			status = "error"
		}

		utils.SafeGo(c.Request.Context(), "middleware.trace.publish", func(_ context.Context) {
			llm.PublishTraceEvent(llm.TraceEvent{
				TraceID:    traceID,
				SpanID:     spanID,
				Kind:       llm.TraceSpanKindLog,
				Service:    "http",
				Operation:  c.Request.Method + " " + c.Request.URL.Path,
				DurationMs: duration,
				Status:     status,
				Metadata: map[string]any{
					"method":          c.Request.Method,
					"path":            c.Request.URL.Path,
					"status":          c.Writer.Status(),
					"client_ip":       c.ClientIP(),
					"user_agent":      c.Request.UserAgent(),
					"w3c_traceparent": c.GetHeader(traceparent.HeaderName) != "",
				},
			})
		})
	}
}

func extractTraceID(c *gin.Context) string {
	if tp, err := traceparent.Parse(c.GetHeader(traceparent.HeaderName)); err == nil && !tp.IsZeroContext() {
		return tp.TraceID
	}
	if legacy := c.GetHeader(traceHeader); legacy != "" {

		if len(legacy) == 32 {
			return legacy
		}

		return traceparent.FormatTraceIDForLegacy(legacy)
	}
	return ""
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
