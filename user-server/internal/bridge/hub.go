package bridge

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/pkg/utils/logger"
)

// Sentinel errors（统一供上层 errors.Is 判定）
var (
	// ErrBridgeOffline 账号未连接（扩展离线）
	ErrBridgeOffline = errors.New("bridge: account not connected")
	// ErrBridgeRateLimited 账号下行限速（兜底：60/min）
	ErrBridgeRateLimited = errors.New("bridge: account deliver rate limited")
	// ErrBridgeBufferFull 发送缓冲已满
	ErrBridgeBufferFull = errors.New("bridge: send buffer full")
)

// =============================================================
// 桥接默认参数（与前端 constants.js / DEFAULTS.md 严格对齐）
// 文档源：docs/bridge/DEFAULTS.md，user-web/bridge/src/core/constants.js
// =============================================================
const (
	// DeliverRateLimitPerMin 单账号每分钟下行限速（兜底）
	// 扩展端 12/min（更严格）；服务端 60/min 仅防止失控洪泛
	// 调整需同步前端 RATE_LIMIT_DEFAULTS.accountCapacity
	DeliverRateLimitPerMin = 60
	// JanitorInterval rate bucket 清理周期
	JanitorInterval = 60 * time.Second
	// JanitorIdleTTL rate bucket 空闲超时（无活动后被回收）
	JanitorIdleTTL = 10 * time.Minute
)

// rateBucket 极简令牌桶
type rateBucket struct {
	mu      sync.Mutex
	tokens  float64
	cap     float64
	refill  float64 // 令牌/毫秒
	last    time.Time
	lastHit time.Time
}

func newRateBucket(capacity float64, perMin float64) *rateBucket {
	return &rateBucket{tokens: capacity, cap: capacity, refill: perMin / 60000, last: time.Now(), lastHit: time.Now()}
}

func (b *rateBucket) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens = math.Min(b.cap, b.tokens+float64(now.Sub(b.last).Milliseconds())*b.refill)
	b.last = now
	if b.tokens >= 1 {
		b.tokens -= 1
		b.lastHit = now
		return true
	}
	return false
}

// idleSince 返回该桶最近一次取到令牌的时间，用于 janitor 判定空闲
func (b *rateBucket) idleSince() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastHit
}

// BridgeHub 桥接连接中心：按 (channel, account_id) 索引扩展连接
//
// 设计：互斥锁保护的 map，无独立事件循环（心跳由每个连接的 writePump 负责）。
// 同一账号同时只保持一条活跃连接（后连顶前连），避免双发。
//
// 资源回收：
//   - rateBuckets 由 janitor 协程定期清理空闲桶（防止长期运行内存单调上涨）
//   - clients 通过 Unregister 删除
type BridgeHub struct {
	mu      sync.RWMutex
	clients map[string]*BridgeClient

	// 每账号下行限速护栏（防 AI 失控洪泛；扩展端已做拟人限速，此处为最后兜底）
	rateMu      sync.Mutex
	rateBuckets map[string]*rateBucket

	// 单调递增的全局 seq（用于下行帧去重/排序）
	seqCounter atomic.Int64

	// 关闭信号：收到后 stopJanitor 退出
	stopJanitor chan struct{}
	janitorDone chan struct{}
}

// NewBridgeHub 创建桥接中心
func NewBridgeHub() *BridgeHub {
	return &BridgeHub{
		clients:     make(map[string]*BridgeClient),
		rateBuckets: make(map[string]*rateBucket),
		stopJanitor: make(chan struct{}),
		janitorDone: make(chan struct{}),
	}
}

var (
	globalHub *BridgeHub
	hubOnce   sync.Once
)

// GetBridgeHub 返回全局单例（仿 websocket.GetHub 的 sync.Once 模式）
func GetBridgeHub() *BridgeHub {
	hubOnce.Do(func() { globalHub = NewBridgeHub() })
	return globalHub
}

// StartJanitor 启动后台 janitor 协程：定期清理空闲 rateBucket + 关闭信号
//
// 间隔与空闲阈值：见 JanitorInterval / JanitorIdleTTL 常量（单源来自 DEFAULTS.md）
// 安全：与 IsOnline/Deliver 的锁独立，不会阻塞业务路径。
func (h *BridgeHub) StartJanitor() {
	go h.runJanitor(JanitorInterval, JanitorIdleTTL)
}

// StartJanitorWith 自定义间隔（仅供测试用，生产代码用 StartJanitor）
func (h *BridgeHub) StartJanitorWith(interval, idleTTL time.Duration) {
	go h.runJanitor(interval, idleTTL)
}

func (h *BridgeHub) runJanitor(interval, idleTTL time.Duration) {
	defer close(h.janitorDone)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-h.stopJanitor:
			return
		case <-t.C:
			h.cleanupIdleRateBuckets(idleTTL)
		}
	}
}

func (h *BridgeHub) cleanupIdleRateBuckets(idleTTL time.Duration) {
	cutoff := time.Now().Add(-idleTTL)
	var removed int
	h.rateMu.Lock()
	for k, b := range h.rateBuckets {
		if b.idleSince().Before(cutoff) {
			delete(h.rateBuckets, k)
			removed++
		}
	}
	h.rateMu.Unlock()
	if removed > 0 {
		logger.Ctx(nil).Info().
			Str("module", "bridge").
			Int("removed_buckets", removed).
			Int("remaining", len(h.rateBuckets)).
			Msg("rate bucket janitor cleanup")
	}
}

// Shutdown 优雅关闭：停止 janitor 协程 + 关闭所有活跃 WS
func (h *BridgeHub) Shutdown() {
	// 停止 janitor
	select {
	case <-h.stopJanitor:
		// 已关闭
	default:
		close(h.stopJanitor)
	}
	// 关闭所有活跃连接
	h.mu.Lock()
	clients := make([]*BridgeClient, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.clients = make(map[string]*BridgeClient)
	h.mu.Unlock()
	for _, c := range clients {
		c.Kick()
	}
}

func (h *BridgeHub) key(channel, account string) string {
	return channel + ":" + account
}

// Register 注册连接；若同 key 已存在旧连接，返回旧连接由调用方踢除
func (h *BridgeHub) Register(c *BridgeClient) *BridgeClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	k := h.key(c.channel, c.account)
	var old *BridgeClient
	if existing, ok := h.clients[k]; ok {
		old = existing
	}
	h.clients[k] = c
	// 扩展（重）上线：异步触发离线失败消息补发（P1-7 修复）。
	// 异步执行避免阻塞 Register 的锁区间；recover 防止个别账号补发异常影响连接建立。
	if OnBridgeClientOnline != nil {
		go func(ch, acc string) {
			defer func() { _ = recover() }()
			OnBridgeClientOnline(ch, acc)
		}(c.channel, c.account)
	}
	return old
}

// Unregister 注销连接（仅当当前映射仍是该连接时）
func (h *BridgeHub) Unregister(c *BridgeClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	k := h.key(c.channel, c.account)
	if cur, ok := h.clients[k]; ok && cur == c {
		delete(h.clients, k)
	}
	// 标记 client 已关闭，Deliver 看到 closed 即跳过（防 close-after-send panic）
	c.CloseSend()
}

// IsOnline 判断账号是否在线
func (h *BridgeHub) IsOnline(channel, account string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[h.key(channel, account)]
	if !ok {
		return false
	}
	return !c.IsClosed()
}

// NextSeq 生成下一个全局单调递增 seq（用于下行帧排序/去重）
func (h *BridgeHub) NextSeq() int64 {
	return h.seqCounter.Add(1)
}

// Deliver 向指定账号下发回复；离线返回 ErrBridgeOffline
//
// 线程安全：取客户端、判 closed、写 send 全在 mu 锁内完成，
// 避免在并发 close(c.send) 之后向已关闭 channel 发送导致 panic。
func (h *BridgeHub) Deliver(channel, account string, payload *UnifiedReply) error {
	// 每账号下行限速护栏（兜底）：超过 60/min 直接丢弃，防止失控洪泛。
	// key 为 channel:accountID（同一账号的不同渠道独立限速，避免跨渠道互相限流）。
	rateKey := h.key(channel, account)
	h.rateMu.Lock()
	b, ok := h.rateBuckets[rateKey]
	if !ok {
		b = newRateBucket(DeliverRateLimitPerMin, DeliverRateLimitPerMin)
		h.rateBuckets[rateKey] = b
	}
	allowed := b.take()
	h.rateMu.Unlock()
	if !allowed {
		// 私域: 无 Prometheus, 审计日志已落库
		logger.Ctx(nil).Warn().
			Str("module", "bridge").
			Str("channel", channel).
			Str("account_id", account).
			Msg("bridge deliver rate limited (60/min)")
		return ErrBridgeRateLimited
	}

	h.mu.Lock()
	c, ok := h.clients[h.key(channel, account)]
	if !ok || c.IsClosed() {
		h.mu.Unlock()
		return ErrBridgeOffline
	}
	data, err := json.Marshal(&Frame{
		Type:  FrameOutboundReply,
		Seq:   h.seqCounter.Add(1),
		Reply: payload,
	})
	if err != nil {
		h.mu.Unlock()
		logger.Ctx(nil).Error().Err(err).Str("module", "bridge").Msg("bridge deliver marshal failed")
		return err
	}
	// 写入 send 通道：不直接 drop（避免静默丢消息）。
	// 改为带短超时的有界阻塞：给 writePump 腾出缓冲的机会，
	// 超时仍未腾出再显式返回 ErrBridgeBufferFull，由上层（BridgeReachAdapter.deliverWS）
	// 降级落盘到 message_hub 等待补发（不在此处静默丢弃）。
	select {
	case c.send <- data:
		h.mu.Unlock()
		return nil
	case <-time.After(deliverSendTimeout):
		h.mu.Unlock()
		logger.Ctx(nil).Warn().
			Str("module", "bridge").
			Str("channel", channel).
			Str("account_id", account).
			Msg("bridge send buffer full (timeout), will fallback to persist")
		return ErrBridgeBufferFull
	}
}

// deliverSendTimeout 下行帧写入 send 通道的阻塞超时（避免缓冲满时静默丢消息）。
// 远小于 writeWait（10s）；writePump 每 pingPeriod(50s) 也会 drain，正常 256 容量足够。
const deliverSendTimeout = 500 * time.Millisecond
