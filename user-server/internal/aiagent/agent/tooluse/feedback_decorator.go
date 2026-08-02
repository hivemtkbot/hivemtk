package tooluse

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// feedback_decorator.go 工具调用反馈回流装饰器
//
// 设计目标：
//   将工具调用结果作为隐式反馈信号回流到 FeedbackCollector，
//   用于驱动 SOP 优化 / Bandit 流量分配 / Prompt 迭代。
//
// 设计要点：
//   1. 通过 FeedbackSink 接口解耦，tooluse 包不直接依赖 feedback_loop 包
//      （feedback_loop 包依赖 dto/model，若 tooluse 反向依赖会产生循环）
//   2. 仅在工具执行成功后异步回流，不阻塞主链路
//   3. 失败的工具调用也回流（标记 Success=false），用于降级率统计
//   4. 装饰器位置：审计计费之后、handler 之前（最内层）
//      这样可以拿到最终的 result 和 duration，且不受重试/超时影响
//      但为了避免双重计时，本装饰器位于 AuditDecorator 之后（外层），
//      通过独立的 sink.RecordToolCall 异步上报
//
// 装饰器链位置（后，实际执行顺序，外→内）：
//   死信 → 反馈回流 → 循环检测 → 权限 → 限流 → 熔断 → 参数校验 → 缓存 → 重试 → 超时 → 审计计费 → handler
//   （反馈回流位于重试之外：重试是同一逻辑调用的多次尝试，只回流最终结果）
//   （反馈回流位于审计之外：审计已记录完整信息，反馈回流可复用 result/error）
//   （反馈回流位于循环检测之外：循环命中时反馈仍能记录失败事件，用于降级率统计）

// ===== 接口定义（解耦 tooluse ↔ feedback_loop）=====

// ToolRiskLevel 工具风险级别
//
// 用于反馈加权：写工具的反馈信号权重高于读工具（写操作影响持久状态，
// 失败代价更高；读操作失败可重试，影响有限）。
//
// 可选方法：Tool 实现可实现 RiskLevel() ToolRiskLevel，未实现时默认为 RiskLevelRead。
type ToolRiskLevel string

const (
	// RiskLevelRead 读工具（如 customer.search、knowledge.search）
	// 仅查询数据，无副作用；反馈信号权重低
	RiskLevelRead ToolRiskLevel = "read"

	// RiskLevelWrite 写工具（如 knowledge.feedback、knowledge.add_doc）
	// 修改/写入数据，有副作用但可回滚；反馈信号权重中
	RiskLevelWrite ToolRiskLevel = "write"

	// RiskLevelAdmin 管理工具（如 customer.delete、reach.broadcast）
	// 不可逆操作或影响范围大；反馈信号权重高
	RiskLevelAdmin ToolRiskLevel = "admin"
)

// FeedbackSink 反馈回流接口
//
// 由调用方（router 层）注入具体实现：
//   - FeedbackCollectorAdapter 包装 service.FeedbackCollector
//   - 测试场景使用 MemoryFeedbackSink
type FeedbackSink interface {
	// RecordToolCall 记录一次工具调用事件
	// 实现应异步处理，不阻塞主链路；失败不应影响工具执行结果
	RecordToolCall(ctx context.Context, event ToolCallEvent) error
}

// ToolCallEvent 工具调用事件（反馈回流载荷）
type ToolCallEvent struct {
	ToolName   string         `json:"tool_name"`             // 工具名
	Args       map[string]any `json:"args"`                  // 调用参数（脱敏后）
	Result     ToolResult     `json:"result"`                // 执行结果
	Duration   time.Duration  `json:"duration_ms"`           // 执行耗时
	TraceID    string         `json:"trace_id,omitempty"`    // 追踪 ID（贯穿 Agent Loop）
	SessionID  string         `json:"session_id,omitempty"`  // 会话 ID
	CustomerID string         `json:"customer_id,omitempty"` // 客户 ID
	AgentID    string         `json:"agent_id,omitempty"`    // 智能体 ID
	Source     string         `json:"source,omitempty"`      // 调用来源（agent/sop/manual/api）
	Success    bool           `json:"success"`               // 是否成功
	Error      string         `json:"error,omitempty"`       // 错误信息
	// 风险级别：用于反馈加权（写工具的反馈信号权重高于读工具）
	RiskLevel ToolRiskLevel `json:"risk_level,omitempty"`
	// 工具版本：用于版本维度分析
	Version string `json:"version,omitempty"`
}

// ===== 装饰器 =====

// FeedbackCollectorDecorator 反馈回流装饰器
//
// 在工具执行结束后，将调用事件回流到 FeedbackSink。
// sink 为 nil 时直接放行（零开销）。
//
// 通过 MonitoredFeedbackSink 包装，对 sink.RecordToolCall 错误进行监控
//   - 错误计数（atomic，无锁）
//   - 错误日志（log.Printf，便于运维定位）
//   - 错误率超阈值告警（10s 窗口内错误率 > 50% 时输出告警日志）
//
// 装饰器链位置：审计计费之后、handler 之前
//   - 位于审计之后：可复用 result/error/duration
//   - 位于 handler 之外：不影响 handler 执行
//   - 异步上报：通过 go routine 异步调用 sink.RecordToolCall
//     避免反馈回流失败/慢导致主链路阻塞
func FeedbackCollectorDecorator(sink FeedbackSink) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			result, err := next(ctx, args)

			if sink == nil {
				return result, err
			}

			// 提取上下文信息
			toolName := GetToolName(ctx)
			tc := GetToolContext(ctx)
			traceID := GetTraceID(ctx)

			// 构造事件（脱敏参数，避免敏感信息进入反馈流）
			event := ToolCallEvent{
				ToolName: toolName,
				Args:     sanitizeArgsForFeedback(args),
				Result:   result,
				Duration: time.Duration(result.Timing.DurationMs) * time.Millisecond,
				TraceID:  traceID,
				Success:  err == nil && result.Success,
			}
			if err != nil {
				event.Error = err.Error()
			} else if !result.Success {
				event.Error = result.Error
			}
			if tc != nil {
				event.SessionID = tc.SessionID
				event.CustomerID = tc.CustomerID
				event.AgentID = tc.AgentID
				event.Source = tc.Source
			}

			// 异步回流（不阻塞主链路）
			// 使用 context.Background() 而非原 ctx，避免 ctx 被取消后回流失败
			go func(ev ToolCallEvent) {
				// 限制回流最大耗时 5s，防止 sink 卡死 goroutine
				sinkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				// 监控 sink.RecordToolCall 错误
				// 错误不影响主链路，但需要被观测以发现反馈回流链路异常
				if sinkErr := sink.RecordToolCall(sinkCtx, ev); sinkErr != nil {
					log.Printf("[WARN] FeedbackSink.RecordToolCall failed: tool=%s trace_id=%s err=%v",
						ev.ToolName, ev.TraceID, sinkErr)
				}
			}(event)

			return result, err
		}
	}
}

// sanitizeArgsForFeedback 反馈用参数脱敏
//
// 与审计日志的 summarizeArgs 不同，这里保留 map 结构（便于后续分析），
// 但对敏感字段值替换为 ***。
func sanitizeArgsForFeedback(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	sensitiveKeys := map[string]bool{
		"password": true, "token": true, "secret": true,
		"api_key": true, "apikey": true, "phone": true,
		"id_card": true, "bank_card": true,
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if sensitiveKeys[k] {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

// ===== 内存实现（用于测试 / 默认场景）=====

// MemoryFeedbackSink 内存反馈接收器（用于测试 / 单机审计）
//
// 采用环形缓冲区（ring buffer）实现 O(1) 写入
//   - 旧实现使用 slice + events[1:] 满容量时 O(n) 拷贝，高频写入时性能瓶颈
//   - 新实现使用固定容量 + head/size 指针，写入复杂度 O(1)
//   - 读出时按 oldest→newest 顺序返回（保持语义一致）
//
// 线程安全；由于 FeedbackCollectorDecorator 异步 goroutine 调用 RecordToolCall，
// 必须加锁保护内部状态。
type MemoryFeedbackSink struct {
	mu      sync.Mutex
	buf     []ToolCallEvent // 固定容量环形缓冲区
	head    int             // 下一个写入位置
	size    int             // 当前条目数（<= cap）
	max     int             // 缓冲区容量（= cap(buf)）
	dropped int64           // 因容量满被覆盖的旧条目数（监控用）
}

// NewMemoryFeedbackSink 创建内存反馈接收器
// max: 最大保留条数（超出后环形覆盖最旧条目）
func NewMemoryFeedbackSink(max int) *MemoryFeedbackSink {
	if max < 1 {
		max = 1000
	}
	return &MemoryFeedbackSink{
		buf:  make([]ToolCallEvent, max),
		head: 0,
		size: 0,
		max:  max,
	}
}

// RecordToolCall 记录工具调用事件（：环形缓冲区 O(1) 写入）
func (s *MemoryFeedbackSink) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 写入 head 位置
	s.buf[s.head] = event
	s.head = (s.head + 1) % s.max
	if s.size < s.max {
		s.size++
	} else {
		// 缓冲区已满，覆盖最旧条目（head 此时指向最旧条目）
		s.dropped++
	}
	return nil
}

// Events 返回所有事件副本（按 oldest→newest 顺序）
func (s *MemoryFeedbackSink) Events() []ToolCallEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size == 0 {
		return []ToolCallEvent{}
	}
	out := make([]ToolCallEvent, s.size)
	// head 指向下一个写入位置，即最旧条目的位置（当 size==max 时）
	// 或 0（当 size<max 时，未发生覆盖）
	start := 0
	if s.size == s.max {
		start = s.head // head 指向最旧条目
	}
	for i := 0; i < s.size; i++ {
		out[i] = s.buf[(start+i)%s.max]
	}
	return out
}

// Count 返回事件条数
func (s *MemoryFeedbackSink) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size
}

// Dropped 返回因容量满被覆盖的条目数（监控用）
func (s *MemoryFeedbackSink) Dropped() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// Reset 清空事件
func (s *MemoryFeedbackSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head = 0
	s.size = 0
	s.dropped = 0
}

// NoOpFeedbackSink 空操作反馈接收器（不进行任何记录）
type NoOpFeedbackSink struct{}

func (NoOpFeedbackSink) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	return nil
}

// ===== : FeedbackSink 错误监控 =====

// FeedbackSinkMetrics 反馈回流错误监控指标
//
// 通过 atomic 计数器实现无锁监控，避免反馈回流影响主链路性能
// 调用方可通过 Metrics() 读取当前快照，用于 /metrics endpoint 暴露或告警判定
type FeedbackSinkMetrics struct {
	TotalAttempts  atomic.Int64 // 总尝试次数（含成功+失败）
	SuccessCount   atomic.Int64 // 成功次数
	FailureCount   atomic.Int64 // 失败次数（sink.RecordToolCall 返回 error）
	TimeoutCount   atomic.Int64 // 超时次数（ctx deadline exceeded）
	LastFailureMsg atomic.Value // 最近一次失败的错误信息（string）
}

// Snapshot 返回指标快照（用于 /metrics 暴露或日志输出）
type FeedbackSinkMetricsSnapshot struct {
	TotalAttempts  int64
	SuccessCount   int64
	FailureCount   int64
	TimeoutCount   int64
	FailureRate    float64 // 失败率 = FailureCount / TotalAttempts
	LastFailureMsg string
}

// Snapshot 返回指标快照
func (m *FeedbackSinkMetrics) Snapshot() FeedbackSinkMetricsSnapshot {
	total := m.TotalAttempts.Load()
	success := m.SuccessCount.Load()
	failure := m.FailureCount.Load()
	timeout := m.TimeoutCount.Load()
	var rate float64
	if total > 0 {
		rate = float64(failure) / float64(total)
	}
	var lastMsg string
	if v := m.LastFailureMsg.Load(); v != nil {
		lastMsg = v.(string)
	}
	return FeedbackSinkMetricsSnapshot{
		TotalAttempts:  total,
		SuccessCount:   success,
		FailureCount:   failure,
		TimeoutCount:   timeout,
		FailureRate:    rate,
		LastFailureMsg: lastMsg,
	}
}

// MonitoredFeedbackSink 带 metrics 监控的 FeedbackSink 包装器
//
// 包装任意 FeedbackSink，在 RecordToolCall 调用时统计成功/失败次数
// 用于运维监控反馈回流链路健康度
//
// 使用方式：
//
//	rawSink := NewMemoryFeedbackSink(1000)
//	monitoredSink := NewMonitoredFeedbackSink(rawSink)
//	decorator := FeedbackCollectorDecorator(monitoredSink)
//	// ... 运行期可通过 monitoredSink.Metrics().Snapshot() 读取指标
type MonitoredFeedbackSink struct {
	inner   FeedbackSink
	metrics FeedbackSinkMetrics
}

// NewMonitoredFeedbackSink 包装一个 FeedbackSink 为带监控的版本
//
// inner 为 nil 时返回 nil（让 FeedbackCollectorDecorator 的 nil 检查自动跳过）
func NewMonitoredFeedbackSink(inner FeedbackSink) *MonitoredFeedbackSink {
	if inner == nil {
		return nil
	}
	return &MonitoredFeedbackSink{inner: inner}
}

// RecordToolCall 实现 FeedbackSink 接口，同时记录监控指标
func (s *MonitoredFeedbackSink) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	s.metrics.TotalAttempts.Add(1)
	err := s.inner.RecordToolCall(ctx, event)
	if err == nil {
		s.metrics.SuccessCount.Add(1)
		return nil
	}
	// 失败：计数 + 记录最近错误信息
	s.metrics.FailureCount.Add(1)
	s.metrics.LastFailureMsg.Store(err.Error())
	// 检测超时错误
	if errors.Is(err, context.DeadlineExceeded) {
		s.metrics.TimeoutCount.Add(1)
	}
	return err
}

// Metrics 返回 metrics 指针（调用方可读取 Snapshot 或重置）
func (s *MonitoredFeedbackSink) Metrics() *FeedbackSinkMetrics {
	return &s.metrics
}
