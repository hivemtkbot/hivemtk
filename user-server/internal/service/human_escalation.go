package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
)


const (
	HumanLockKey = "hivemtk:lock:human:"
	HumanReasonKey = "hivemtk:session:escalate_reason:"
	MerchantNotifQueue = "hivemtk:queue:merchant_notif"
	HumanLockReasonTTL = 24 * time.Hour
)

// EscalationEvent 转人工事件载荷（推送到商户通知队列）
type EscalationEvent struct {
	Event      string    `json:"event"`                 
	SessionID  string    `json:"session_id"`            
	Reason     string    `json:"reason"`                
	Severity   string    `json:"severity"`              
	Timestamp  time.Time `json:"timestamp"`             
	Channel    string    `json:"channel,omitempty"`     
	CustomerID string    `json:"customer_id,omitempty"` 
	AgentCode  string    `json:"agent_code,omitempty"`  
}

// HumanEscalationManager 转人工协同状态机中枢
//
// 在 B5 时机被后台 AgentRuntime 状态机调用：检测到危机感 >= 4 / 情绪=angry / 强转人工意图
// 任何一个条件被命中，立即触发熔断。
type HumanEscalationManager struct {
	cache cache.Cache

	notifier Notifier

	mu    sync.RWMutex
	stats EscalationStats
}

// Notifier 通知接口（解耦消息总线实现）
type Notifier interface {
	Notify(ctx context.Context, event *EscalationEvent) error
}

// EscalationStats 转人工统计
type EscalationStats struct {
	TotalTriggers      int64     
	TotalRejections    int64     
	TotalNotifications int64     
	LastTriggerAt      time.Time 
	LastSessionID      string    
	TriggersByReason   sync.Map  
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

// SetNotifier 自定义通知实现（如 邮件 / 钉钉 / 飞书 webhook）
func (h *HumanEscalationManager) SetNotifier(ctx context.Context, n Notifier) {
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

	lockKey := HumanLockKey + sessionID
	if err := h.cache.Set(ctx, lockKey, "true", 0); err != nil {
		h.mu.Lock()
		h.stats.TotalRejections++
		h.mu.Unlock()
		return fmt.Errorf("set human lock failed: %w", err)
	}

	reasonKey := HumanReasonKey + sessionID
	_ = h.cache.Set(ctx, reasonKey, reason, HumanLockReasonTTL)

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
//
// 返回指针避免 sync.Map 被值拷贝（go vet: copies lock value）
// 调用方在 RLock 期间读取快照，读取完成后应尽快释放（不要再持有指针跨锁）
func (h *HumanEscalationManager) GetStats(ctx context.Context) *EscalationStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return &h.stats
}


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

