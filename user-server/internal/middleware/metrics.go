package middleware

// metrics.go Prometheus 指标暴露与采集
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md P3 监控指标
//
// 私域部署：内置 Prometheus 兼容的指标收集
// 对应 AUDIT_REPORT P0-19 修复
//
// 通过 /metrics 端点暴露：
//
//	http_requests_total{method,path,status} counter
//	http_request_duration_seconds{method,path} summary
//	http_active_connections gauge
//	http_request_size_bytes{method,path} summary
//	http_response_size_bytes{method,path} summary
//
// 业务指标（置信度/拟人度/反馈学习）：
//   - confidence_* / humanize_* / feedback_*
//
// 实现说明：基础类型（CounterVec 等）已迁移至 internal/pkg/metrics，
// 本文件仅负责 HTTP 中间件 + 指标格式化输出，避免 import cycle。

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"marketing/internal/pkg/metrics"
)

// PrometheusMetricsMiddleware Prometheus 指标中间件
// 收集每个 HTTP 请求的耗时、状态码、字节数等指标
func PrometheusMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		metrics.GlobalMetrics.ActiveConns.Inc()
		defer metrics.GlobalMetrics.ActiveConns.Dec()

		// 记录请求大小
		if c.Request.ContentLength > 0 {
			metrics.GlobalMetrics.RequestSize.Observe(c.Request.Method+c.FullPath(), float64(c.Request.ContentLength))
		}

		// 处理请求
		c.Next()

		// 记录指标
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		labels := c.Request.Method + "|" + c.FullPath() + "|" + status

		metrics.GlobalMetrics.RequestTotal.Inc(labels)
		metrics.GlobalMetrics.RequestDuration.Observe(c.Request.Method+"|"+c.FullPath(), duration)
		metrics.GlobalMetrics.ResponseSize.Observe(c.Request.Method+"|"+c.FullPath(), float64(c.Writer.Size()))
	}
}

// MetricsHandler Prometheus 指标暴露端点
// 返回文本格式的指标（兼容 Prometheus exposition format v0.0.4）
func MetricsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var sb strings.Builder
	// 预分配缓冲区，避免反复扩容
	sb.Grow(8 * 1024)

	sb.WriteString("# HELP http_requests_total Total number of HTTP requests\n")
	sb.WriteString("# TYPE http_requests_total counter\n")
	metrics.GlobalMetrics.RequestTotal.Range(func(labels string, count uint64) {
		sb.WriteString("http_requests_total{")
		sb.WriteString(formatHTTPLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP http_active_connections Current number of active connections\n")
	sb.WriteString("# TYPE http_active_connections gauge\n")
	sb.WriteString("http_active_connections ")
	sb.WriteString(strconv.FormatInt(metrics.GlobalMetrics.ActiveConns.Value(), 10))
	sb.WriteByte('\n')

	sb.WriteString("\n# HELP http_request_duration_seconds HTTP request duration in seconds\n")
	sb.WriteString("# TYPE http_request_duration_seconds summary\n")
	metrics.GlobalMetrics.RequestDuration.Range(func(labels string, sum float64, count uint64) {
		sb.WriteString("http_request_duration_seconds_sum{")
		sb.WriteString(formatHTTPLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatFloat(sum, 'f', 6, 64))
		sb.WriteByte('\n')
		sb.WriteString("http_request_duration_seconds_count{")
		sb.WriteString(formatHTTPLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== P0-3 置信度业务指标 =====
	sb.WriteString("\n# HELP confidence_scored_total P0-3 confidence scoring total\n")
	sb.WriteString("# TYPE confidence_scored_total counter\n")
	metrics.GlobalMetrics.ConfidenceScoredTotal.Range(func(labels string, count uint64) {
		sb.WriteString("confidence_scored_total{")
		sb.WriteString(formatConfidenceLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP confidence_transfer_total P0-3 transfer-to-human decisions\n")
	sb.WriteString("# TYPE confidence_transfer_total counter\n")
	metrics.GlobalMetrics.ConfidenceTransferTotal.Range(func(labels string, count uint64) {
		sb.WriteString("confidence_transfer_total{")
		sb.WriteString(formatConfidenceLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP confidence_auto_reply_total P0-3 auto-reply decisions\n")
	sb.WriteString("# TYPE confidence_auto_reply_total counter\n")
	metrics.GlobalMetrics.ConfidenceAutoReplyTotal.Range(func(labels string, count uint64) {
		sb.WriteString("confidence_auto_reply_total{")
		sb.WriteString(formatConfidenceLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== P0-4 拟人度业务指标 =====
	sb.WriteString("\n# HELP humanize_scored_total P0-4 humanize evaluation total\n")
	sb.WriteString("# TYPE humanize_scored_total counter\n")
	metrics.GlobalMetrics.HumanizeScoredTotal.Range(func(labels string, count uint64) {
		sb.WriteString("humanize_scored_total{")
		sb.WriteString(formatHumanizeLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP humanize_regenerate_total P0-4 regenerate triggered\n")
	sb.WriteString("# TYPE humanize_regenerate_total counter\n")
	metrics.GlobalMetrics.HumanizeRegenerateTotal.Range(func(labels string, count uint64) {
		sb.WriteString("humanize_regenerate_total{")
		sb.WriteString(formatHumanizeLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP humanize_score_value P0-4 score value distribution\n")
	sb.WriteString("# TYPE humanize_score_value summary\n")
	metrics.GlobalMetrics.HumanizeScoreValue.Range(func(labels string, sum float64, count uint64) {
		sb.WriteString("humanize_score_value_sum{")
		sb.WriteString(formatHumanizeLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatFloat(sum, 'f', 6, 64))
		sb.WriteByte('\n')
		sb.WriteString("humanize_score_value_count{")
		sb.WriteString(formatHumanizeLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== P0-5 反馈学习业务指标 =====
	sb.WriteString("\n# HELP feedback_events_total P0-5 feedback events ingested\n")
	sb.WriteString("# TYPE feedback_events_total counter\n")
	metrics.GlobalMetrics.FeedbackEventsTotal.Range(func(labels string, count uint64) {
		sb.WriteString("feedback_events_total{")
		sb.WriteString(formatFeedbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP bandit_samples_total P0-5 Bandit arm samples\n")
	sb.WriteString("# TYPE bandit_samples_total counter\n")
	metrics.GlobalMetrics.FeedbackBanditSamplesTotal.Range(func(labels string, count uint64) {
		sb.WriteString("bandit_samples_total{")
		sb.WriteString(formatFeedbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP bandit_rewards P0-5 Bandit rewards\n")
	sb.WriteString("# TYPE bandit_rewards summary\n")
	metrics.GlobalMetrics.FeedbackBanditRewardsTotal.Range(func(labels string, sum float64, count uint64) {
		sb.WriteString("bandit_rewards_sum{")
		sb.WriteString(formatFeedbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatFloat(sum, 'f', 6, 64))
		sb.WriteByte('\n')
		sb.WriteString("bandit_rewards_count{")
		sb.WriteString(formatFeedbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP prompt_candidates_total P0-5 Prompt candidates generated\n")
	sb.WriteString("# TYPE prompt_candidates_total counter\n")
	metrics.GlobalMetrics.FeedbackPromptCandidatesTotal.Range(func(labels string, count uint64) {
		sb.WriteString("prompt_candidates_total{")
		sb.WriteString(formatFeedbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== R7/R3 出站幂等指标 =====
	sb.WriteString("\n# HELP outbound_total AI 出站消息计数（按渠道与结果）\n")
	sb.WriteString("# TYPE outbound_total counter\n")
	metrics.GlobalMetrics.OutboundTotal.Range(func(labels string, count uint64) {
		sb.WriteString("outbound_total{")
		sb.WriteString(formatOutboundLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== R9 平台 JWT 401 自愈指标 =====
	sb.WriteString("\n# HELP platform_jwt_refresh_total 平台 JWT 失效(401)自愈重试次数\n")
	sb.WriteString("# TYPE platform_jwt_refresh_total counter\n")
	metrics.GlobalMetrics.PlatformJWTRefreshTotal.Range(func(labels string, count uint64) {
		sb.WriteString("platform_jwt_refresh_total{")
		sb.WriteString(formatPathLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== V2 事件总线队列深度 =====
	sb.WriteString("\n# HELP eventbus_queue_depth 事件总线队列积压深度\n")
	sb.WriteString("# TYPE eventbus_queue_depth gauge\n")
	sb.WriteString("eventbus_queue_depth{queue=\"normal\"} ")
	sb.WriteString(strconv.FormatInt(metrics.GlobalMetrics.EventBusNormalDepth.Value(), 10))
	sb.WriteByte('\n')
	sb.WriteString("eventbus_queue_depth{queue=\"critical\"} ")
	sb.WriteString(strconv.FormatInt(metrics.GlobalMetrics.EventBusCriticalDepth.Value(), 10))
	sb.WriteByte('\n')

	// ===== R9 webhook 去重指标 =====
	sb.WriteString("\n# HELP webhook_dedup_total webhook 入站去重计数\n")
	sb.WriteString("# TYPE webhook_dedup_total counter\n")
	metrics.GlobalMetrics.WebhookDedupTotal.Range(func(labels string, count uint64) {
		sb.WriteString("webhook_dedup_total{")
		sb.WriteString(formatOutcomeLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	// ===== 2026-07-31 AI 智能体性能优化 (T21) - 5 个核心指标 =====
	sb.WriteString("\n# HELP ai_agent_wall_time_seconds AI 智能体端到端 wall time (P50/P90 关键指标)\n")
	sb.WriteString("# TYPE ai_agent_wall_time_seconds summary\n")
	metrics.GlobalMetrics.AIAgentWallTime.Range(func(labels string, sum float64, count uint64) {
		sb.WriteString("ai_agent_wall_time_seconds_sum{")
		sb.WriteString(formatAIAgentLayerIntentLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatFloat(sum, 'f', 6, 64))
		sb.WriteByte('\n')
		sb.WriteString("ai_agent_wall_time_seconds_count{")
		sb.WriteString(formatAIAgentLayerIntentLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP ai_agent_lcp_time_seconds AI 智能体流式首字时间 LCP\n")
	sb.WriteString("# TYPE ai_agent_lcp_time_seconds summary\n")
	metrics.GlobalMetrics.AIAgentLCPTime.Range(func(labels string, sum float64, count uint64) {
		sb.WriteString("ai_agent_lcp_time_seconds_sum{")
		sb.WriteString(formatAIAgentLayerIntentLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatFloat(sum, 'f', 6, 64))
		sb.WriteByte('\n')
		sb.WriteString("ai_agent_lcp_time_seconds_count{")
		sb.WriteString(formatAIAgentLayerIntentLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP ai_agent_layer_decision_total AI 智能体双层架构决策 (layer1 vs layer2)\n")
	sb.WriteString("# TYPE ai_agent_layer_decision_total counter\n")
	metrics.GlobalMetrics.AIAgentLayerDecision.Range(func(labels string, count uint64) {
		sb.WriteString("ai_agent_layer_decision_total{")
		sb.WriteString(formatAIAgentLayerIntentLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP ai_agent_llm_call_total AI 智能体 LLM 调用次数\n")
	sb.WriteString("# TYPE ai_agent_llm_call_total counter\n")
	metrics.GlobalMetrics.AIAgentLLMCall.Range(func(labels string, count uint64) {
		sb.WriteString("ai_agent_llm_call_total{")
		sb.WriteString(formatAIAgentScenarioModelLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	sb.WriteString("\n# HELP ai_agent_fallback_total AI 智能体降级链触发次数 (7B→3B→Cache→Template)\n")
	sb.WriteString("# TYPE ai_agent_fallback_total counter\n")
	metrics.GlobalMetrics.AIAgentFallback.Range(func(labels string, count uint64) {
		sb.WriteString("ai_agent_fallback_total{")
		sb.WriteString(formatAIAgentFallbackLabels(labels))
		sb.WriteString("} ")
		sb.WriteString(strconv.FormatUint(count, 10))
		sb.WriteByte('\n')
	})

	c.String(200, sb.String())
}

// formatOutboundLabels 出站指标 label 格式化（channel|result）
func formatOutboundLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 24)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`channel="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`result="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatPathLabels 单标签 path 格式化
func formatPathLabels(labels string) string {
	var b strings.Builder
	b.WriteString(`path="`)
	b.WriteString(escapeLabelValue(labels))
	b.WriteByte('"')
	return b.String()
}

// formatOutcomeLabels 单标签 outcome 格式化
func formatOutcomeLabels(labels string) string {
	var b strings.Builder
	b.WriteString(`outcome="`)
	b.WriteString(escapeLabelValue(labels))
	b.WriteByte('"')
	return b.String()
}

// escapeLabelValue 转义 Prometheus 标签值。
//
// Prometheus exposition format 要求标签值中的 `\` `"` `\n` 必须分别转义为
// `\\` `\"` `\n`（字面量反斜杠+n）。未转义会导致 scrape 解析失败。
// 参考: https://prometheus.io/docs/instrumenting/exposition_formats/#text-format-details
func escapeLabelValue(v string) string {
	if !strings.ContainsAny(v, "\\\"\n") {
		return v
	}
	// 预分配：原长度 + 转义字符上限
	var b strings.Builder
	b.Grow(len(v) + 4)
	for _, r := range v {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatHTTPLabels HTTP 指标 label 格式化（method, path, status）
func formatHTTPLabels(labels string) string {
	parts := splitLabels(labels)
	// 使用 strings.Builder 避免多次字符串拼接分配
	var b strings.Builder
	b.Grow(len(parts) * 24)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`method="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`path="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 2:
			b.WriteString(`status="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatConfidenceLabels 置信度指标 label 格式化
//
// 输入格式："scenario" 或 "scenario|decision"
func formatConfidenceLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 20)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`scenario="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`decision="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatHumanizeLabels 拟人度指标 label 格式化
//
// 输入格式："evaluator_type" 或 "evaluator_type|sample_strategy"
func formatHumanizeLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 20)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`evaluator="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`strategy="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatFeedbackLabels 反馈学习指标 label 格式化
//
// 输入格式："event_type" 或 "event_type|signal_key" 或 "arm_key"
func formatFeedbackLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 20)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`type="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`key="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// splitLabels 将 "a|b|c" 拆分为 ["a","b","c"]。
//
// 兼容空字符串输入（strings.Split("", "|") 返回 [""]），与原实现保持一致。
func splitLabels(s string) []string {
	// 复用标准库 strings.Split 的高效实现，避免手写 rune 循环导致的多次字符串分配
	return strings.Split(s, "|")
}

// formatAIAgentLayerIntentLabels AI 智能体 wall time / lcp / layer_decision 指标 label 格式化
//
// 输入格式："agent_type|layer|intent" 或 "agent_type|stream_mode"
func formatAIAgentLayerIntentLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 24)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`agent_type="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`layer="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 2:
			b.WriteString(`intent="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 3:
			b.WriteString(`stream_mode="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatAIAgentScenarioModelLabels AI 智能体 LLM 调用指标 label 格式化
//
// 输入格式："scenario|model|result"
func formatAIAgentScenarioModelLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 24)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`scenario="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`model="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 2:
			b.WriteString(`result="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}

// formatAIAgentFallbackLabels AI 智能体降级链指标 label 格式化
//
// 输入格式："from_layer|to_layer|reason"
func formatAIAgentFallbackLabels(labels string) string {
	parts := splitLabels(labels)
	var b strings.Builder
	b.Grow(len(parts) * 24)
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(',')
		}
		switch i {
		case 0:
			b.WriteString(`from_layer="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 1:
			b.WriteString(`to_layer="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		case 2:
			b.WriteString(`reason="`)
			b.WriteString(escapeLabelValue(p))
			b.WriteByte('"')
		}
	}
	return b.String()
}
