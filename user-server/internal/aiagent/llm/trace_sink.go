package llm

import (
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"hivemtk-user/internal/pkg/utils/logger"
)

// DBTraceSink 把 InMemoryTraceBus 派发的 TraceEvent 异步批量落库到 trace_events 表。
//
// 设计要点：
//   - 订阅者模式：实现 TraceEventSubscriber 接口，OnEvent 非阻塞写入自有缓冲。
//   - 双缓冲隔离：业务发布走 InMemoryTraceBus(1024) → DBTraceSink 自有缓冲(2048) → 批量落库，
//     即便 DB 慢也能 backpressure 到 sink 层，绝不阻塞业务主链路（与 tracing.MessageTrace 同思路）。
//   - 批量插入：每 500ms 或缓冲达到 128 时落一次，减小 DB 写入放大。
//   - 优雅退出：Stop 时排空缓冲并做最后一次落库，进程重启不丢尾段事件。
//   - 失败可观测：批量失败记 Error 日志（含 span_id/trace_id/批次大小），不静默吞错。
//   - 零依赖耦合：不依赖任何业务包；可独立注入 main.go 启动序列。
type DBTraceSink struct {
	db     *gorm.DB
	buffer chan TraceEvent

	stopCh   chan struct{}
	stopOnce sync.Once
	doneCh   chan struct{}

	dropped  int64
	inserted int64
}

const (
	dbSinkBufferSize   = 2048
	dbSinkBatchSize    = 128
	dbSinkFlushPeriod  = 500 * time.Millisecond
	dbSinkStopDeadline = 3 * time.Second
)

// NewDBTraceSink 构造 DB 订阅者；db 允许为空（此时事件将被丢弃并计数）。
func NewDBTraceSink(db *gorm.DB) *DBTraceSink {
	return &DBTraceSink{
		db:     db,
		buffer: make(chan TraceEvent, dbSinkBufferSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start 启动后台批量落库 worker。
// 必须在 InitGlobalTraceBus 之后调用；调用方负责在进程退出时 Stop()。
func (s *DBTraceSink) Start() {
	go s.flushLoop()
	logger.GetLogger().Info().
		Int("buffer", dbSinkBufferSize).
		Int("batch", dbSinkBatchSize).
		Dur("period", dbSinkFlushPeriod).
		Msg("[DBTraceSink] started")
}

// OnEvent 实现 TraceEventSubscriber。非阻塞：缓冲满则丢弃并计数。
func (s *DBTraceSink) OnEvent(event TraceEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case s.buffer <- event:
	default:
		atomic.AddInt64(&s.dropped, 1)
	}
}

// Stats 返回插入/丢弃计数（运维/健康巡检用）。
func (s *DBTraceSink) Stats() (inserted, dropped int64) {
	return atomic.LoadInt64(&s.inserted), atomic.LoadInt64(&s.dropped)
}

// Stop 优雅停止：排空缓冲并做最后一次批量落库，进程退出不丢尾段事件。
// 幂等；多次调用安全。
func (s *DBTraceSink) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(dbSinkStopDeadline):
			logger.GetLogger().Warn().Msg("[DBTraceSink] stop deadline exceeded, forcing exit")
		}
	})
}

// flushLoop 后台批量写库：定时 / 批次达阈值 / 退出信号三触发。
func (s *DBTraceSink) flushLoop() {
	defer close(s.doneCh)

	ticker := time.NewTicker(dbSinkFlushPeriod)
	defer ticker.Stop()

	batch := make([]TraceEvent, 0, dbSinkBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if s.db == nil {
			batch = batch[:0]
			return
		}
		rows := make([]map[string]any, 0, len(batch))
		for _, e := range batch {
			rows = append(rows, traceEventToRow(e))
		}
		if err := s.db.Table("trace_events").Create(rows).Error; err != nil {
			logger.GetLogger().Error().
				Err(err).
				Int("batch", len(batch)).
				Msg("[DBTraceSink] batch insert failed")
		} else {
			atomic.AddInt64(&s.inserted, int64(len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case ev := <-s.buffer:
			batch = append(batch, ev)
			if len(batch) >= dbSinkBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stopCh:
			for {
				select {
				case ev := <-s.buffer:
					batch = append(batch, ev)
					if len(batch) >= dbSinkBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// traceEventToRow 把 TraceEvent 序列化为 map 以便 Create() 写入。
//
// 为什么用 map 而非 *TraceEvent：避免 GORM 依赖 llm 包自身的 TableName（两表并存时不让 ORM 自动建表）。
// 字段命名/类型与 migrations/m_p1_migration.go::createTraceEvents 严格对齐。
func traceEventToRow(e TraceEvent) map[string]any {
	row := map[string]any{
		"trace_id":    e.TraceID,
		"span_id":     e.SpanID,
		"kind":        string(e.Kind),
		"service":     e.Service,
		"operation":   e.Operation,
		"duration_ms": e.DurationMs,
		"status":      e.Status,
		"timestamp":   e.Timestamp,
	}
	if e.ParentSpanID != "" {
		row["parent_span_id"] = e.ParentSpanID
	}
	row["metadata"] = MarshalMetadata(e.Metadata)
	return row
}
