package service

import (
	"context"

	"errors"

	"fmt"

	"math"

	"strings"

	"sync"

	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"sync/atomic"
)

const (
	SendStepPermission = "permission"

	SendStepRateLimit = "rate_limit"

	SendStepRetry = "retry"

	SendStepFallback = "fallback"

	SendStepAudit = "audit"

	SendStepCost = "cost"

	SendStepJourney = "journey"

	SendStepSend = "send"
)

var DefaultSendPipelineSteps = []string{
	SendStepPermission,
	SendStepRateLimit,
	SendStepRetry,
	SendStepFallback,
	SendStepAudit,
	SendStepCost,
	SendStepJourney,
	SendStepSend,
}

var (
	ErrSendPermissionDenied = errors.New("send permission denied")

	// ErrSendDoNotContact R-3a：收件人命中全局跨渠道退订标志位，必须跳过发送
	ErrSendDoNotContact = errors.New("do not contact: recipient opted out globally")

	// ErrSendQuietHoursDeferred R-4：命中全渠道 quiet hours，消息已入延迟队列（非失败）
	ErrSendQuietHoursDeferred = errors.New("send deferred to quiet hours release time")

	ErrSendRateLimited = errors.New("send rate limited")

	ErrSendAllChannelFailed = errors.New("all channels failed (primary + fallback)")

	ErrSendInsufficientCost = errors.New("insufficient balance for send")

	ErrSendChannelNotConfig = errors.New("channel adapter not configured")
)

type ReachSendRequest struct {
	Channel     string
	AccountID   string
	RecipientID string
	CustomerID  string
	OperatorID  string
	MsgType     string
	Content     string
	Subject     string
	TemplateID  string
	Params      map[string]string
	Attachments []string
	CardID      string
	Fallback    *FallbackConfig
	Metadata    map[string]string
}

type FallbackConfig struct {
	Enabled       bool
	BackupChannel string
	BackupAccount string
	MaxAttempts   int
}

type SendResponse struct {
	Success        bool          `json:"success"`
	MessageID      string        `json:"message_id"`
	Channel        string        `json:"channel"`
	AccountID      string        `json:"account_id"`
	FallbackUsed   bool          `json:"fallback_used"`
	PrimaryChannel string        `json:"primary_channel"`
	RetryCount     int           `json:"retry_count"`
	StepResults    []SendStepLog `json:"step_results"`
	Error          string        `json:"error,omitempty"`
	DurationMs     int64         `json:"duration_ms"`
	SentAt         time.Time     `json:"sent_at"`
	// Deferred R-4：命中 quiet hours 进入延迟队列时为 true（非失败语义）
	Deferred bool `json:"deferred,omitempty"`
	// DeferredAt R-4：计划首发时间
	DeferredAt time.Time `json:"deferred_at,omitempty"`
}

type SendStepLog struct {
	Step       string    `json:"step"`
	Success    bool      `json:"success"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	Output     []any     `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	Skipped    bool      `json:"skipped,omitempty"`
}

type SendPermissionChecker interface {
	CheckSendPermission(ctx context.Context, req *ReachSendRequest) error
}

// DoNotContactChecker R-3a：全局跨渠道退订标志位检查接口
// （由 DoNotContactService 实现，permission 步骤发送前置调用）
type DoNotContactChecker interface {
	IsBlocked(ctx context.Context, oneID, channel string) bool
}

type SendRateLimiter interface {
	Allow(ctx context.Context, key string, limit RateLimitSpec) bool
}

type RateLimitSpec struct {
	QPS          int
	Burst        int
	DailyQuota   int
	PerUserLimit int
}

type SendRetryPolicy struct {
	MaxRetries      int
	IntervalMs      int
	Backoff         string
	MaxIntervalMs   int
	RetryableErrors []string
}

func DefaultSendRetryPolicy() SendRetryPolicy {
	return SendRetryPolicy{
		MaxRetries:    3,
		IntervalMs:    500,
		Backoff:       "exponential",
		MaxIntervalMs: 10000,
	}
}

type SendAuditLogger interface {
	LogSend(ctx context.Context, req *ReachSendRequest, resp *SendResponse)
}

type SendCostTracker interface {
	Charge(ctx context.Context, channel string, req *ReachSendRequest) (cost float64, err error)
}

type SendJourneyTracker interface {
	RecordTouch(ctx context.Context, customerID, channel, source string) error
}

type ChannelAdapter interface {
	Send(ctx context.Context, req *ReachSendRequest) (msgID string, err error)
}

type AllowAllSendPermission struct{}

func (AllowAllSendPermission) CheckSendPermission(ctx context.Context, req *ReachSendRequest) error {
	return nil
}

type NoOpSendRateLimiter struct{}

func (NoOpSendRateLimiter) Allow(ctx context.Context, key string, limit RateLimitSpec) bool {
	return true
}

type MemorySendRateLimiter struct {
	shards [rateLimiterShards]*rateLimiterShard
}

type rateLimiterShard struct {
	mu      sync.Mutex
	buckets map[string]*sendRateBucket
}

type sendRateBucket struct {
	tokens   float64
	lastFill time.Time
	qps      int
	burst    int
}

const (
	rateLimiterShards = 64

	rateLimiterBucketIdleTTL = 10 * time.Minute
)

var rateLimiterMaxBuckets = 4096

func NewMemorySendRateLimiter() *MemorySendRateLimiter {
	l := &MemorySendRateLimiter{}
	for i := range l.shards {
		l.shards[i] = &rateLimiterShard{buckets: make(map[string]*sendRateBucket)}
	}
	return l
}

func (l *MemorySendRateLimiter) shardOf(ctx context.Context, key string) *rateLimiterShard {
	var h uint32
	for i := 0; i < len(key); i++ {
		h = h*31 + uint32(key[i])
	}
	return l.shards[h%rateLimiterShards]
}

func (l *MemorySendRateLimiter) Allow(ctx context.Context, key string, limit RateLimitSpec) bool {
	if limit.QPS <= 0 && limit.Burst <= 0 {
		return true
	}
	s := l.shardOf(ctx, key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.buckets[key]
	if !ok || b.qps != limit.QPS || b.burst != limit.Burst || now.Sub(b.lastFill) > rateLimiterBucketIdleTTL {
		if !ok {
			if len(s.buckets) >= rateLimiterMaxBuckets {
				s.evictStalestLocked()
			}
		}
		b = &sendRateBucket{
			tokens:   float64(limit.Burst),
			lastFill: now,
			qps:      limit.QPS,
			burst:    limit.Burst,
		}
		s.buckets[key] = b
	}

	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens = math.Min(float64(b.burst), b.tokens+elapsed*float64(b.qps))
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (l *MemorySendRateLimiter) totalBucketCount() int {
	total := 0
	for i := range l.shards {
		s := l.shards[i]
		s.mu.Lock()
		total += len(s.buckets)
		s.mu.Unlock()
	}
	return total
}

func (s *rateLimiterShard) evictStalestLocked() {
	var staleKey string
	var staleTime time.Time
	first := true
	for k, b := range s.buckets {
		if first || b.lastFill.Before(staleTime) {
			staleKey = k
			staleTime = b.lastFill
			first = false
		}
	}
	if staleKey != "" {
		delete(s.buckets, staleKey)
	}
}

func (l *MemorySendRateLimiter) Reset(ctx context.Context, key string) {
	s := l.shardOf(ctx, key)
	s.mu.Lock()
	delete(s.buckets, key)
	s.mu.Unlock()
}

type MemorySendAuditLogger struct {
	mu      sync.Mutex
	entries []*SendAuditEntry
	maxSize int
}

type SendAuditEntry struct {
	Timestamp  time.Time
	OperatorID string
	Channel    string
	AccountID  string
	Recipient  string
	CustomerID string
	Content    string
	Success    bool
	MessageID  string
	Error      string
	DurationMs int64
}

func NewMemorySendAuditLogger(maxSize int) *MemorySendAuditLogger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemorySendAuditLogger{maxSize: maxSize}
}

func (l *MemorySendAuditLogger) LogSend(ctx context.Context, req *ReachSendRequest, resp *SendResponse) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := &SendAuditEntry{
		Timestamp:  time.Now(),
		OperatorID: req.OperatorID,
		Channel:    req.Channel,
		AccountID:  req.AccountID,
		Recipient:  req.RecipientID,
		CustomerID: req.CustomerID,
		Content:    req.Content,
		Success:    resp.Success,
		MessageID:  resp.MessageID,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
	}
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
}

func (l *MemorySendAuditLogger) Entries(ctx context.Context) []*SendAuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*SendAuditEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

type NoOpSendCostTracker struct{}

func (NoOpSendCostTracker) Charge(ctx context.Context, channel string, req *ReachSendRequest) (float64, error) {
	return 0, nil
}

type MemorySendCostTracker struct {
	mu        sync.Mutex
	balance   float64
	costs     map[string]float64
	totalUsed float64
}

func NewMemorySendCostTracker(initialBalance float64) *MemorySendCostTracker {
	return &MemorySendCostTracker{
		balance: initialBalance,
		costs: map[string]float64{
			"sms":      0.05,
			"email":    0.001,
			"wecom":    0,
			"weixin":   0,
			"douyin":   0,
			"kuaishou": 0,
			"xhs":      0,
			"dingtalk": 0,
			"card":     0.01,
		},
	}
}

func (t *MemorySendCostTracker) SetCost(ctx context.Context, channel string, cost float64) {
	t.mu.Lock()
	t.costs[channel] = cost
	t.mu.Unlock()
}

func (t *MemorySendCostTracker) Charge(ctx context.Context, channel string, req *ReachSendRequest) (float64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cost := t.costs[channel]
	if t.balance < cost {
		return 0, ErrSendInsufficientCost
	}
	t.balance -= cost
	t.totalUsed += cost
	return cost, nil
}

func (t *MemorySendCostTracker) Balance(ctx context.Context) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.balance
}

func (t *MemorySendCostTracker) TotalUsed(ctx context.Context) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalUsed
}

type NoOpSendJourneyTracker struct{}

func (NoOpSendJourneyTracker) RecordTouch(ctx context.Context, customerID, channel, source string) error {
	return nil
}

type CustomerJourneySendTracker struct {
	Service *CustomerJourneyService
}

func (t CustomerJourneySendTracker) RecordTouch(ctx context.Context, customerID, channel, source string) error {
	if t.Service == nil || customerID == "" {
		return nil
	}
	t.Service.Touch(ctx, customerID, source)
	return nil
}

type SendPipeline interface {
	Send(ctx context.Context, req *ReachSendRequest) *SendResponse
}

type SendPipelineConfig struct {
	PermissionChecker SendPermissionChecker
	// DoNotContact R-3a：全局退订标志位检查器（nil 时跳过该检查）
	DoNotContact     DoNotContactChecker
	RateLimiter      SendRateLimiter
	RateLimitSpec    RateLimitSpec
	RetryPolicy      SendRetryPolicy
	AuditLogger      SendAuditLogger
	CostTracker      SendCostTracker
	JourneyTracker   SendJourneyTracker
	Adapter          ChannelAdapter
	FallbackAdapters map[string]ChannelAdapter
	Steps            []string

	// QuietHoursEnabled R-4：启用全渠道 quiet hours 守卫（22:00-8:00 CST）。
	// 短信渠道不受此守卫影响（既有铁律保持：sms.go 夜间拒绝式拦截）。
	QuietHoursEnabled bool
	// QuietHoursDeferrer R-4：命中 quiet hours 后的延迟入队器；
	// nil 且 QuietHoursEnabled=true 时使用全局 GlobalQuietHoursQueue（惰性创建）。
	QuietHoursDeferrer SendQuietHoursDeferrer
	// QuietHoursClock 可替换时钟（测试注入），生产为 nil 即 time.Now
	QuietHoursClock func() time.Time
}

// GlobalQuietHoursQueue 全局进程内延迟队列（R-4 默认实现）。
// 生产装配建议：main 中 q := service.NewMemoryQuietHoursQueue();
// cfg.QuietHoursDeferrer = q; cfg.QuietHoursEnabled = true; q.Start(ctx, pipeline)。
var globalQuietHoursOnce sync.Once

func GetGlobalQuietHoursQueue() *MemoryQuietHoursQueue {
	globalQuietHoursOnce.Do(func() {
		globalQuietHoursQueue = NewMemoryQuietHoursQueue()
	})
	return globalQuietHoursQueue
}

var globalQuietHoursQueue *MemoryQuietHoursQueue

func DefaultSendPipelineConfig(adapter ChannelAdapter) SendPipelineConfig {
	return SendPipelineConfig{
		PermissionChecker: AllowAllSendPermission{},
		RateLimiter:       NoOpSendRateLimiter{},
		RetryPolicy:       DefaultSendRetryPolicy(),
		AuditLogger:       NewMemorySendAuditLogger(1000),
		CostTracker:       NoOpSendCostTracker{},
		JourneyTracker:    NoOpSendJourneyTracker{},
		Adapter:           adapter,
		FallbackAdapters:  map[string]ChannelAdapter{},
		Steps:             DefaultSendPipelineSteps,
	}
}

var defaultThirdPartyRateLimitByChannel = map[string]RateLimitSpec{
	"sms":          {QPS: 50, Burst: 100},
	"email":        {QPS: 30, Burst: 60},
	"wecom":        {QPS: 30, Burst: 60},
	"weixin":       {QPS: 30, Burst: 60},
	"douyin":       {QPS: 30, Burst: 60},
	"douyin_web":   {QPS: 30, Burst: 60},
	"kuaishou":     {QPS: 30, Burst: 60},
	"kuaishou_web": {QPS: 30, Burst: 60},
	"xiaohongshu":  {QPS: 30, Burst: 60},
	"xhs":          {QPS: 30, Burst: 60},
	"xhs_web":      {QPS: 30, Burst: 60},
	"tiktok":       {QPS: 30, Burst: 60},
	"tiktok_web":   {QPS: 30, Burst: 60},
	"xianyu":       {QPS: 30, Burst: 60},
	"xianyu_web":   {QPS: 30, Burst: 60},
	"dingtalk":     {QPS: 30, Burst: 60},
	"telegram":     {QPS: 30, Burst: 60},
	"whatsapp":     {QPS: 30, Burst: 60},
	"feishu":       {QPS: 30, Burst: 60},
}

func DefaultThirdPartyRateLimitSpec(channel string) RateLimitSpec {
	if spec, ok := defaultThirdPartyRateLimitByChannel[channel]; ok {
		return spec
	}
	return RateLimitSpec{}
}

func NewDefaultRateLimitedPipelineConfig(adapter ChannelAdapter) SendPipelineConfig {
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.RateLimiter = NewMemorySendRateLimiter()
	return cfg
}

// NewDefaultRateLimitedPipelineConfigWithCache D14-T15b：优先 GCRA（redis_rate）限流，
// 全局缓存非 Redis 后端时回退进程内 MemorySendRateLimiter（语义与 T15 全局频控降级一致）。
func NewDefaultRateLimitedPipelineConfigWithCache(adapter ChannelAdapter) SendPipelineConfig {
	cfg := DefaultSendPipelineConfig(adapter)
	if gl := NewGCRARateLimiterFromGlobalCache(); gl != nil {
		cfg.RateLimiter = gl
	} else {
		cfg.RateLimiter = NewMemorySendRateLimiter()
	}
	return cfg
}

type defaultSendPipeline struct {
	config SendPipelineConfig
}

func NewSendPipeline(config SendPipelineConfig) SendPipeline {
	if config.Steps == nil || len(config.Steps) == 0 {
		config.Steps = DefaultSendPipelineSteps
	}
	return &defaultSendPipeline{config: config}
}

// ===== R-4 全渠道 quiet hours =====
//
// 决策依据 MASTER_COMPETITIVE_DECISIONS.md M17 R-4：
// 夜间守卫覆盖全渠道（短信既有铁律保持不变，由 sms.go isSMSNightRestricted 拒绝式拦截）。
// 命中时段的消息进入延迟队列（次日首发时间点），而非拒绝。

var cstZone = time.FixedZone("CST", 8*3600)

// quietHoursStartHour / quietHoursEndHour 全渠道主动触达静默窗口 22:00-8:00 (CST)
const (
	quietHoursStartHour = 22
	quietHoursEndHour   = 8

	// nextDayFirstSendHour 次日首发时间点：窗口结束时刻（08:00 CST）
	nextDayFirstSendHour = quietHoursEndHour
)

// inQuietHoursWindow 判定 t 是否落在 [startHour, endHour) 跨午夜静默窗口内（CST）。
// 例如 start=22,end=8：22:00:00~07:59:59 命中；21:59 与 08:00 不命中。
func inQuietHoursWindow(t time.Time, startHour, endHour int) bool {
	h := t.In(cstZone).Hour()
	if startHour > endHour {
		return h >= startHour || h < endHour
	}
	return h >= startHour && h < endHour
}

// nextQuietHoursRelease 计算静默窗口结束后的首发时间点（endHour:00 CST）。
// 窗口内 → 当日（跨午夜则为次日）endHour:00；窗口外返回 t 本身。
func nextQuietHoursRelease(t time.Time, endHour int) time.Time {
	local := t.In(cstZone)
	release := time.Date(local.Year(), local.Month(), local.Day(), endHour, 0, 0, 0, cstZone)
	if !local.Before(release) {
		release = release.AddDate(0, 0, 1)
	}
	return release
}

// SendQuietHoursDeferrer R-4：quiet hours 命中后的延迟入队接口
type SendQuietHoursDeferrer interface {
	Defer(ctx context.Context, req *ReachSendRequest, sendAt time.Time) error
}

// MemoryQuietHoursQueue 进程内 quiet hours 延迟队列（R-4 最小实现）。
// Defer 入队；Start 启动后台循环将到期消息经原 pipeline 重发。
// 注意：进程重启丢队（与 MemorySendAuditLogger 同级语义）；多实例共享需迁 Redis ZSET，量级未到不做。
type MemoryQuietHoursQueue struct {
	mu      sync.Mutex
	items   []deferredSendItem
	wake    chan struct{}
	started atomic.Bool
}

type deferredSendItem struct {
	req    *ReachSendRequest
	sendAt time.Time
}

func NewMemoryQuietHoursQueue() *MemoryQuietHoursQueue {
	return &MemoryQuietHoursQueue{wake: make(chan struct{}, 1)}
}

// Defer 实现 SendQuietHoursDeferrer
func (q *MemoryQuietHoursQueue) Defer(_ context.Context, req *ReachSendRequest, sendAt time.Time) error {
	q.mu.Lock()
	q.items = append(q.items, deferredSendItem{req: req, sendAt: sendAt})
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
	logger.Warnf("[R-4] 触达命中全渠道 quiet hours，进入延迟队列 channel=%s recipient=%s send_at=%s",
		req.Channel, req.RecipientID, sendAt.Format(time.RFC3339))
	return nil
}

// Len 当前队列长度（测试/运维）
func (q *MemoryQuietHoursQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Start 启动到期重发循环；每秒扫描一次，到期项经 pipeline.Send 重发。
func (q *MemoryQuietHoursQueue) Start(ctx context.Context, pipeline SendPipeline) {
	if !q.started.CompareAndSwap(false, true) {
		return
	}
	// 最高标准审计 P1-3 修复：静默时段到期重发循环（消息发送路径）改走 SafeGo
	utils.SafeGo(ctx, "reach_send_pipeline.quiet_hours_queue", func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			case <-ticker.C:
			}
			due := time.Now()
			var pending []deferredSendItem
			q.mu.Lock()
			kept := q.items[:0]
			for _, it := range q.items {
				if it.sendAt.After(due) {
					kept = append(kept, it)
				} else {
					pending = append(pending, it)
				}
			}
			q.items = kept
			q.mu.Unlock()
			for _, it := range pending {
				pipeline.Send(ctx, it.req)
			}
		}
	})
}

// ===== R-8 AuditLogger 持久化 =====
//
// 决策依据 M17 R-8：合规日志必须落库。LogComplianceReminder 除原有 WARN 日志外，
// 异步批量写 reach_compliance_log 表（缓冲满/定时刷盘），不阻塞发送主路径。

const complianceReminderTag = "[COMPLIANCE]"

// ReachComplianceLog 合规提醒审计日志（表 reach_compliance_log）
type ReachComplianceLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Channel     string    `gorm:"type:varchar(30);index" json:"channel"`
	RecipientID string    `gorm:"type:varchar(128)" json:"recipient_id"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (ReachComplianceLog) TableName() string { return "reach_compliance_log" }

const (
	complianceFlushBatchSize = 100
	complianceFlushInterval  = 5 * time.Second
)

// ComplianceAuditLogger R-8：异步批量落库的合规日志器
type ComplianceAuditLogger struct {
	mu      sync.Mutex
	buf     []*ReachComplianceLog
	db      *gorm.DB
	flushCh chan struct{}
	stop    chan struct{}
	stopped sync.Once
}

var (
	complianceLoggerOnce sync.Once
	complianceLogger     *ComplianceAuditLogger
)

// InitComplianceAuditLogger 初始化全局合规审计落库器（main/router 装配时调用一次）。
// db 为 nil 时退化为仅 WARN 日志（向后兼容）。表结构经 EnsureTable 惰性创建，
// migrate.go 正式注册另行报告。
func InitComplianceAuditLogger(db *gorm.DB) *ComplianceAuditLogger {
	complianceLoggerOnce.Do(func() {
		complianceLogger = &ComplianceAuditLogger{
			db:      db,
			flushCh: make(chan struct{}, 1),
			stop:    make(chan struct{}),
		}
		if db != nil {
			if err := db.AutoMigrate(&ReachComplianceLog{}); err != nil {
				logger.Errorf("[R-8] reach_compliance_log 建表失败: %v", err)
			}
			go complianceLogger.flushLoop()
		}
	})
	return complianceLogger
}

// GetComplianceAuditLogger 获取全局实例（未初始化返回 nil）
func GetComplianceAuditLogger() *ComplianceAuditLogger { return complianceLogger }

// record 追加一条到缓冲；缓冲满立即触发刷盘信号。
// 无 db 时仅入缓冲（容量封顶），Flush 为 no-op——保证接口行为一致。
func (l *ComplianceAuditLogger) record(channel, recipientID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.buf = append(l.buf, &ReachComplianceLog{Channel: channel, RecipientID: recipientID})
	if len(l.buf) > complianceFlushBatchSize*10 {
		l.buf = l.buf[len(l.buf)-complianceFlushBatchSize*10:]
	}
	full := len(l.buf) >= complianceFlushBatchSize
	l.mu.Unlock()
	if full {
		select {
		case l.flushCh <- struct{}{}:
		default:
		}
	}
}

// Flush 将缓冲批量写入 DB（供 flushLoop 与测试调用）；db 未配置时清空缓冲并返回
func (l *ComplianceAuditLogger) Flush() error {
	l.mu.Lock()
	if len(l.buf) == 0 {
		l.mu.Unlock()
		return nil
	}
	batch := l.buf
	l.buf = nil
	l.mu.Unlock()
	if l.db == nil || len(batch) == 0 {
		return nil
	}
	if err := l.db.CreateInBatches(batch, len(batch)).Error; err != nil {
		// 失败回灌缓冲头部，避免丢失（容量上限截断防 OOM）
		l.mu.Lock()
		l.buf = append(batch, l.buf...)
		if len(l.buf) > complianceFlushBatchSize*10 {
			l.buf = l.buf[len(l.buf)-complianceFlushBatchSize*10:]
		}
		l.mu.Unlock()
		return err
	}
	return nil
}

func (l *ComplianceAuditLogger) flushLoop() {
	ticker := time.NewTicker(complianceFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			_ = l.Flush()
			return
		case <-l.flushCh:
			if err := l.Flush(); err != nil {
				logger.Errorf("[R-8] 合规日志刷盘失败: %v", err)
			}
		case <-ticker.C:
			if err := l.Flush(); err != nil {
				logger.Errorf("[R-8] 合规日志定时刷盘失败: %v", err)
			}
		}
	}
}

// Stop 停止刷盘循环并冲刷残余缓冲
func (l *ComplianceAuditLogger) Stop() {
	l.stopped.Do(func() { close(l.stop) })
}

// BufferedCount 当前缓冲条数（测试用）
func (l *ComplianceAuditLogger) BufferedCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buf)
}

func LogComplianceReminder(channel, recipientID string) {
	logger.Warnf("%s 主动触达发送已触发：channel=%s, recipient=%s。"+
		"请严格遵守各渠道平台（微信/企业微信/抖音/快手/小红书/Telegram/WhatsApp(Meta)/短信/邮件 等）的"+
		"开发者规范、服务条款及当地法律法规；仅可向已授权、已明确同意接收的联系人发送，"+
		"严格控制发送频率，禁止发送垃圾营销、欺诈、骚扰或违法违规内容。"+
		"因违规发送导致的账号封禁、平台处罚、行政处罚或法律后果由使用者自行承担。",
		complianceReminderTag, channel, recipientID)
	// R-8：异步持久化到 reach_compliance_log（未初始化时为 no-op）
	complianceLogger.record(channel, recipientID)
}

func (p *defaultSendPipeline) Send(ctx context.Context, req *ReachSendRequest) *SendResponse {
	LogComplianceReminder(req.Channel, req.RecipientID)

	start := time.Now()
	resp := &SendResponse{
		PrimaryChannel: req.Channel,
		Channel:        req.Channel,
		AccountID:      req.AccountID,
		StepResults:    []SendStepLog{},
	}

	stepFuncs := map[string]func(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog{
		SendStepPermission: p.runPermission,
		SendStepRateLimit:  p.runRateLimit,
		SendStepRetry:      p.runRetry,
		SendStepFallback:   p.runFallback,
		SendStepAudit:      p.runAudit,
		SendStepCost:       p.runCost,
		SendStepJourney:    p.runJourney,
		SendStepSend:       p.runSend,
	}

	for _, step := range p.config.Steps {
		fn, ok := stepFuncs[step]
		if !ok {
			continue
		}
		log := fn(ctx, req, resp)
		resp.StepResults = append(resp.StepResults, log)
		// R-4：命中 quiet hours 已入延迟队列，终止后续步骤（非失败语义）
		if resp.Deferred {
			resp.Success = true
			resp.Error = ""
			resp.DurationMs = time.Since(start).Milliseconds()
			resp.SentAt = time.Now()
			return resp
		}
		if !log.Success && !log.Skipped {
			resp.Success = false
			resp.Error = log.Error
			resp.DurationMs = time.Since(start).Milliseconds()
			resp.SentAt = time.Now()
			if p.config.AuditLogger != nil {
				p.config.AuditLogger.LogSend(ctx, req, resp)
			}
			return resp
		}
	}

	resp.Success = true
	resp.DurationMs = time.Since(start).Milliseconds()
	resp.SentAt = time.Now()
	if p.config.AuditLogger != nil {
		p.config.AuditLogger.LogSend(ctx, req, resp)
	}
	return resp
}

func (p *defaultSendPipeline) runPermission(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepPermission, StartedAt: start}
	if p.config.PermissionChecker == nil {
		log.Skipped = true
		log.Success = true
		log.EndedAt = time.Now()
		log.DurationMs = time.Since(start).Milliseconds()
		return log
	}
	if err := p.config.PermissionChecker.CheckSendPermission(ctx, req); err != nil {
		log.Success = false
		log.Error = err.Error()
	} else if p.config.DoNotContact != nil {
		// R-3a：发送前置全局退订标志位检查——命中即拒绝发送（permission 步骤内）
		oneID := req.CustomerID
		if v, ok := req.Metadata["one_id"]; ok && v != "" {
			oneID = v
		}
		if oneID != "" && p.config.DoNotContact.IsBlocked(ctx, oneID, req.Channel) {
			log.Success = false
			log.Error = ErrSendDoNotContact.Error()
			logger.Warnf("[DNC] pipeline 跳过发送 customer=%s channel=%s（命中全局退订标志位）", oneID, req.Channel)
		} else if p.checkQuietHours(ctx, req, resp) {
			log.Success = true
			log.Skipped = true
			log.Output = []any{map[string]any{
				"deferred": true,
				"send_at":  resp.DeferredAt.Format(time.RFC3339),
				"reason":   ErrSendQuietHoursDeferred.Error(),
			}}
		} else {
			log.Success = true
		}
	} else if p.checkQuietHours(ctx, req, resp) {
		log.Success = true
		log.Skipped = true
		log.Output = []any{map[string]any{
			"deferred": true,
			"send_at":  resp.DeferredAt.Format(time.RFC3339),
			"reason":   ErrSendQuietHoursDeferred.Error(),
		}}
	} else {
		log.Success = true
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// checkQuietHours R-4：全渠道 quiet hours 守卫（22:00-8:00 CST）。
//
// 短信渠道豁免（既有铁律保持：sms.go isSMSNightRestricted 拒绝式拦截，避免双重处理）。
// 命中时入延迟队列（次日 08:00 CST 首发），设置 resp.Deferred/DeferredAt 并返回 true。
func (p *defaultSendPipeline) checkQuietHours(ctx context.Context, req *ReachSendRequest, resp *SendResponse) bool {
	if !p.config.QuietHoursEnabled || req.Channel == "sms" {
		return false
	}
	now := time.Now()
	if p.config.QuietHoursClock != nil {
		now = p.config.QuietHoursClock()
	}
	if !inQuietHoursWindow(now, quietHoursStartHour, quietHoursEndHour) {
		return false
	}
	deferrer := p.config.QuietHoursDeferrer
	if deferrer == nil {
		deferrer = GetGlobalQuietHoursQueue()
	}
	sendAt := nextQuietHoursRelease(now, nextDayFirstSendHour)
	if err := deferrer.Defer(ctx, req, sendAt); err != nil {
		logger.Errorf("[R-4] quiet hours 延迟入队失败，按拒绝处理 channel=%s recipient=%s: %v",
			req.Channel, req.RecipientID, err)
		resp.Deferred = false
		return false
	}
	resp.Deferred = true
	resp.DeferredAt = sendAt
	return true
}

func (p *defaultSendPipeline) runRateLimit(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepRateLimit, StartedAt: start}
	if p.config.RateLimiter == nil {
		log.Skipped = true
		log.Success = true
		log.EndedAt = time.Now()
		log.DurationMs = time.Since(start).Milliseconds()
		return log
	}
	key := fmt.Sprintf("%s:%s:%s", req.Channel, req.AccountID, req.CustomerID)
	spec := p.config.RateLimitSpec
	if spec.QPS <= 0 && spec.Burst <= 0 {
		spec = DefaultThirdPartyRateLimitSpec(req.Channel)
	}
	if !p.config.RateLimiter.Allow(ctx, key, spec) {
		log.Success = false
		log.Error = ErrSendRateLimited.Error()
	} else {
		log.Success = true
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

func (p *defaultSendPipeline) runRetry(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepRetry, StartedAt: start}
	policy := p.config.RetryPolicy
	if policy.MaxRetries <= 0 {
		policy = DefaultSendRetryPolicy()
	}

	var lastErr error
	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		msgID, err := p.executeSendWithFallback(ctx, req)
		if err == nil {
			lastErr = nil
			resp.RetryCount = attempt
			resp.MessageID = msgID
			log.Success = true
			log.Output = []any{map[string]any{
				"attempt":    attempt,
				"message_id": msgID,
			}}
			log.EndedAt = time.Now()
			log.DurationMs = time.Since(start).Milliseconds()
			return log
		}
		lastErr = err
		if !p.isRetryable(ctx, err, policy.RetryableErrors) {
			break
		}
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			resp.RetryCount = attempt
			break
		}
		if attempt < policy.MaxRetries {
			wait := p.computeBackoff(ctx, policy, attempt)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				lastErr = ctx.Err()
			}
			if ctx.Err() != nil {
				resp.RetryCount = attempt
				break
			}
		}
	}
	resp.RetryCount = policy.MaxRetries
	log.Success = false
	log.Error = fmt.Sprintf("重试 %d 次后仍失败: %v", policy.MaxRetries, lastErr)
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

func (p *defaultSendPipeline) executeSendWithFallback(ctx context.Context, req *ReachSendRequest) (string, error) {
	if p.config.Adapter == nil {
		return "", ErrSendChannelNotConfig
	}
	msgID, err := p.config.Adapter.Send(ctx, req)
	if err == nil {
		return msgID, nil
	}
	if req.Fallback == nil || !req.Fallback.Enabled || req.Fallback.BackupChannel == "" {
		return "", err
	}
	if req.Fallback.MaxAttempts <= 0 {
		req.Fallback.MaxAttempts = 1
	}
	for i := 0; i < req.Fallback.MaxAttempts; i++ {
		backupAdapter, ok := p.config.FallbackAdapters[req.Fallback.BackupChannel]
		if !ok {
			continue
		}
		backupReq := *req
		backupReq.Channel = req.Fallback.BackupChannel
		backupReq.AccountID = req.Fallback.BackupAccount
		backupReq.Fallback = nil
		msgID, err2 := backupAdapter.Send(ctx, &backupReq)
		if err2 == nil {
			return msgID, nil
		}
		err = err2
	}
	return "", err
}

func (p *defaultSendPipeline) isRetryable(ctx context.Context, err error, retryableErrors []string) bool {
	if err == nil {
		return false
	}
	if len(retryableErrors) == 0 {
		return true
	}
	errStr := err.Error()
	for _, re := range retryableErrors {
		if strings.Contains(errStr, re) {
			return true
		}
	}
	return false
}

func (p *defaultSendPipeline) computeBackoff(ctx context.Context, policy SendRetryPolicy, attempt int) time.Duration {
	interval := policy.IntervalMs
	if policy.Backoff == "exponential" {
		mult := 1
		for i := 0; i < attempt; i++ {
			mult *= 2
		}
		interval = policy.IntervalMs * mult
		if policy.MaxIntervalMs > 0 && interval > policy.MaxIntervalMs {
			interval = policy.MaxIntervalMs
		}
	}
	return time.Duration(interval) * time.Millisecond
}

func (p *defaultSendPipeline) runFallback(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepFallback, StartedAt: start}
	if req.Fallback != nil && req.Fallback.Enabled && req.Fallback.BackupChannel != "" {
		log.Success = true
		log.Output = []any{map[string]any{
			"backup_channel": req.Fallback.BackupChannel,
			"max_attempts":   req.Fallback.MaxAttempts,
		}}
	} else {
		log.Skipped = true
		log.Success = true
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

func (p *defaultSendPipeline) runAudit(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepAudit, StartedAt: start}
	if p.config.AuditLogger == nil {
		log.Skipped = true
		log.Success = true
		log.EndedAt = time.Now()
		log.DurationMs = time.Since(start).Milliseconds()
		return log
	}
	log.Success = true
	log.Output = []any{map[string]any{
		"note": "audit log will be recorded at pipeline finalize",
	}}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// 7. 计费
func (p *defaultSendPipeline) runCost(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepCost, StartedAt: start}
	if p.config.CostTracker == nil {
		log.Skipped = true
		log.Success = true
		log.EndedAt = time.Now()
		log.DurationMs = time.Since(start).Milliseconds()
		return log
	}
	cost, err := p.config.CostTracker.Charge(ctx, req.Channel, req)
	if err != nil {
		log.Success = false
		log.Error = err.Error()
	} else {
		log.Success = true
		log.Output = []any{map[string]any{"cost": cost}}
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// 8. 客户轨迹
func (p *defaultSendPipeline) runJourney(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepJourney, StartedAt: start}
	if p.config.JourneyTracker == nil {
		log.Skipped = true
		log.Success = true
		log.EndedAt = time.Now()
		log.DurationMs = time.Since(start).Milliseconds()
		return log
	}
	if err := p.config.JourneyTracker.RecordTouch(ctx, req.CustomerID, req.Channel, "reach_pipeline"); err != nil {
		log.Success = false
		log.Error = err.Error()
	} else {
		log.Success = true
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// 9. 实际发送（状态确认）
// 注意：实际发送已在 runRetry 中执行（因为重试需要包裹发送）
// 此步骤仅用于确认发送状态；最终审计日志记录在 Send 方法末尾统一执行
func (p *defaultSendPipeline) runSend(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepSend, StartedAt: start}
	if resp.MessageID == "" {
		log.Success = false
		log.Error = "no message_id (send may have failed in retry step)"
	} else {
		log.Success = true
		log.Output = []any{map[string]any{
			"message_id": resp.MessageID,
			"channel":    resp.Channel,
		}}
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// SendPipelineStats Pipeline 统计（用于运维监控）
type SendPipelineStats struct {
	TotalSends   int64
	SuccessSends int64
	FailedSends  int64
	RateLimited  int64
	// DoNotContactSkipped R-3a：因命中全局退订标志位被跳过的次数
	DoNotContactSkipped int64
	// QuietHoursDeferred R-4：命中全渠道 quiet hours 进入延迟队列的次数
	QuietHoursDeferred int64
	FallbackUsed       int64
	TotalRetries       int64
	TotalCost          float64
}

// countedSendPipeline 带统计的 Pipeline 包装器
type countedSendPipeline struct {
	inner SendPipeline
	stats SendPipelineStats
	mu    sync.RWMutex
}

// NewCountedSendPipeline 创建带统计的 Pipeline
func NewCountedSendPipeline(inner SendPipeline) SendPipeline {
	return &countedSendPipeline{inner: inner}
}

// Send 执行并统计
func (p *countedSendPipeline) Send(ctx context.Context, req *ReachSendRequest) *SendResponse {
	resp := p.inner.Send(ctx, req)
	p.mu.Lock()
	p.stats.TotalSends++
	if resp.Success {
		p.stats.SuccessSends++
	} else {
		p.stats.FailedSends++
	}
	if resp.FallbackUsed {
		p.stats.FallbackUsed++
	}
	if resp.Deferred {
		p.stats.QuietHoursDeferred++
	}
	p.stats.TotalRetries += int64(resp.RetryCount)
	for _, step := range resp.StepResults {
		if !step.Success {
			switch step.Step {
			case SendStepRateLimit:
				p.stats.RateLimited++
			case SendStepPermission:
				// R-3a：全局退订标志位命中的跳过计数上报
				if step.Error == ErrSendDoNotContact.Error() {
					p.stats.DoNotContactSkipped++
				}
			}
		}
	}
	p.mu.Unlock()
	return resp
}

// Stats 返回统计快照
func (p *countedSendPipeline) Stats(ctx context.Context) SendPipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// FuncChannelAdapter 将普通发送函数适配为 ChannelAdapter 接口（真实实现，非 mock）。
type FuncChannelAdapter struct {
	SendFunc func(ctx context.Context, req *ReachSendRequest) (string, error)
	CallCnt  int32
}

// NewFuncChannelAdapter 创建函数式适配器
func NewFuncChannelAdapter(fn func(ctx context.Context, req *ReachSendRequest) (string, error)) *FuncChannelAdapter {
	return &FuncChannelAdapter{SendFunc: fn}
}

// Send 实现 ChannelAdapter
func (m *FuncChannelAdapter) Send(ctx context.Context, req *ReachSendRequest) (string, error) {
	atomic.AddInt32(&m.CallCnt, 1)
	if m.SendFunc != nil {
		return m.SendFunc(ctx, req)
	}
	return fmt.Sprintf("msg-%d", atomic.LoadInt32(&m.CallCnt)), nil
}

// Count 返回调用次数
func (m *FuncChannelAdapter) Count(ctx context.Context) int32 {
	return atomic.LoadInt32(&m.CallCnt)
}

// AlwaysFailAdapter 始终失败的适配器
type AlwaysFailAdapter struct {
	CallCnt int32
	Err     error
}

// NewAlwaysFailAdapter 创建始终失败的适配器
func NewAlwaysFailAdapter(err error) *AlwaysFailAdapter {
	if err == nil {
		err = errors.New("always fail")
	}
	return &AlwaysFailAdapter{Err: err}
}

// Send 实现 ChannelAdapter
func (a *AlwaysFailAdapter) Send(ctx context.Context, req *ReachSendRequest) (string, error) {
	atomic.AddInt32(&a.CallCnt, 1)
	return "", a.Err
}

// Count 返回调用次数
func (a *AlwaysFailAdapter) Count(ctx context.Context) int32 {
	return atomic.LoadInt32(&a.CallCnt)
}

// FlakyAdapter 不稳定适配器（前 N 次失败，之后成功）
type FlakyAdapter struct {
	CallCnt    int32
	FailBefore int32
	Err        error
}

// NewFlakyAdapter 创建不稳定适配器
func NewFlakyAdapter(failBefore int32) *FlakyAdapter {
	return &FlakyAdapter{
		FailBefore: failBefore,
		Err:        errors.New("flaky failure"),
	}
}

// Send 实现 ChannelAdapter
func (a *FlakyAdapter) Send(ctx context.Context, req *ReachSendRequest) (string, error) {
	cnt := atomic.AddInt32(&a.CallCnt, 1)
	if cnt <= a.FailBefore {
		return "", a.Err
	}
	return fmt.Sprintf("msg-flaky-%d", cnt), nil
}

// Count 返回调用次数
func (a *FlakyAdapter) Count(ctx context.Context) int32 {
	return atomic.LoadInt32(&a.CallCnt)
}
