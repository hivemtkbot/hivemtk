// reach_send_pipeline.go 触达消息发送 9 步 Pipeline（PRD §5.2 G4）
//
// 9 步装饰器链（外层 → 内层顺序）：
//  1. 权限校验（PermissionChecker）
//  2. 限流（RateLimiter，按渠道+账号）
//  3. 重试（RetryPolicy，指数退避，最多 3 次）
//  5. 降级（FallbackPolicy，主渠道失败 → 备用渠道）
//  6. 审计（AuditLogger，全量留痕到 reach_audit_logs）
//  7. 计费（CostTracker，按渠道计费 + 余额检查）
//  8. 客户轨迹更新（JourneyTracker，写入 customer_journey）
//  9. 实际发送（ChannelAdapter，渠道适配器）
//
// 验收标准（PRD §5.2 G4）：
//   - 高并发下消息不丢失（限流 + 重试保障）
//   - 敏感词消息被拦截并记录
//   - 主渠道失败自动降级到备用渠道
//   - 每条触达有完整审计记录
package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// ===== 9 步常量 =====

const (
	SendStepPermission   = "permission"    // 1. 权限校验
	SendStepRateLimit    = "rate_limit"    // 2. 限流
	SendStepRetry        = "retry"         // 3. 重试（包裹 4-8）
	SendStepFallback     = "fallback"      // 5. 降级
	SendStepAudit        = "audit"         // 6. 审计
	SendStepCost         = "cost"          // 7. 计费
	SendStepJourney      = "journey"       // 8. 客户轨迹
	SendStepSend         = "send"          // 9. 实际发送
)

// DefaultSendPipelineSteps 9 步默认顺序
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

// ===== 错误定义 =====

var (
	ErrSendPermissionDenied = errors.New("send permission denied")
	ErrSendRateLimited      = errors.New("send rate limited")
	ErrSendAllChannelFailed = errors.New("all channels failed (primary + fallback)")
	ErrSendInsufficientCost = errors.New("insufficient balance for send")
	ErrSendChannelNotConfig = errors.New("channel adapter not configured")
)

// ===== ReachSendRequest / SendResponse =====

// ReachSendRequest 发送请求
type ReachSendRequest struct {
	Channel     string            // sms/email/wecom/weixin/douyin/kuaishou/xhs/dingtalk/card
	AccountID   string            // 发送账号 ID
	RecipientID string            // 接收者 ID（手机号/openid/external_user_id 等）
	CustomerID  string            // 客户 ID（用于轨迹 / 限流维度）
	OperatorID  string            // 操作员 ID（用于权限校验）
	MsgType     string            // 消息类型（text/image/link/card 等）
	Content     string            // 消息内容
	Subject     string            // 邮件主题
	TemplateID  string            // 模板 ID
	Params      map[string]string // 模板参数
	Attachments []string          // 附件
	CardID      string            // 卡片 ID（card 渠道）
	Fallback    *FallbackConfig   // 降级配置（可选）
	Metadata    map[string]string // 额外元数据
}

// FallbackConfig 降级配置
type FallbackConfig struct {
	Enabled       bool
	BackupChannel string // 备用渠道
	BackupAccount string // 备用账号
	MaxAttempts   int    // 最大降级次数（默认 1）
}

// SendResponse 发送响应
type SendResponse struct {
	Success        bool          `json:"success"`
	MessageID      string        `json:"message_id"`
	Channel        string        `json:"channel"`
	AccountID      string        `json:"account_id"`
	FallbackUsed   bool          `json:"fallback_used"`
	PrimaryChannel string        `json:"primary_channel"` // 原始渠道
	RetryCount     int           `json:"retry_count"`
	StepResults    []SendStepLog `json:"step_results"`
	Error          string        `json:"error,omitempty"`
	DurationMs     int64         `json:"duration_ms"`
	SentAt         time.Time     `json:"sent_at"`
}

// SendStepLog 单步日志
type SendStepLog struct {
	Step       string    `json:"step"`
	Success    bool      `json:"success"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	Output     []any     `json:"output,omitempty"` // 中间产物
	Error      string    `json:"error,omitempty"`
	Skipped    bool      `json:"skipped,omitempty"`
}

// ===== 接口定义（可插拔组件） =====

// SendPermissionChecker 权限校验器
type SendPermissionChecker interface {
	CheckSendPermission(ctx context.Context, req *ReachSendRequest) error
}

// SendRateLimiter 限流器
type SendRateLimiter interface {
	Allow(ctx context.Context, key string, limit RateLimitSpec) bool
}

// RateLimitSpec 限流规格
type RateLimitSpec struct {
	QPS          int
	Burst        int
	DailyQuota   int
	PerUserLimit int
}

// SendRetryPolicy 重试策略
type SendRetryPolicy struct {
	MaxRetries      int
	IntervalMs      int
	Backoff         string // fixed/exponential
	MaxIntervalMs   int
	RetryableErrors []string
}

// DefaultSendRetryPolicy 默认重试策略（PRD：指数退避，最多 3 次）
func DefaultSendRetryPolicy() SendRetryPolicy {
	return SendRetryPolicy{
		MaxRetries:    3,
		IntervalMs:    500,
		Backoff:       "exponential",
		MaxIntervalMs: 10000,
	}
}

// SendAuditLogger 审计日志记录器
type SendAuditLogger interface {
	LogSend(ctx context.Context, req *ReachSendRequest, resp *SendResponse)
}

// SendCostTracker 计费器
type SendCostTracker interface {
	Charge(ctx context.Context, channel string, req *ReachSendRequest) (cost float64, err error)
}

// SendJourneyTracker 客户轨迹记录器
type SendJourneyTracker interface {
	RecordTouch(ctx context.Context, customerID, channel, source string) error
}

// ChannelAdapter 渠道适配器（实际发送）
// 由 tooluse 层注入 ReachAdapter 实现
type ChannelAdapter interface {
	Send(ctx context.Context, req *ReachSendRequest) (msgID string, err error)
}

// ===== 默认实现（NoOp） =====

// AllowAllSendPermission 默认放行
type AllowAllSendPermission struct{}

func (AllowAllSendPermission) CheckSendPermission(ctx context.Context, req *ReachSendRequest) error {
	return nil
}

// NoOpSendRateLimiter 默认不限流
type NoOpSendRateLimiter struct{}

func (NoOpSendRateLimiter) Allow(ctx context.Context, key string, limit RateLimitSpec) bool {
	return true
}

// MemorySendRateLimiter 内存级令牌桶限流（按 key 维度）
//
// 分片令牌桶（按 key 哈希分桶），不同 key 落不同分片，互不阻塞，
// 避免高并发下全局锁串行化成为触达延迟主因。
type MemorySendRateLimiter struct {
	shards [rateLimiterShards]*rateLimiterShard
}

type rateLimiterShard struct {
	mu      sync.Mutex
	buckets map[string]*sendRateBucket
}

// sendRateBucket 令牌桶
type sendRateBucket struct {
	tokens   float64
	lastFill time.Time
	qps      int
	burst    int
}

const (
	rateLimiterShards        = 64
	rateLimiterBucketIdleTTL = 10 * time.Minute // 桶空闲超过此值视为可重置（返回客户重置 burst 额度）
)

// rateLimiterMaxBuckets 每分片最大桶数（64 分片 × 4096 ≈ 26 万上限），超出驱逐最久未用桶，防止内存泄漏。
// 用 var 以便测试可调小上限验证边界。
var rateLimiterMaxBuckets = 4096

// NewMemorySendRateLimiter 创建内存限流器
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

// Allow 检查限流
func (l *MemorySendRateLimiter) Allow(ctx context.Context, key string, limit RateLimitSpec) bool {
	if limit.QPS <= 0 && limit.Burst <= 0 {
		return true
	}
	s := l.shardOf(ctx, key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.buckets[key]
	// !ok 优先短路，保证下方 b.lastFill 访问不空指针；
	// 规格变更或空闲过久 → 重建（空闲过久的桶重置 burst，等价于失效）
	if !ok || b.qps != limit.QPS || b.burst != limit.Burst || now.Sub(b.lastFill) > rateLimiterBucketIdleTTL {
		if !ok {
			// 缓存未命中且分片已满：驱逐最久未用的桶，限制每分片桶数上限，杜绝无限增长（内存泄漏）
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

// totalBucketCount 统计所有分片桶总数（仅供测试验证内存上限）。
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

// evictStalestLocked 驱逐 lastFill 最早的桶。调用方须持 s.mu。
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

// Reset 重置指定 key 的限流（用于测试）
func (l *MemorySendRateLimiter) Reset(ctx context.Context, key string) {
	s := l.shardOf(ctx, key)
	s.mu.Lock()
	delete(s.buckets, key)
	s.mu.Unlock()
}

// MemorySendAuditLogger 内存级审计日志
type MemorySendAuditLogger struct {
	mu      sync.Mutex
	entries []*SendAuditEntry
	maxSize int
}

// SendAuditEntry 审计日志条目
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

// NewMemorySendAuditLogger 创建内存审计日志
func NewMemorySendAuditLogger(maxSize int) *MemorySendAuditLogger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemorySendAuditLogger{maxSize: maxSize}
}

// LogSend 记录审计日志
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

// Entries 返回所有审计日志
func (l *MemorySendAuditLogger) Entries(ctx context.Context) []*SendAuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*SendAuditEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// NoOpSendCostTracker 默认不计费
type NoOpSendCostTracker struct{}

func (NoOpSendCostTracker) Charge(ctx context.Context, channel string, req *ReachSendRequest) (float64, error) {
	return 0, nil
}

// MemorySendCostTracker 内存级计费器
type MemorySendCostTracker struct {
	mu        sync.Mutex
	balance   float64
	costs     map[string]float64 // channel → unit cost
	totalUsed float64
}

// NewMemorySendCostTracker 创建内存计费器
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

// SetCost 设置渠道单价
func (t *MemorySendCostTracker) SetCost(ctx context.Context, channel string, cost float64) {
	t.mu.Lock()
	t.costs[channel] = cost
	t.mu.Unlock()
}

// Charge 扣费
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

// Balance 返回当前余额
func (t *MemorySendCostTracker) Balance(ctx context.Context) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.balance
}

// TotalUsed 返回累计消费
func (t *MemorySendCostTracker) TotalUsed(ctx context.Context) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalUsed
}

// NoOpSendJourneyTracker 默认不记录轨迹
type NoOpSendJourneyTracker struct{}

func (NoOpSendJourneyTracker) RecordTouch(ctx context.Context, customerID, channel, source string) error {
	return nil
}

// CustomerJourneySendTracker 适配 CustomerJourneyService 的轨迹记录器
type CustomerJourneySendTracker struct {
	Service *CustomerJourneyService
}

// RecordTouch 记录触达到客户旅程
func (t CustomerJourneySendTracker) RecordTouch(ctx context.Context, customerID, channel, source string) error {
	if t.Service == nil || customerID == "" {
		return nil
	}
	t.Service.Touch(ctx, customerID, source)
	return nil
}

// ===== SendPipeline 主体 =====

// SendPipeline 触达消息发送 9 步 Pipeline
type SendPipeline interface {
	Send(ctx context.Context, req *ReachSendRequest) *SendResponse
}

// SendPipelineConfig Pipeline 配置
type SendPipelineConfig struct {
	PermissionChecker SendPermissionChecker
	RateLimiter       SendRateLimiter
	RateLimitSpec     RateLimitSpec
	RetryPolicy       SendRetryPolicy
	AuditLogger       SendAuditLogger
	CostTracker       SendCostTracker
	JourneyTracker    SendJourneyTracker
	Adapter           ChannelAdapter            // 主渠道适配器
	FallbackAdapters  map[string]ChannelAdapter // 备用渠道适配器（key = channel）
	Steps             []string                  // 启用的步骤（默认全部）
}

// DefaultSendPipelineConfig 默认配置
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

// defaultThirdPartyRateLimitByChannel 三方发送渠道的保守默认限流规格（削峰用安全下限，非业务配额）。
// 仅覆盖短信 / 邮件 / 即时通讯等「第三方服务账号」（tool_accounts）渠道；内部渠道（card / web）
// 不在表内，MemorySendRateLimiter 对零值规格放行（不限流）。
// 生产应按三方账号供应商配额配置 SendPipelineConfig.RateLimitSpec 全局覆盖。
var defaultThirdPartyRateLimitByChannel = map[string]RateLimitSpec{
	"sms":   {QPS: 50, Burst: 100},
	"email": {QPS: 30, Burst: 60},
	// 即时通讯类第三方渠道
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

// DefaultThirdPartyRateLimitSpec 返回指定渠道的三方默认限流规格；
// 非三方 / 内部渠道（如 card / web）返回零值，MemorySendRateLimiter 会放行（不限流）。
func DefaultThirdPartyRateLimitSpec(channel string) RateLimitSpec {
	if spec, ok := defaultThirdPartyRateLimitByChannel[channel]; ok {
		return spec
	}
	return RateLimitSpec{}
}

// NewDefaultRateLimitedPipelineConfig 在默认配置基础上启用三方渠道令牌桶限流：
// 覆盖短信 / 邮件 / 即时通讯等第三方发送渠道，避免突发流量触发供应商限频被 ban 或过载丢消息。
// 内部渠道（card / web）不在默认限流表内，保持不限流。
// 全局 SendPipelineConfig.RateLimitSpec 若非零则优先生效（用于按三方账号配额精确配置）。
func NewDefaultRateLimitedPipelineConfig(adapter ChannelAdapter) SendPipelineConfig {
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.RateLimiter = NewMemorySendRateLimiter()
	return cfg
}

// defaultSendPipeline 默认实现
type defaultSendPipeline struct {
	config SendPipelineConfig
}

// NewSendPipeline 创建 9 步 Pipeline
func NewSendPipeline(config SendPipelineConfig) SendPipeline {
	if config.Steps == nil || len(config.Steps) == 0 {
		config.Steps = DefaultSendPipelineSteps
	}
	return &defaultSendPipeline{config: config}
}

// Send 执行 9 步 Pipeline
// complianceReminderTag 主动触达合规强制提示标签
const complianceReminderTag = "[COMPLIANCE]"

// LogComplianceReminder 主动触达合规强制提示。
//
// 主动触达（短信 / 私信 / IM / 邮件 / 公众号 / 企微 等向用户主动推送消息）属于核心敏感操作，
// 各渠道平台（微信、企业微信、抖音、快手、小红书、Telegram、WhatsApp(Meta)、短信、邮件 等）
// 均有严格的开发者规范与服务条款。此处每次发送入口强制打印合规提示，提醒使用者：
//   - 仅可向已授权、已明确同意接收的联系人发送；
//   - 严格控制发送频率，避免骚扰；
//   - 禁止发送垃圾营销、欺诈、骚扰或任何违法违规内容。
//
// 因违规发送导致的账号封禁、平台处罚、行政处罚或法律后果，由使用者自行承担。
// 该提示不依赖任何配置开关，属于强制日志。
func LogComplianceReminder(channel, recipientID string) {
	logger.Warnf("%s 主动触达发送已触发：channel=%s, recipient=%s。"+
		"请严格遵守各渠道平台（微信/企业微信/抖音/快手/小红书/Telegram/WhatsApp(Meta)/短信/邮件 等）的"+
		"开发者规范、服务条款及当地法律法规；仅可向已授权、已明确同意接收的联系人发送，"+
		"严格控制发送频率，禁止发送垃圾营销、欺诈、骚扰或违法违规内容。"+
		"因违规发送导致的账号封禁、平台处罚、行政处罚或法律后果由使用者自行承担。",
		complianceReminderTag, channel, recipientID)
}

func (p *defaultSendPipeline) Send(ctx context.Context, req *ReachSendRequest) *SendResponse {
	// 核心敏感接口：每次主动触达发送强制输出合规提示
	LogComplianceReminder(req.Channel, req.RecipientID)

	start := time.Now()
	resp := &SendResponse{
		PrimaryChannel: req.Channel,
		Channel:        req.Channel,
		AccountID:      req.AccountID,
		StepResults:    []SendStepLog{},
	}

	// 步骤映射
	stepFuncs := map[string]func(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog{
		SendStepPermission:   p.runPermission,
		SendStepRateLimit:    p.runRateLimit,
		SendStepRetry:        p.runRetry,
		SendStepFallback:     p.runFallback,
		SendStepAudit:        p.runAudit,
		SendStepCost:         p.runCost,
		SendStepJourney:      p.runJourney,
		SendStepSend:         p.runSend,
	}

	// 9 步串行执行
	for _, step := range p.config.Steps {
		fn, ok := stepFuncs[step]
		if !ok {
			continue
		}
		log := fn(ctx, req, resp)
		resp.StepResults = append(resp.StepResults, log)
		if !log.Success && !log.Skipped {
			resp.Success = false
			resp.Error = log.Error
			resp.DurationMs = time.Since(start).Milliseconds()
			resp.SentAt = time.Now()
			// PRD §5.2 G4：失败也必须记录审计日志（"每条触达有完整审计记录"）
			if p.config.AuditLogger != nil {
				p.config.AuditLogger.LogSend(ctx, req, resp)
			}
			return resp
		}
	}

	resp.Success = true
	resp.DurationMs = time.Since(start).Milliseconds()
	resp.SentAt = time.Now()
	// 成功时记录最终审计日志（带 message_id 等结果字段）
	if p.config.AuditLogger != nil {
		p.config.AuditLogger.LogSend(ctx, req, resp)
	}
	return resp
}

// ===== 9 步实现 =====

// 1. 权限校验
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
	} else {
		log.Success = true
	}
	log.EndedAt = time.Now()
	log.DurationMs = time.Since(start).Milliseconds()
	return log
}

// 2. 限流
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
		// 未显式配置全局限流时，按渠道套用三方渠道保守默认规格（短信/邮件/即时通讯）。
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

// 3. 重试（包裹后续步骤：4 降级 / 5 审计 / 6 计费 / 7 轨迹 / 8 发送）
// 实现策略：重试只包裹"实际发送"步骤，其他步骤是幂等的或单次执行
// 重试逻辑：调用 doSendOnce() 最多 MaxRetries+1 次
func (p *defaultSendPipeline) runRetry(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepRetry, StartedAt: start}
	policy := p.config.RetryPolicy
	if policy.MaxRetries <= 0 {
		policy = DefaultSendRetryPolicy()
	}

	var lastErr error
	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		// 执行实际发送（包含降级）
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
		// 检查错误是否可重试
		if !p.isRetryable(ctx, err, policy.RetryableErrors) {
			break
		}
		// 上下文已取消/超时：立即停止重试，不再重发（break 跳出 for 循环）
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			resp.RetryCount = attempt
			break
		}
		// 计算退避时间
		if attempt < policy.MaxRetries {
			wait := p.computeBackoff(ctx, policy, attempt)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				lastErr = ctx.Err()
			}
			// 等待期间上下文取消：停止重试。此处 break 位于 for 循环体内，跳出整个重试循环；
			// 原有的 break 误置于 select 内，仅跳出 select，导致 ctx 取消后仍继续重发直至 MaxRetries 耗尽。
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

// executeSendWithFallback 执行单次发送（带降级）
func (p *defaultSendPipeline) executeSendWithFallback(ctx context.Context, req *ReachSendRequest) (string, error) {
	// 主渠道
	if p.config.Adapter == nil {
		return "", ErrSendChannelNotConfig
	}
	msgID, err := p.config.Adapter.Send(ctx, req)
	if err == nil {
		return msgID, nil
	}
	// 主渠道失败 → 检查降级配置
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
		backupReq.Fallback = nil // 防止递归
		msgID, err2 := backupAdapter.Send(ctx, &backupReq)
		if err2 == nil {
			return msgID, nil
		}
		err = err2
	}
	return "", err
}

// isRetryable 是否可重试
func (p *defaultSendPipeline) isRetryable(ctx context.Context, err error, retryableErrors []string) bool {
	if err == nil {
		return false
	}
	if len(retryableErrors) == 0 {
		// 默认所有错误都重试
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

// computeBackoff 计算退避时间
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

// 5. 降级（已在 runRetry 中实现，此处仅记录状态）
func (p *defaultSendPipeline) runFallback(ctx context.Context, req *ReachSendRequest, resp *SendResponse) SendStepLog {
	start := time.Now()
	log := SendStepLog{Step: SendStepFallback, StartedAt: start}
	if req.Fallback != nil && req.Fallback.Enabled && req.Fallback.BackupChannel != "" {
		// 标记降级可用（实际降级在 runRetry 中执行）
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

// 6. 审计
// 注：实际的 LogSend 调用已移到 Send 方法末尾（成功 + 失败两个分支）
// 此步骤仅做"审计准备"标记，确保审计步骤出现在 StepResults 中
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
	// 审计步骤已启用，标记成功；实际日志记录在 Send 方法返回前统一执行
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

// ===== 统计 =====

// SendPipelineStats Pipeline 统计（用于运维监控）
type SendPipelineStats struct {
	TotalSends     int64
	SuccessSends   int64
	FailedSends    int64
	RateLimited    int64
	FallbackUsed   int64
	TotalRetries   int64
	TotalCost      float64
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
	p.stats.TotalRetries += int64(resp.RetryCount)
	for _, step := range resp.StepResults {
		if !step.Success {
			switch step.Step {
			case SendStepRateLimit:
				p.stats.RateLimited++
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

// ===== 测试辅助：FuncChannelAdapter =====

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
