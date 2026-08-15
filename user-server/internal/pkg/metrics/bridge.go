// 桥接架构专用指标（2026-08-15 M3-P1-E3 指标采集）
//
// 命名规范：bridge_<module>_<metric>_<unit>
//   - bridge_ingest_total          Counter   桥接 ingest 请求总数
//   - bridge_ingest_errors_total   Counter   ingest 错误数（按错误码分类）
//   - bridge_ingest_duration_ms    Histogram ingest 耗时分布
//   - bridge_outbox_fetched_total  Counter   outbox 下行拉取消息总数
//   - bridge_outbox_duration_ms    Histogram outbox 拉取耗时分布
//   - bridge_outbox_acked_total    Counter   outbox ack 数（按状态分类）
//   - bridge_ack_duration_ms       Histogram ack 耗时分布
//   -bridge_circuit_breaker_state  Gauge     熔断器状态（0=CLOSED 1=HALF_OPEN 2=OPEN）
//   - bridge_pending_ack_size      Gauge     pendingAck 队列长度
//   - bridge_pending_dead_letters  Gauge     dead letters 队列长度
//   - bridge_dlq_total             Counter   DLQ 累计（按 channel）
//   - bridge_emergency_stop        Gauge     紧急停止状态（0/1）
//   - bridge_idempotency_hits      Counter   幂等命中数（去重）
//   - bridge_pii_redactions_total  Counter   PII 脱敏次数
package metrics

import (
	"sync"
)

// BridgeMetrics 桥接指标聚合（懒初始化）
type BridgeMetrics struct {
	IngestTotal         *CounterVec
	IngestErrors        *CounterVec
	IngestDuration      *HistogramVec
	OutboxFetched       *CounterVec
	OutboxDuration      *HistogramVec
	OutboxAcked         *CounterVec
	AckDuration         *HistogramVec
	CircuitBreakerState *GaugeVec
	PendingAckSize      *GaugeVec
	PendingDeadLetters  *GaugeVec
	DLQTotal            *CounterVec
	EmergencyStop       *GaugeVec
	IdempotencyHits     *CounterVec
	PIIRedactions       *CounterVec
}

var (
	bridgeOnce sync.Once
	bridge     *BridgeMetrics
)

// GetBridge 获取桥接指标（懒初始化，全局单例）
func GetBridge() *BridgeMetrics {
	bridgeOnce.Do(func() {
		bridge = &BridgeMetrics{
			IngestTotal: NewCounter(
				"bridge_ingest_total",
				"Total bridge ingest requests (POST /api/bridge/ingest)",
				[]string{"channel", "agent_id"},
			),
			IngestErrors: NewCounter(
				"bridge_ingest_errors_total",
				"Total bridge ingest errors by error code",
				[]string{"channel", "error_code"},
			),
			IngestDuration: NewHistogram(
				"bridge_ingest_duration_ms",
				"Bridge ingest request duration in ms",
				[]string{"channel"},
				[]float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			),
			OutboxFetched: NewCounter(
				"bridge_outbox_fetched_total",
				"Total outbox messages fetched (GET /api/bridge/outbox)",
				[]string{"channel", "agent_id"},
			),
			OutboxDuration: NewHistogram(
				"bridge_outbox_duration_ms",
				"Bridge outbox fetch request duration in ms",
				[]string{"channel"},
				[]float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			),
			OutboxAcked: NewCounter(
				"bridge_outbox_acked_total",
				"Total outbox ack results by status",
				[]string{"channel", "status"},
			),
			AckDuration: NewHistogram(
				"bridge_ack_duration_ms",
				"Bridge ack request duration in ms",
				[]string{"channel"},
				[]float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			),
			CircuitBreakerState: NewGauge(
				"bridge_circuit_breaker_state",
				"Circuit breaker state (0=CLOSED 1=HALF_OPEN 2=OPEN)",
				[]string{"channel"},
			),
			PendingAckSize: NewGauge(
				"bridge_pending_ack_size",
				"Current pendingAck queue size (waiting for ack)",
				[]string{"channel"},
			),
			PendingDeadLetters: NewGauge(
				"bridge_pending_dead_letters",
				"Current dead letters queue size (gave up retry)",
				[]string{"channel"},
			),
			DLQTotal: NewCounter(
				"bridge_dlq_total",
				"Total messages sent to DLQ (dead letter queue)",
				[]string{"channel", "reason"},
			),
			EmergencyStop: NewGauge(
				"bridge_emergency_stop",
				"Emergency stop status (0=active 1=stopped)",
				[]string{},
			),
			IdempotencyHits: NewCounter(
				"bridge_idempotency_hits_total",
				"Total idempotency hits (deduplication)",
				[]string{"endpoint"},
			),
			PIIRedactions: NewCounter(
				"bridge_pii_redactions_total",
				"Total PII redactions (sanitization)",
				[]string{"field"},
			),
		}
	})
	return bridge
}


