package tooluse

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// 拆分自 decorator.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。

func (l *TokenBucketLimiter) Acquire(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: float64(l.burst), lastRef: now}
		l.buckets[key] = b
	}
	// 按时间间隔补充令牌
	elapsed := now.Sub(b.lastRef).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastRef = now
	if b.tokens < 1.0 {
		return ErrRateLimited
	}
	b.tokens -= 1.0
	return nil
}

// ===== 内置 RetryPolicy 实现：指数退避 =====

// ExponentialBackoffPolicy 指数退避重试策略
type ExponentialBackoffPolicy struct {
	MaxAttemptsValue int           // 最大重试次数（含首次）
	BaseDelay        time.Duration // 基础延迟
	MaxDelay         time.Duration // 最大延迟
	Jitter           bool          // 是否添加随机抖动
}

// NewExponentialBackoffPolicy 创建指数退避策略
func NewExponentialBackoffPolicy(maxAttempts int, baseDelay, maxDelay time.Duration) *ExponentialBackoffPolicy {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 10 * time.Second
	}
	return &ExponentialBackoffPolicy{
		MaxAttemptsValue: maxAttempts,
		BaseDelay:        baseDelay,
		MaxDelay:         maxDelay,
		Jitter:           true,
	}
}

// MaxAttempts 最大重试次数
func (p *ExponentialBackoffPolicy) MaxAttempts() int {
	return p.MaxAttemptsValue
}

// NextBackoff 下一次重试延迟
// attempt 从 1 开始（第 1 次重试的延迟）
// delay = baseDelay * 2^(attempt-1)，最大不超过 maxDelay
func (p *ExponentialBackoffPolicy) NextBackoff(attempt int, lastErr error) (time.Duration, bool) {
	if attempt < 1 {
		return 0, false
	}
	if attempt > p.MaxAttemptsValue {
		return 0, false
	}
	// 指数退避
	delay := float64(p.BaseDelay) * math.Pow(2, float64(attempt-1))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	// 添加抖动（±20%）
	if p.Jitter {
		jitter := (rand.Float64() - 0.5) * 0.4 * delay
		delay += jitter
		if delay < 0 {
			delay = float64(p.BaseDelay)
		}
	}
	return time.Duration(delay), true
}

// ===== 内置 AuditLogger 实现：内存记录（用于测试 / 默认审计） =====

// MemoryAuditLogger 内存审计日志（用于测试 / 单机审计）
type MemoryAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
	maxSize int
}

// NewMemoryAuditLogger 创建内存审计日志
// maxSize: 最大保留条数（超出后滚动覆盖最旧条目）
func NewMemoryAuditLogger(maxSize int) *MemoryAuditLogger {
	if maxSize < 1 {
		maxSize = 1000
	}
	return &MemoryAuditLogger{
		entries: make([]AuditEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Log 记录审计日志
func (l *MemoryAuditLogger) Log(ctx context.Context, entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= l.maxSize {
		// 滚动覆盖最旧条目
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)
}

// Entries 返回所有审计日志副本
func (l *MemoryAuditLogger) Entries() []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]AuditEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Count 返回审计日志条数
func (l *MemoryAuditLogger) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Reset 清空审计日志
func (l *MemoryAuditLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}

// ===== 内置 CostTracker 实现：内存计费（用于测试 / 统计） =====

// MemoryCostTracker 内存计费统计
type MemoryCostTracker struct {
	mu      sync.Mutex
	records map[string]*costRecord
}

type costRecord struct {
	TotalCalls      int64
	SuccessCalls    int64
	FailedCalls     int64
	TotalDurationMs int64
}

// NewMemoryCostTracker 创建内存计费统计
func NewMemoryCostTracker() *MemoryCostTracker {
	return &MemoryCostTracker{
		records: make(map[string]*costRecord),
	}
}

// Record 记录一次工具调用的成本
func (t *MemoryCostTracker) Record(ctx context.Context, toolName string, success bool, duration time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.records[toolName]
	if !ok {
		rec = &costRecord{}
		t.records[toolName] = rec
	}
	atomic.AddInt64(&rec.TotalCalls, 1)
	if success {
		atomic.AddInt64(&rec.SuccessCalls, 1)
	} else {
		atomic.AddInt64(&rec.FailedCalls, 1)
	}
	atomic.AddInt64(&rec.TotalDurationMs, duration.Milliseconds())
	return nil
}

// CostStats 计费统计快照
type CostStats struct {
	ToolName        string  `json:"tool_name"`
	TotalCalls      int64   `json:"total_calls"`
	SuccessCalls    int64   `json:"success_calls"`
	FailedCalls     int64   `json:"failed_calls"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationMs   float64 `json:"avg_duration_ms"`
}

// Stats 返回所有工具的计费统计
func (t *MemoryCostTracker) Stats() []CostStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]CostStats, 0, len(t.records))
	for name, rec := range t.records {
		stats := CostStats{
			ToolName:        name,
			TotalCalls:      atomic.LoadInt64(&rec.TotalCalls),
			SuccessCalls:    atomic.LoadInt64(&rec.SuccessCalls),
			FailedCalls:     atomic.LoadInt64(&rec.FailedCalls),
			TotalDurationMs: atomic.LoadInt64(&rec.TotalDurationMs),
		}
		if stats.TotalCalls > 0 {
			stats.SuccessRate = float64(stats.SuccessCalls) / float64(stats.TotalCalls)
			stats.AvgDurationMs = float64(stats.TotalDurationMs) / float64(stats.TotalCalls)
		}
		out = append(out, stats)
	}
	return out
}

// Reset 清空计费统计
func (t *MemoryCostTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make(map[string]*costRecord)
}
