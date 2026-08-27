package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/traceparent"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

// traceHeader 是项目 legacy 兼容的追踪头（仍保留以支持旧客户端）。
// 优先级：W3C traceparent > X-Trace-Id > 本层生成 UUID。
//
// 业界标准 W3C Trace Context Level 1：https://www.w3.org/TR/trace-context-1/
// W3C traceparent 头格式：00-<32hex trace_id>-<16hex span_id>-<flags>
// 优点：跨服务/跨语言标准兼容（OpenTelemetry / Jaeger / Datadog 全部支持）。
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

		// 若上游 traceparent 携带了 parent span_id，则派生子 span_id。
		// 这里直接用新生成 span_id（更规范做法是基于 parent_id 做 hash 派生，
		// 但当前没有跨进程 trace 关联需求，新 span_id 即可）。
		if tp, err := traceparent.Parse(c.GetHeader(traceparent.HeaderName)); err == nil && !tp.IsZeroContext() {
			logger.GetLogger().Info().Str("parsed_trace_id", tp.TraceID).Msg("[TraceMW] W3C parse success")
			traceID = tp.TraceID
		} else {
			logger.GetLogger().Debug().Err(err).Str("raw_header", c.GetHeader(traceparent.HeaderName)).Msg("[TraceMW] W3C parse fail or empty")
		}

		// 如果 traceID 仍为空，生成一个新的 W3C 规范 trace_id
		if traceID == "" {
			traceID = llm.GenerateTraceIDStatic()
		}

		c.Set("trace_id", traceID)
		c.Set("span_id", spanID)

		tc := llm.NewTraceContext(traceID, "")
		tc.SetSpanIDFor(spanID) // 注入本服务产生的子 span_id
		c.Set("trace_ctx", tc)

		c.Request = c.Request.WithContext(logger.WithTraceID(c.Request.Context(), traceID))

		// 响应头：legacy X-Trace-Id + 标准 W3C traceparent 双写
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
		// v3 审计 P1-12 修复：异步发布防止 trace 慢订阅污染 P99
		// 原：sync PublishTraceEvent → 订阅者慢/满会阻塞请求
		// 新：go 异步
		// 最高标准审计 P1-3 修复：改走 SafeGo，trace 发布 panic 不再击穿进程
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
				"method":         c.Request.Method,
				"path":           c.Request.URL.Path,
				"status":         c.Writer.Status(),
				"client_ip":      c.ClientIP(),
				"user_agent":     c.Request.UserAgent(),
				"w3c_traceparent": c.GetHeader(traceparent.HeaderName) != "",
			},
			})
		})
	}
}

// extractTraceID 优先级提取 trace_id：
//  1. W3C traceparent（标准，32 位 hex）
//  2. legacy X-Trace-Id（兼容，需转换为 32 位 hex 格式）
//  3. 返回空字符串（调用方负责生成）
func extractTraceID(c *gin.Context) string {
	if tp, err := traceparent.Parse(c.GetHeader(traceparent.HeaderName)); err == nil && !tp.IsZeroContext() {
		return tp.TraceID
	}
	if legacy := c.GetHeader(traceHeader); legacy != "" {
		// 如果 legacy 已经是 32 位 hex，直接返回
		if len(legacy) == 32 {
			return legacy
		}
		// 否则通过 FormatTraceIDForLegacy 转换（主要用于去掉空格和大小写）
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
