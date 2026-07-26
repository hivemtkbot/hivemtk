// Package metrics 业务指标埋点
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md P3 监控指标
//
// 解决 import cycle：
//
//	middleware 包内某些文件（app_key_auth）依赖 service 包；
//	service 包需要调用指标埋点 → 通过本包隔离依赖
//
// 私域独立部署：无 merchant_id 字段
//
// 指标列表（/metrics 端点暴露）：
//   - http_requests_total{method,path,status}
//   - http_active_connections
//   - http_request_duration_seconds{method,path}
//   - confidence_scored_total{scenario}
//   - confidence_transfer_total{scenario}
//   - confidence_auto_reply_total{scenario}
//   - humanize_scored_total{evaluator,strategy}
//   - humanize_regenerate_total{evaluator}
//   - humanize_score_value_sum / count{evaluator}
//   - feedback_events_total{type,key}
//   - bandit_samples_total{arm_key}
//   - bandit_rewards_sum / count{arm_key}
//   - prompt_candidates_total{scenario}
package metrics

import "sync"

// ============================================================================
// 基础向量类型（CounterVec / HistogramVec / Gauge / SummaryVec）
// ============================================================================

// CounterVec 计数器向量
type CounterVec struct {
	mu     sync.RWMutex
	values map[string]uint64
}

// NewCounterVec 构造一个空的 CounterVec
func NewCounterVec() *CounterVec {
	return &CounterVec{values: make(map[string]uint64)}
}

func (c *CounterVec) Inc(labels string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = make(map[string]uint64)
	}
	c.values[labels]++
}

func (c *CounterVec) Value(labels string) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values[labels]
}

// Range 遍历所有 labels。
//
// 注意：回调 f 在调用前先复制一份快照，避免长时间持有读锁阻塞 Inc 热路径
// （例如 /metrics 端点执行字符串拼接时可能耗时较长）。
func (c *CounterVec) Range(f func(labels string, value uint64)) {
	type entry struct {
		labels string
		value  uint64
	}
	c.mu.RLock()
	snapshot := make([]entry, 0, len(c.values))
	for labels, value := range c.values {
		snapshot = append(snapshot, entry{labels: labels, value: value})
	}
	c.mu.RUnlock()
	for _, e := range snapshot {
		f(e.labels, e.value)
	}
}

// HistogramVec 直方图向量
//
// 使用 sum/count 聚合，Range 消费 sums/counts，
// 与 Prometheus Summary 的 _sum/_count 输出语义一致。
type HistogramVec struct {
	mu     sync.RWMutex
	sums   map[string]float64
	counts map[string]uint64
}

// NewHistogramVec 构造一个空的 HistogramVec
func NewHistogramVec() *HistogramVec {
	return &HistogramVec{
		sums:   make(map[string]float64),
		counts: make(map[string]uint64),
	}
}

func (h *HistogramVec) Observe(labels string, value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sums == nil {
		h.sums = make(map[string]float64)
		h.counts = make(map[string]uint64)
	}
	h.sums[labels] += value
	h.counts[labels]++
}

// Range 遍历所有 labels（sums + counts）。
//
// 注意：回调 f 在调用前先复制一份快照，避免长时间持有读锁阻塞 Inc/Observe 热路径。
func (h *HistogramVec) Range(f func(labels string, sum float64, count uint64)) {
	type entry struct {
		labels string
		sum    float64
		count  uint64
	}
	h.mu.RLock()
	snapshot := make([]entry, 0, len(h.sums))
	for labels, sum := range h.sums {
		snapshot = append(snapshot, entry{labels: labels, sum: sum, count: h.counts[labels]})
	}
	h.mu.RUnlock()
	for _, e := range snapshot {
		f(e.labels, e.sum, e.count)
	}
}

// Gauge 仪表盘
type Gauge struct {
	mu    sync.RWMutex
	value int64
}

func (g *Gauge) Inc() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value++
}

func (g *Gauge) Dec() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value--
}

func (g *Gauge) Set(v int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = v
}

func (g *Gauge) Value() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// SummaryVec 摘要向量
type SummaryVec struct {
	mu     sync.RWMutex
	sums   map[string]float64
	counts map[string]uint64
}

func (s *SummaryVec) Observe(labels string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sums == nil {
		s.sums = make(map[string]float64)
		s.counts = make(map[string]uint64)
	}
	s.sums[labels] += value
	s.counts[labels]++
}

// Range 遍历所有 labels（sums + counts）。
//
// 注意：回调 f 在调用前先复制一份快照，避免长时间持有读锁阻塞 Observe 热路径。
func (s *SummaryVec) Range(f func(labels string, sum float64, count uint64)) {
	type entry struct {
		labels string
		sum    float64
		count  uint64
	}
	s.mu.RLock()
	snapshot := make([]entry, 0, len(s.sums))
	for labels, sum := range s.sums {
		snapshot = append(snapshot, entry{labels: labels, sum: sum, count: s.counts[labels]})
	}
	s.mu.RUnlock()
	for _, e := range snapshot {
		f(e.labels, e.sum, e.count)
	}
}

// ============================================================================
// 指标集合
// ============================================================================

// MetricsCollector 全局指标集合
type MetricsCollector struct {
	// 通用 HTTP 指标
	RequestTotal    *CounterVec
	RequestDuration *HistogramVec
	ActiveConns     *Gauge
	RequestSize     *SummaryVec
	ResponseSize    *SummaryVec
	// P0-3 置信度
	ConfidenceScoredTotal    *CounterVec
	ConfidenceTransferTotal  *CounterVec
	ConfidenceAutoReplyTotal *CounterVec
	// P0-4 拟人度
	HumanizeScoredTotal     *CounterVec
	HumanizeRegenerateTotal *CounterVec
	HumanizeScoreValue      *SummaryVec
	// P0-5 反馈学习
	FeedbackEventsTotal           *CounterVec
	FeedbackBanditSamplesTotal    *CounterVec
	FeedbackBanditRewardsTotal    *SummaryVec
	FeedbackPromptCandidatesTotal *CounterVec
	// R7/R3 出站幂等（架构评审整改）
	OutboundTotal *CounterVec // channel|result(success|duplicate|failed)
	// R9 可观测性：平台 JWT 401 自愈次数（架构评审 R9）
	PlatformJWTRefreshTotal *CounterVec // path
	// V2 事件总线队列深度（架构评审 V2/R9）
	EventBusNormalDepth   *Gauge
	EventBusCriticalDepth *Gauge
	// 架构评审 R9：webhook 去重计数
	WebhookDedupTotal *CounterVec // outcome(duplicate|accepted)
	// P1-A：智能体工具调用指标（深度审查第二轮 P1-A）
	// labels: tool_name|result(success|failed|panic)
	// 用于监控工具调用频率、失败率、按工具维度分析
	ToolCallTotal    *CounterVec
	ToolCallDuration *HistogramVec // labels: tool_name
	ToolCallErrors   *CounterVec   // labels: tool_name|error_type(permission|ratelimit|timeout|panic|internal)
}

// GlobalMetrics 全局指标收集器
var GlobalMetrics = &MetricsCollector{
	RequestTotal:                  &CounterVec{values: make(map[string]uint64)},
	RequestDuration:               &HistogramVec{},
	ActiveConns:                   &Gauge{},
	RequestSize:                   &SummaryVec{},
	ResponseSize:                  &SummaryVec{},
	ConfidenceScoredTotal:         &CounterVec{values: make(map[string]uint64)},
	ConfidenceTransferTotal:       &CounterVec{values: make(map[string]uint64)},
	ConfidenceAutoReplyTotal:      &CounterVec{values: make(map[string]uint64)},
	HumanizeScoredTotal:           &CounterVec{values: make(map[string]uint64)},
	HumanizeRegenerateTotal:       &CounterVec{values: make(map[string]uint64)},
	HumanizeScoreValue:            &SummaryVec{},
	FeedbackEventsTotal:           &CounterVec{values: make(map[string]uint64)},
	FeedbackBanditSamplesTotal:    &CounterVec{values: make(map[string]uint64)},
	FeedbackBanditRewardsTotal:    &SummaryVec{},
	FeedbackPromptCandidatesTotal: &CounterVec{values: make(map[string]uint64)},
	OutboundTotal:                 &CounterVec{values: make(map[string]uint64)},
	PlatformJWTRefreshTotal:       &CounterVec{values: make(map[string]uint64)},
	EventBusNormalDepth:           &Gauge{},
	EventBusCriticalDepth:         &Gauge{},
	WebhookDedupTotal:             &CounterVec{values: make(map[string]uint64)},
	// P1-A：智能体工具调用指标初始化
	ToolCallTotal:    &CounterVec{values: make(map[string]uint64)},
	ToolCallDuration: &HistogramVec{sums: make(map[string]float64), counts: make(map[string]uint64)},
	ToolCallErrors:   &CounterVec{values: make(map[string]uint64)},
}
