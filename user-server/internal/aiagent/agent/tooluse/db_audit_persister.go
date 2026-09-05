package tooluse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

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
	default:
		if l.fallback != nil {
			l.fallback.Log(ctx, entry)
		}
	}
}

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
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			if drained > 0 {
				_ = drained
			}
			return
		}
	}
}

func (l *DBAuditLogger) flushBatch(batch []AuditEntry) {
	if len(batch) == 0 {
		return
	}
	if l.db == nil {
		if l.fallback != nil {
			ctx := context.Background()
			for _, e := range batch {
				l.fallback.Log(ctx, e)
			}
		}
		return
	}
	records := make([]ToolCallAuditRecord, 0, len(batch))
	for _, e := range batch {
		records = append(records, auditEntryToRecord(e))
	}
	if err := l.db.CreateInBatches(records, 100).Error; err != nil {
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

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertInfo     AlertLevel = "info"
	AlertWarning  AlertLevel = "warning"
	AlertCritical AlertLevel = "critical"
)

// AlertEvent 告警事件
type AlertEvent struct {
	Level     AlertLevel     `json:"level"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	ToolName  string         `json:"tool_name,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// AlertHandler 告警处理回调
//
// 接收告警事件，由调用方实现具体通知逻辑
//   - 应用层日志
//   - 钉钉机器人
//   - 飞书群机器人
//   - Slack
type AlertHandler interface {
	OnAlert(event AlertEvent)
}

// AlertHandlerFunc 函数式 AlertHandler
type AlertHandlerFunc func(event AlertEvent)

func (f AlertHandlerFunc) OnAlert(event AlertEvent) { f(event) }

// ToolAlertManager 工具调用告警管理器
//
// 基于规则触发告警：
//  1. 工具失败率超过阈值
//  2. 熔断器开启
//  3. 死信队列堆积
//  4. 单次工具调用耗时过长
type ToolAlertManager struct {
	mu             sync.Mutex
	handlers       []AlertHandler
	failureRateMap map[string]*failureRateTracker
}

type failureRateTracker struct {
	total       int
	failed      int
	windowStart time.Time
}

// NewToolAlertManager 创建工具调用告警管理器
func NewToolAlertManager() *ToolAlertManager {
	return &ToolAlertManager{
		failureRateMap: make(map[string]*failureRateTracker),
	}
}

// AddHandler 添加告警处理回调
func (a *ToolAlertManager) AddHandler(handler AlertHandler) {
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
func (a *ToolAlertManager) OnToolCall(entry AuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	toolName := entry.ToolName
	tracker, ok := a.failureRateMap[toolName]
	if !ok {
		tracker = &failureRateTracker{windowStart: time.Now()}
		a.failureRateMap[toolName] = tracker
	}

	if time.Since(tracker.windowStart) > time.Minute {
		tracker.total = 0
		tracker.failed = 0
		tracker.windowStart = time.Now()
	}

	tracker.total++
	if !entry.Success {
		tracker.failed++
	}

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
func (a *ToolAlertManager) AlertCircuitOpen(toolName string, state CircuitState) {
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
func (a *ToolAlertManager) AlertDeadLetterBacklog(toolName string, backlogCount int) {
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

func (a *ToolAlertManager) emitAlert(event AlertEvent) {
	event.Timestamp = time.Now()
	for _, handler := range a.handlers {
		go func(h AlertHandler) {
			defer func() {
				_ = recover()
			}()
			h.OnAlert(event)
		}(handler)
	}
}

// Stats 返回各工具的失败率统计（用于 /metrics endpoint）
func (a *ToolAlertManager) Stats() map[string]map[string]any {
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

// CompositeAuditLogger 复合 AuditLogger
//
// 同时将审计日志写入多个目标：
//   - 内存（NewMemoryAuditLogger，便于即时查询）
//   - DB（DBAuditLogger，便于持久化）
//   - 告警管理器（ToolAlertManager，触发告警）
type CompositeAuditLogger struct {
	loggers []AuditLogger
	alert   *ToolAlertManager
}

// NewCompositeAuditLogger 创建复合 AuditLogger
func NewCompositeAuditLogger(memory AuditLogger, dbLogger *DBAuditLogger, alert *ToolAlertManager) *CompositeAuditLogger {
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
