package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"marketing/internal/cache"
	"marketing/internal/pkg/utils/logger"
)

// 方向7: 转人工门禁数据流向
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/转人工门禁数据流向图.md
//
// 核心职责：
//  1. 触发转人工熔断（高危危机感命中时执行）：
//     - 下发 Redis 分布式协同锁，永久锁定当前会话为人工状态
//     - 记录转人工原因（危机感/情绪/意图）
//     - 推送"叮咚"通知到商户异步总线（hivemtk:queue:merchant_notif）
//  2. 检查会话是否已锁定（路由前置阻断器）：
//     - 高频调用，命中时直接放行（AI 不再调用大模型）
//  3. 释放人工锁定（坐席主动"解决"会话时调用）
//  4. 通知总线消费者（坐席 Vue 3 工作台 WebSocket）
//
// Redis Key 规范：
//   - hivemtk:lock:human:{sessionID}            // 人工接管锁（永久）
//   - hivemtk:session:escalate_reason:{sessionID}  // 转人工原因（24h TTL）
//   - hivemtk:queue:merchant_notif              // 商户通知队列（LPUSH/BLPOP）

const (
	// HumanLockKey 会话被人工接管时永久锁定（AI 路由绕过）
	HumanLockKey = "hivemtk:lock:human:"
	// HumanReasonKey 转人工原因
	HumanReasonKey = "hivemtk:session:escalate_reason:"
	// MerchantNotifQueue 商户异步通知队列
	MerchantNotifQueue = "hivemtk:queue:merchant_notif"
	// HumanLockReasonTTL 转人工原因 TTL（24h）
	HumanLockReasonTTL = 24 * time.Hour
)

// EscalationEvent 转人工事件载荷（推送到商户通知队列）
type EscalationEvent struct {
	Event     string    `json:"event"`      // 事件类型（TRANSFER_TO_HUMAN）
	SessionID string    `json:"session_id"` // 会话 ID
	Reason    string    `json:"reason"`     // 转人工原因
	Severity  string    `json:"severity"`   // 严重度（high/medium/low）
	Timestamp time.Time `json:"timestamp"`  // 事件时间
	Channel   string    `json:"channel,omitempty"`    // 渠道
	CustomerID string   `json:"customer_id,omitempty"` // 客户 ID
	AgentCode  string   `json:"agent_code,omitempty"`  // 智能体代号
}

// HumanEscalationManager 转人工协同状态机中枢
//
// 在 B5 时机被后台 AgentRuntime 状态机调用：检测到危机感 >= 4 / 情绪=angry / 强转人工意图
// 任何一个条件被命中，立即触发熔断。
type HumanEscalationManager struct {
	cache cache.Cache

	// 通知总线抽象（可注入；默认走 cache.LPush）
	notifier Notifier

	// 内部统计
	mu    sync.RWMutex
	stats EscalationStats
}

// Notifier 通知接口（解耦消息总线实现）
type Notifier interface {
	// Notify 转人工事件
	Notify(ctx context.Context, event *EscalationEvent) error
}

// EscalationStats 转人工统计
type EscalationStats struct {
	TotalTriggers      int64      // 总触发次数
	TotalRejections    int64      // 缓存拒绝次数
	TotalNotifications int64      // 通知推送成功次数
	LastTriggerAt      time.Time  // 最近触发时间
	LastSessionID      string     // 最近会话 ID
	TriggersByReason   sync.Map   // reason -> count
}

// NewHumanEscalationManager 构造转人工管理器
func NewHumanEscalationManager(c cache.Cache) *HumanEscalationManager {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	return &HumanEscalationManager{
		cache:    c,
		notifier: NewCacheNotifier(c),
	}
}

// SetNotifier 自定义通知实现（如 Kafka / RocketMQ）
func (h *HumanEscalationManager) SetNotifier(n Notifier) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifier = n
}

// TriggerCensorshipEscalation 触发转人工熔断
//
// 入参：
//   - sessionID: 会话唯一 ID
//   - reason: 触发原因（如 "crisis_high:骗子" / "angry_sentiment" / "handoff_human_intent"）
//
// 流程：
//  1. 下发 Redis 分布式锁（永久）
//  2. 记录转人工原因（24h TTL）
//  3. 推送"叮咚"通知到商户异步总线
//  4. 记录统计
//
// 返回：
//   - error: 仅在 Redis 锁下发失败时返回
func (h *HumanEscalationManager) TriggerCensorshipEscalation(ctx context.Context, sessionID, reason string) error {
	if sessionID == "" {
		return errors.New("sessionID required")
	}
	if h.cache == nil {
		return errors.New("cache unavailable")
	}

	// 1. 下发 Redis 永久锁
	lockKey := HumanLockKey + sessionID
	if err := h.cache.Set(ctx, lockKey, "true", 0); err != nil {
		h.mu.Lock()
		h.stats.TotalRejections++
		h.mu.Unlock()
		return fmt.Errorf("set human lock failed: %w", err)
	}

	// 2. 记录转人工原因
	reasonKey := HumanReasonKey + sessionID
	_ = h.cache.Set(ctx, reasonKey, reason, HumanLockReasonTTL)

	// 3. 推送"叮咚"通知
	event := &EscalationEvent{
		Event:     "TRANSFER_TO_HUMAN",
		SessionID: sessionID,
		Reason:    reason,
		Severity:  severityFromReason(reason),
		Timestamp: time.Now(),
	}
	if h.notifier != nil {
		if err := h.notifier.Notify(ctx, event); err != nil {
			logger.Warnf("[HumanEscalation] notify failed session=%s err=%v", sessionID, err)
		}
	}

	// 4. 统计
	h.mu.Lock()
	h.stats.TotalTriggers++
	h.stats.LastTriggerAt = time.Now()
	h.stats.LastSessionID = sessionID
	if v, ok := h.stats.TriggersByReason.Load(reason); ok {
		h.stats.TriggersByReason.Store(reason, v.(int64)+1)
	} else {
		h.stats.TriggersByReason.Store(reason, int64(1))
	}
	h.mu.Unlock()

	logger.Infof("[HumanEscalation] session=%s 触发转人工熔断，原因=%s", sessionID, reason)
	return nil
}

// IsSessionLockedForHuman 检查会话是否被人工接管
//
// 用作 AI 大模型调用前的"前置阻断器"：
//   - 返回 true：AI 彻底闭嘴，消息直接路由到坐席工作台
//   - 返回 false：进入正常 AI 路由
//
// 缓存降级策略：Redis 不可用时返回 false（保守路由到 AI，保证可用性）
func (h *HumanEscalationManager) IsSessionLockedForHuman(ctx context.Context, sessionID string) bool {
	if h.cache == nil || sessionID == "" {
		return false
	}
	key := HumanLockKey + sessionID
	val, err := h.cache.Get(ctx, key)
	if err != nil {
		return false
	}
	return val == "true"
}

// GetEscalationReason 获取转人工原因
func (h *HumanEscalationManager) GetEscalationReason(ctx context.Context, sessionID string) (string, error) {
	if h.cache == nil || sessionID == "" {
		return "", errors.New("cache unavailable")
	}
	reason, err := h.cache.Get(ctx, HumanReasonKey+sessionID)
	if err != nil {
		return "", err
	}
	return reason, nil
}

// ReleaseHumanLock 解除人工接管锁（坐席主动"解决"会话时调用）
func (h *HumanEscalationManager) ReleaseHumanLock(ctx context.Context, sessionID string) error {
	if h.cache == nil || sessionID == "" {
		return errors.New("cache or sessionID unavailable")
	}
	if err := h.cache.Delete(ctx, HumanLockKey+sessionID); err != nil {
		return err
	}
	_ = h.cache.Delete(ctx, HumanReasonKey+sessionID)
	logger.Infof("[HumanEscalation] session=%s 已解除人工接管", sessionID)
	return nil
}

// GetStats 获取统计
func (h *HumanEscalationManager) GetStats() EscalationStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.stats
}

// ============================================================================
// 内部辅助
// ============================================================================

// severityFromReason 从原因推断严重度
func severityFromReason(reason string) string {
	switch {
	case len(reason) == 0:
		return "low"
	case hasAnySubstring(reason, []string{"high_risk", "handoff_human", "scam", "fraud", "lawsuit"}):
		return "high"
	case hasAnySubstring(reason, []string{"medium_risk", "complaint", "angry"}):
		return "medium"
	default:
		return "low"
	}
}

func hasAnySubstring(s string, subs []string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// ============================================================================
// CacheNotifier 默认通知实现（通过 Redis 列表推送到商户总线）
// ============================================================================

// CacheNotifier 基于 Cache 的默认通知器
type CacheNotifier struct {
	cache cache.Cache
	queue string
}

// NewCacheNotifier 构造默认通知器
func NewCacheNotifier(c cache.Cache) *CacheNotifier {
	return &CacheNotifier{cache: c, queue: MerchantNotifQueue}
}

// Notify 通过 LPush 推送到商户通知队列
func (n *CacheNotifier) Notify(ctx context.Context, event *EscalationEvent) error {
	if n.cache == nil {
		return errors.New("cache unavailable")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return n.cache.LPush(ctx, n.queue, string(payload), 0)
}

// ============================================================================
// 全局单例（可选）
// ============================================================================

var (
	globalEscalationMgr *HumanEscalationManager
	onceEscalationMgr   sync.Once
)

// GetGlobalEscalationManager 获取全局转人工管理器
func GetGlobalEscalationManager() *HumanEscalationManager {
	onceEscalationMgr.Do(func() {
		globalEscalationMgr = NewHumanEscalationManager(cache.GetGlobalCache())
	})
	return globalEscalationMgr
}
