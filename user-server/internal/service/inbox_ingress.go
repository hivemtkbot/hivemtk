package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"marketing/internal/cache"
	"marketing/internal/model"
	dbUtil "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 渠道接入消息中台 - Redis 锁与待处理队列 Key 前缀
const (
	// InboxHumanLockKey 会话被人工接管时永久锁定（AI 路由绕过）
	// key: hivemtk:lock:human:{sessionID}
	InboxHumanLockKey = "hivemtk:lock:human:"
	// InboxAILockKey AI 处理中的串行化锁（15s TTL）
	// key: hivemtk:lock:ai_processing:{sessionID}
	InboxAILockKey = "hivemtk:lock:ai_processing:"
	// InboxPendingKey 待处理消息队列（AI 推理期间用户继续发的消息）
	// key: hivemtk:pending:{sessionID}
	InboxPendingKey = "hivemtk:pending:"
	// InboxLockTTL 人类接管锁过期（极长，等同永久；实际由转人工门禁显式释放）
	InboxLockTTL = 0
	// InboxAIProcessingTTL AI 串行化锁默认 15s
	InboxAIProcessingTTL = 15 * time.Second
	// InboxPendingTTL 待处理消息队列 TTL
	InboxPendingTTL = 5 * time.Minute
)

// InboxIngressResult 消息入站处理结果
type InboxIngressResult struct {
	Accepted    bool   `json:"accepted"`      // 是否接受处理
	HumanLocked bool   `json:"human_locked"`  // 是否命中人工接管锁
	QueuedForAI bool   `json:"queued_for_ai"` // 是否已入队（拿到 AI 处理锁或加入待处理队列）
	SessionID   string `json:"session_id"`
	Reason      string `json:"reason,omitempty"` // 决策原因
}

// InboxIngressService 渠道接入消息中台服务
//
// 核心职责：
//  1. 标准化外部渠道事件 (MessageEvent) -> 内部消息
//  2. 命中人工接管锁时直接落库（绕过 AI 路由）
//  3. 未命中时通过 Redis SetNX 串行化 AI 处理（防抖 + 防止并发重复推理）
//  4. AI 推理期间用户继续发的消息进入 pending 队列，等待下一轮合并处理
type InboxIngressService struct {
	hubRepo   *repository.MessageHubRepository
	cache     cache.Cache
	mu        sync.Mutex
	triggerCh chan string // 触发 AgentRuntime 处理通知（可选）
}

// NewInboxIngressService 构造入站服务(无参,内部用 dbUtil.GetDB())
func NewInboxIngressService() *InboxIngressService {
	return NewInboxIngressServiceWithDB(dbUtil.GetDB(), nil)
}

// NewInboxIngressServiceWithDB 构造带 DB 的入站服务(显式注入 db,兼容旧调用)
//
// 五层架构修复（v1.1）：service 层不再持有 *gorm.DB，
// 内部用 db 构造 MessageHubRepository；db 为 nil 时 repo 也为 nil（Create 等方法做无操作短路）
func NewInboxIngressServiceWithDB(db *gorm.DB, c cache.Cache) *InboxIngressService {
	if c == nil {
		c = cache.GetGlobalCache()
	}
	var hubRepo *repository.MessageHubRepository
	if db != nil {
		hubRepo = repository.NewMessageHubRepositoryWithDB(db)
	}
	return &InboxIngressService{
		hubRepo:   hubRepo,
		cache:     c,
		triggerCh: make(chan string, 1024),
	}
}

// TriggerChannel 返回 AgentRuntime 监听通道（非阻塞消费）
func (s *InboxIngressService) TriggerChannel(ctx context.Context) <-chan string {
	return s.triggerCh
}

// IsSessionHumanLocked 检查会话是否被人工接管
func (s *InboxIngressService) IsSessionHumanLocked(ctx context.Context, sessionID string) (bool, error) {
	if s.cache == nil || sessionID == "" {
		return false, nil
	}
	key := InboxHumanLockKey + sessionID
	v, err := s.cache.Get(ctx, key)
	if err != nil {
		return false, nil // 缓存降级：返回未锁定（保守路由到 AI）
	}
	return v == "true", nil
}

// LockSessionForHuman 永久锁定会话为人工接管
func (s *InboxIngressService) LockSessionForHuman(ctx context.Context, sessionID, reason string) error {
	if s.cache == nil || sessionID == "" {
		return errors.New("cache unavailable")
	}
	key := InboxHumanLockKey + sessionID
	if err := s.cache.Set(ctx, key, "true", InboxLockTTL); err != nil {
		return err
	}
	if reason != "" {
		_ = s.cache.Set(ctx, InboxHumanLockKey+"reason:"+sessionID, reason, 24*time.Hour)
	}
	logger.Infof("[Inbox] 会话 %s 已被人工接管: %s", sessionID, reason)
	return nil
}

// UnlockSessionForHuman 解除人工接管锁（人工主动释放）
func (s *InboxIngressService) UnlockSessionForHuman(ctx context.Context, sessionID string) error {
	if s.cache == nil || sessionID == "" {
		return nil
	}
	_ = s.cache.Delete(ctx, InboxHumanLockKey+sessionID)
	_ = s.cache.Delete(ctx, InboxHumanLockKey+"reason:"+sessionID)
	return nil
}

// tryAcquireAILock 尝试获取 AI 处理串行化锁；返回 true 表示拿到锁
func (s *InboxIngressService) tryAcquireAILock(ctx context.Context, sessionID string) (bool, error) {
	if s.cache == nil || sessionID == "" {
		return true, nil // 无缓存时降级为放行
	}
	key := InboxAILockKey + sessionID
	return s.cache.SetNX(ctx, key, "busy", InboxAIProcessingTTL)
}

// ReleaseAILock 释放 AI 处理锁
func (s *InboxIngressService) ReleaseAILock(ctx context.Context, sessionID string) {
	if s.cache == nil || sessionID == "" {
		return
	}
	_ = s.cache.Delete(ctx, InboxAILockKey+sessionID)
}

// IsSessionAIBusy 检查会话当前是否在 AI 推理中
func (s *InboxIngressService) IsSessionAIBusy(ctx context.Context, sessionID string) (bool, error) {
	if s.cache == nil || sessionID == "" {
		return false, nil
	}
	v, err := s.cache.Get(ctx, InboxAILockKey+sessionID)
	if err != nil {
		return false, nil
	}
	return v == "busy", nil
}

// AppendPendingMessage 把消息推入待处理队列（AI 推理期间追加）
func (s *InboxIngressService) AppendPendingMessage(ctx context.Context, sessionID string, content string) error {
	if s.cache == nil || sessionID == "" {
		return nil
	}
	return s.cache.LPush(ctx, InboxPendingKey+sessionID, content, InboxPendingTTL)
}

// PopPendingMessages 弹出并清空待处理队列
func (s *InboxIngressService) PopPendingMessages(ctx context.Context, sessionID string) ([]string, error) {
	if s.cache == nil || sessionID == "" {
		return nil, nil
	}
	key := InboxPendingKey + sessionID
	items, err := s.cache.LRange(ctx, key, 0, -1)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Delete(ctx, key)
	return items, nil
}

// NormalizeEvent 标准化外部 MessageEvent 字段（缺失字段补齐）
func (s *InboxIngressService) NormalizeEvent(ctx context.Context, event *model.MessageEvent) error {
	if event == nil {
		return errors.New("event is nil")
	}
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.SessionID == "" {
		// 没有 sessionID 时回退到 senderID 构造
		event.SessionID = event.Channel + ":" + event.SenderID
	}
	if event.Channel == "" {
		return fmt.Errorf("invalid channel (empty)")
	}
	if event.SenderID == "" {
		return fmt.Errorf("invalid sender_id (empty)")
	}
	if event.MsgType == "" {
		event.MsgType = model.MsgTypeText
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return nil
}

// HandleIngressMessage 渠道消息统一入口
//
// 处理流程：
//  1. 标准化事件字段
//  2. 检查人工接管锁（命中：仅落库，绕过 AI 路由）
//  3. 尝试获取 AI 处理锁（拿到：触发 AgentRuntime）
//  4. 未拿到锁：将消息追加到 pending 队列（等下一轮合并）
//  5. 持久化到 message_hub（无论是否走 AI）
func (s *InboxIngressService) HandleIngressMessage(ctx context.Context, event *model.MessageEvent) (*InboxIngressResult, error) {
	result := &InboxIngressResult{
		SessionID: event.SessionID,
	}
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return result, err
	}
	result.SessionID = event.SessionID

	// 卡位 1：检查人工接管锁
	humanLocked, _ := s.IsSessionHumanLocked(ctx, event.SessionID)
	if humanLocked {
		result.HumanLocked = true
		result.Accepted = true
		result.Reason = "session is human-locked; bypass AI routing"
		// 落库供坐席查看
		if err := s.persistMessage(ctx, event); err != nil {
			return result, fmt.Errorf("持久化消息失败: %w", err)
		}
		return result, nil
	}

	// 卡位 2：AI 处理串行化锁
	acquired, _ := s.tryAcquireAILock(ctx, event.SessionID)
	if !acquired {
		// AI 正在推理，把消息推入 pending 队列
		if err := s.AppendPendingMessage(ctx, event.SessionID, event.Content); err != nil {
			logger.Warnf("[Inbox] 追加待处理消息失败 session=%s: %v", event.SessionID, err)
		}
		result.Accepted = true
		result.QueuedForAI = true
		result.Reason = "AI is processing; message appended to pending queue"
		if err := s.persistMessage(ctx, event); err != nil {
			return result, fmt.Errorf("持久化消息失败: %w", err)
		}
		return result, nil
	}

	// 成功获取 AI 处理锁
	result.Accepted = true
	result.QueuedForAI = true
	result.Reason = "AI lock acquired; trigger AgentRuntime"
	if err := s.persistMessage(ctx, event); err != nil {
		// 失败时释放锁，避免死锁
		s.ReleaseAILock(ctx, event.SessionID)
		return result, fmt.Errorf("持久化消息失败: %w", err)
	}

	// 通知 AgentRuntime（异步非阻塞）
	select {
	case s.triggerCh <- event.SessionID:
	default:
		// channel 已满时不阻塞
	}
	return result, nil
}

// persistMessage 持久化消息到 message_hub 表
func (s *InboxIngressService) persistMessage(ctx context.Context, event *model.MessageEvent) error {
	if s.hubRepo == nil {
		return nil
	}
	accountID := "default"
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			accountID = v
		}
	}
	hub := &model.MessageHub{
		MsgID:          event.EventID,
		Platform:       event.Channel,
		AccountID:      accountID,
		Direction:      "inbound",
		MsgType:        event.MsgType,
		SenderID:       event.SenderID,
		SenderName:     event.SenderName,
		ReceiverID:     event.ReceiverID,
		Content:        event.Content,
		MediaURL:       event.MediaURL,
		ConversationID: event.ConversationID,
		IsGroup:        event.IsGroup,
		GroupID:        event.GroupID,
		IsAIReply:      event.IsAIReply,
		AIAgent:        event.AIAgent,
		IsRead:         false,
		SentAt:         event.Timestamp,
		Extra:          nil,
	}
	if event.Extra != nil {
		extra := model.JSONMap{}
		for k, v := range event.Extra {
			extra[k] = v
		}
		hub.Extra = extra
	}
	// 五层架构修复：原 s.db.WithContext(ctx).Create(hub).Error 违反
	// "service 不可直接访问 DB" 约束，已下沉到 MessageHubRepository.Create
	return s.hubRepo.Create(ctx, hub)
}
