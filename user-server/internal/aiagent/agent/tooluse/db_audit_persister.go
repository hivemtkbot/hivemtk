package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// db_audit_persister.go P2-G: 工具调用审计日志 DB 持久化
// P2-H: 告警机制（基于阈值触发）
//
// 设计目标：
//   - P2-G: 将 AuditEntry 持久化到 PostgreSQL，便于长期保留和查询
//   - P2-H: 当工具失败率/熔断状态超过阈值时触发告警（通过回调通知外部系统）
//
// 设计要点：
//   - DB 写入异步（避免阻塞主流程）
//   - 写入失败仅记日志（不影响工具调用结果）
//   - 表结构自管理（AutoMigrate 由调用方在启动时执行）

// ===== DB 模型 =====

// ToolCallAuditRecord 工具调用审计记录（DB 模型）
//
// 表名：tool_call_audits
type ToolCallAuditRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TraceID       string    `gorm:"index;size:64" json:"trace_id"`
	ToolName      string    `gorm:"index;size:128" json:"tool_name"`
	CallerID      string    `gorm:"size:64" json:"caller_id"`
	AgentID       string    `gorm:"size:64" json:"agent_id"`
	CustomerID    string    `gorm:"size:64" json:"customer_id"`
	SessionID     string    `gorm:"index;size:64" json:"session_id"`
	Success       bool      `gorm:"index" json:"success"`
	Error         string    `gorm:"type:text" json:"error"`
	DurationMs    int64     `json:"duration_ms"`
	RetryCount    int       `json:"retry_count"`
	AuditTrace    string    `gorm:"size:128" json:"audit_trace"`
	ArgsSummary   string    `gorm:"type:text" json:"args_summary"`
	ResultSummary string    `gorm:"type:text" json:"result_summary"`
	ExecutedAt    time.Time `gorm:"index" json:"executed_at"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (ToolCallAuditRecord) TableName() string {
	return "tool_call_audits"
}

// ===== DB 持久化 AuditLogger =====

// DBAuditLogger DB 持久化 AuditLogger
//
// 实现 AuditLogger 接口，将审计日志写入 PostgreSQL
// 写入异步执行（避免阻塞主流程）
type DBAuditLogger struct {
	db       *gorm.DB
	queue    chan AuditEntry
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopCh   chan struct{}
	// fallback logger：DB 写入失败时降级到内存 logger
	fallback AuditLogger
}

// NewDBAuditLogger 创建 DB 持久化 AuditLogger
//
// 参数：
//   - db: PostgreSQL 连接
//   - queueSize: 异步队列大小（默认 10000）
//   - fallback: DB 写入失败时的降级 logger（可为 nil）
func NewDBAuditLogger(db *gorm.DB, queueSize int, fallback AuditLogger) *DBAuditLogger {
	if queueSize <= 0 {
		queueSize = 10000
	}
	l := &DBAuditLogger{
		db:       db,
		queue:    make(chan AuditEntry, queueSize),
		stopCh:   make(chan struct{}),
		fallback: fallback,
	}
	l.wg.Add(1)
	go l.consume()
	return l
}

// Log 实现 AuditLogger 接口
//
// 异步写入：将 entry 推入队列，由后台 goroutine 批量写入 DB
// 队列满时降级到 fallback logger（避免丢失审计日志）
func (l *DBAuditLogger) Log(ctx context.Context, entry AuditEntry) {
	select {
	case l.queue <- entry:
		// 成功入队
	default:
		// 队列满：降级到 fallback
		if l.fallback != nil {
			l.fallback.Log(ctx, entry)
		}
	}
}

// consume 后台消费 goroutine
//
// 批量从队列读取 entry，写入 DB（每 100 条或 1 秒刷新一次）
// 退出时（Close 触发 stopCh）确保：
//  1. flush 当前 batch
//  2. drain 队列剩余 entry 并 flush（避免丢日志）
func (l *DBAuditLogger) consume() {
	defer l.wg.Done()

	const batchSize = 100
	const flushInterval = time.Second
	batch := make([]AuditEntry, 0, batchSize)
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()

	for {
		select {
		case entry := <-l.queue:
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				l.flushBatch(batch)
				batch = batch[:0]
			}
		case <-timer.C:
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(flushInterval)
		case <-l.stopCh:
			// 退出前 drain 队列剩余 entry（避免丢日志）
			// 1. 先 drain queue
			drained := 0
			for {
				select {
				case entry := <-l.queue:
					batch = append(batch, entry)
					drained++
					if len(batch) >= batchSize {
						l.flushBatch(batch)
						batch = batch[:0]
					}
				default:
					goto flushAndExit
				}
			}
		flushAndExit:
			// 2. flush 剩余 batch
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			if drained > 0 {
				// 仅用于调试观察（不输出避免依赖 logger）
				_ = drained
			}
			return
		}
	}
}

// flushBatch 批量写入 DB
//
// 当 db == nil 时（DB 未启用），降级到 fallback logger
// 当 DB 写入失败时，也降级到 fallback
func (l *DBAuditLogger) flushBatch(batch []AuditEntry) {
	if len(batch) == 0 {
		return
	}
	// DB 未启用：直接降级到 fallback
	if l.db == nil {
		if l.fallback != nil {
			ctx := context.Background()
			for _, e := range batch {
				l.fallback.Log(ctx, e)
			}
		}
		return
	}
	// 转换为 DB 模型
	records := make([]ToolCallAuditRecord, 0, len(batch))
	for _, e := range batch {
		records = append(records, auditEntryToRecord(e))
	}
	// 批量插入
	if err := l.db.CreateInBatches(records, 100).Error; err != nil {
		// DB 写入失败：降级到 fallback
		if l.fallback != nil {
			ctx := context.Background()
			for _, e := range batch {
				l.fallback.Log(ctx, e)
			}
		}
	}
}

// Close 优雅关闭：等待队列消费完毕
func (l *DBAuditLogger) Close() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
		l.wg.Wait()
	})
}

// auditEntryToRecord 将 AuditEntry 转为 DB 模型
func auditEntryToRecord(e AuditEntry) ToolCallAuditRecord {
	return ToolCallAuditRecord{
		TraceID:       e.TraceID,
		ToolName:      e.ToolName,
		CallerID:      e.CallerID,
		AgentID:       e.AgentID,
		CustomerID:    e.CustomerID,
		SessionID:     e.SessionID,
		Success:       e.Success,
		Error:         e.Error,
		DurationMs:    int64(e.Duration / time.Millisecond),
		RetryCount:    e.RetryCount,
		AuditTrace:    e.AuditTrace,
		ArgsSummary:   e.ArgsSummary,
		ResultSummary: e.ResultSummary,
		ExecutedAt:    e.ExecutedAt,
	}
}

// AutoMigrateAuditTable 自动迁移审计表（启动时调用）
func AutoMigrateAuditTable(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.AutoMigrate(&ToolCallAuditRecord{})
}

// ===== 告警机制 =====

// AlertLevel 告警级别
type AlertLevel string

const (
	// AlertInfo 信息级
	AlertInfo AlertLevel = "info"
	// AlertWarning 警告级
	AlertWarning AlertLevel = "warning"
	// AlertCritical 严重级
	AlertCritical AlertLevel = "critical"
)

// AlertEvent 告警事件
type AlertEvent struct {
	Level     AlertLevel `json:"level"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	ToolName  string     `json:"tool_name,omitempty"`
	TraceID   string     `json:"trace_id,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
	// 扩展字段（便于告警接收方附加业务信息）
	Extra map[string]any `json:"extra,omitempty"`
}

// AlertHandler 告警处理回调
//
// 接收告警事件，由调用方实现具体通知逻辑
//   - 邮件通知
//   - 钉钉机器人
//   - 飞书群机器人
//   - Slack
//   - Prometheus AlertManager
type AlertHandler interface {
	OnAlert(event AlertEvent)
}

// AlertHandlerFunc 函数式 AlertHandler
type AlertHandlerFunc func(event AlertEvent)

func (f AlertHandlerFunc) OnAlert(event AlertEvent) { f(event) }

// AlertManager 告警管理器
//
// 基于规则触发告警：
//  1. 工具失败率超过阈值
//  2. 熔断器开启
//  3. 死信队列堆积
//  4. 单次工具调用耗时过长
type AlertManager struct {
	mu             sync.Mutex
	handlers       []AlertHandler
	failureRateMap map[string]*failureRateTracker // 按 tool_name 统计失败率
}

// failureRateTracker 失败率追踪器
type failureRateTracker struct {
	total       int
	failed      int
	windowStart time.Time
}

// NewAlertManager 创建告警管理器
func NewAlertManager() *AlertManager {
	return &AlertManager{
		failureRateMap: make(map[string]*failureRateTracker),
	}
}

// AddHandler 添加告警处理回调
func (a *AlertManager) AddHandler(handler AlertHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handlers = append(a.handlers, handler)
}

// OnToolCall 工具调用后回调（由 AuditDecorator 调用）
//
// 触发条件：
//   - 失败率超过 50%（窗口 1 分钟，至少 10 次调用）
//   - 熔断器开启（由 CircuitBreakerDecorator 直接触发）
//   - 单次调用耗时 > 5s
func (a *AlertManager) OnToolCall(entry AuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	toolName := entry.ToolName
	tracker, ok := a.failureRateMap[toolName]
	if !ok {
		tracker = &failureRateTracker{windowStart: time.Now()}
		a.failureRateMap[toolName] = tracker
	}

	// 窗口重置（1 分钟）
	if time.Since(tracker.windowStart) > time.Minute {
		tracker.total = 0
		tracker.failed = 0
		tracker.windowStart = time.Now()
	}

	tracker.total++
	if !entry.Success {
		tracker.failed++
	}

	// 规则 1：失败率 > 50% 且总调用 >= 10
	if tracker.total >= 10 {
		rate := float64(tracker.failed) / float64(tracker.total)
		if rate > 0.5 {
			a.emitAlert(AlertEvent{
				Level:    AlertWarning,
				Title:    fmt.Sprintf("工具 %s 失败率过高", toolName),
				Message:  fmt.Sprintf("失败率 %.2f%%（%d/%d），最近 1 分钟", rate*100, tracker.failed, tracker.total),
				ToolName: toolName,
				TraceID:  entry.TraceID,
				Extra: map[string]any{
					"failure_rate": rate,
					"total_calls":  tracker.total,
					"failed_calls": tracker.failed,
				},
			})
		}
	}

	// 规则 2：单次调用耗时 > 5s
	if entry.Duration > 5*time.Second {
		a.emitAlert(AlertEvent{
			Level:    AlertWarning,
			Title:    fmt.Sprintf("工具 %s 调用耗时过长", toolName),
			Message:  fmt.Sprintf("耗时 %v，超过 5s 阈值", entry.Duration),
			ToolName: toolName,
			TraceID:  entry.TraceID,
			Extra: map[string]any{
				"duration_ms": entry.Duration.Milliseconds(),
			},
		})
	}
}

// AlertCircuitOpen 触发熔断器开启告警（由 CircuitBreakerDecorator 调用）
func (a *AlertManager) AlertCircuitOpen(toolName string, state CircuitState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.emitAlert(AlertEvent{
		Level:    AlertCritical,
		Title:    fmt.Sprintf("工具 %s 熔断器开启", toolName),
		Message:  fmt.Sprintf("熔断器状态：%s，连续失败达到阈值，请检查下游服务", state),
		ToolName: toolName,
		Extra: map[string]any{
			"circuit_state": state.String(),
		},
	})
}

// AlertDeadLetterBacklog 触发死信队列堆积告警
func (a *AlertManager) AlertDeadLetterBacklog(toolName string, backlogCount int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	level := AlertWarning
	if backlogCount > 100 {
		level = AlertCritical
	}
	a.emitAlert(AlertEvent{
		Level:    level,
		Title:    fmt.Sprintf("工具 %s 死信队列堆积", toolName),
		Message:  fmt.Sprintf("待处理死信 %d 条，请及时排查", backlogCount),
		ToolName: toolName,
		Extra: map[string]any{
			"backlog_count": backlogCount,
		},
	})
}

// emitAlert 内部触发告警（异步通知所有 handler）
func (a *AlertManager) emitAlert(event AlertEvent) {
	event.Timestamp = time.Now()
	for _, handler := range a.handlers {
		go func(h AlertHandler) {
			defer func() {
				// 防止 handler panic 影响其他 handler
				_ = recover()
			}()
			h.OnAlert(event)
		}(handler)
	}
}

// Stats 返回各工具的失败率统计（用于 /metrics endpoint）
func (a *AlertManager) Stats() map[string]map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]map[string]any, len(a.failureRateMap))
	for tool, tracker := range a.failureRateMap {
		rate := 0.0
		if tracker.total > 0 {
			rate = float64(tracker.failed) / float64(tracker.total)
		}
		out[tool] = map[string]any{
			"total_calls":  tracker.total,
			"failed_calls": tracker.failed,
			"failure_rate": rate,
			"window_start": tracker.windowStart,
		}
	}
	return out
}

// ===== DB AuditLogger + AlertManager 集成 =====

// CompositeAuditLogger 复合 AuditLogger
//
// 同时将审计日志写入多个目标：
//   - 内存（NewMemoryAuditLogger，便于即时查询）
//   - DB（DBAuditLogger，便于持久化）
//   - 告警管理器（AlertManager，触发告警）
type CompositeAuditLogger struct {
	loggers []AuditLogger
	alert   *AlertManager
}

// NewCompositeAuditLogger 创建复合 AuditLogger
func NewCompositeAuditLogger(memory AuditLogger, dbLogger *DBAuditLogger, alert *AlertManager) *CompositeAuditLogger {
	c := &CompositeAuditLogger{alert: alert}
	if memory != nil {
		c.loggers = append(c.loggers, memory)
	}
	if dbLogger != nil {
		c.loggers = append(c.loggers, dbLogger)
	}
	return c
}

// Log 同时写入所有 logger + 触发告警
func (c *CompositeAuditLogger) Log(ctx context.Context, entry AuditEntry) {
	for _, l := range c.loggers {
		l.Log(ctx, entry)
	}
	if c.alert != nil {
		c.alert.OnToolCall(entry)
	}
}

// MarshalJSON 兼容 JSON 序列化（用于调试）
func (e AlertEvent) MarshalJSON() ([]byte, error) {
	type alias AlertEvent
	return json.Marshal(alias(e))
}
