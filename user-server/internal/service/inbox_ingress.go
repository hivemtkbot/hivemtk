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

// AITrigger 入站消息触发 AI 客服的抽象。
//
// 解耦 InboxIngressService 与具体 AI 编排实现，避免 service -> bridge 的导入环。
// 网页桥接场景由 WebhookService 实现（复用与 web 私信同源的同步主链路）；
// 单元测试可注入 fake 以验证“新消息触发 AI / 历史消息不触发 AI”的语义。
type AITrigger interface {
	TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string)
}

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
	triggerCh chan string // 触发 AgentRuntime 处理通知（可选，保留兼容）
	aiTrigger AITrigger   // 入站消息触发 AI 客服的实现（桥接场景为 WebhookService）
	inboxSvc  *InboxService // 统一收件箱会话同步（桥接消息落库后同步到 inbox_conversations）
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

// SetAITrigger 注入 AI 触发实现（生产环境由 WebhookService 提供，测试可注入 fake）
func (s *InboxIngressService) SetAITrigger(t AITrigger) {
	s.aiTrigger = t
}

// SetInboxService 注入统一收件箱服务，使桥接消息落库 message_hub 后
// 同步会话到 inbox_conversations（统一收件箱 list 数据源）。
// 未注入时跳过同步（降级为仅 message_hub 落库），不影响主链路。
func (s *InboxIngressService) SetInboxService(svc *InboxService) {
	s.inboxSvc = svc
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
		// 桥接场景：列表视图未进入具体会话时 conversation_id 为空，
		// 客户消息 sender_id 回落为 conversation_id 会变空。改用 sender_name 兜底，
		// 避免整条消息被丢弃（其他渠道恒带 sender_id，不受影响）。
		if event.SenderName != "" {
			event.SenderID = event.SenderName
		} else {
			event.SenderID = event.Channel + ":unknown"
		}
	}
	// ConversationID 兜底：抖音等桥接渠道在列表页/浮层/实时私信下常取不到
	// 活动会话 ID（扩展侧 getConversationId() 返回 null，且 parseMessageItem 不携带），
	// 导致 message_hub.conversation_id 全为 NULL，UI 按会话聚合查不到消息。
	// 逐级兜底：ConversationID → SessionID → Channel:account_id，保证每条消息可聚合。
	if event.ConversationID == "" {
		if event.SessionID != "" {
			event.ConversationID = event.SessionID
		} else {
			accountID := ""
			if event.Extra != nil {
				if v, ok := event.Extra["account_id"].(string); ok {
					accountID = v
				}
			}
			if accountID == "" {
				accountID = event.Channel + ":unknown"
			}
			event.ConversationID = event.Channel + ":" + accountID
		}
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

	// 卡位 2：AI 处理串行化锁（轻量守卫，避免同一消息并发重复触发）
	acquired, _ := s.tryAcquireAILock(ctx, event.SessionID)
	result.Accepted = true
	result.QueuedForAI = true
	result.Reason = "AI lock acquired; trigger AI customer service"

	// 持久化到 message_hub（无论是否走 AI，实时消息都必须落库）
	if err := s.persistMessage(ctx, event); err != nil {
		if acquired {
			s.ReleaseAILock(ctx, event.SessionID)
		}
		return result, fmt.Errorf("持久化消息失败: %w", err)
	}

	// 触发 AI 客服（异步推理，不阻塞 WS 回包）
	accountID := "default"
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			accountID = v
		}
	}
	if s.aiTrigger != nil {
		s.aiTrigger.TriggerInboundAI(ctx, event.Channel, accountID, event.ConversationID, event.SenderID, event.Content, event.EventID)
	} else {
		logger.Warnf("[Inbox] aiTrigger 未配置，入站消息未触发 AI session=%s", event.SessionID)
	}

	// 立即释放串行锁：AI 生成由 runAIGeneration 的 replySem 并发控制，
	// 会话上下文由编排器维护；此处释放可避免后续多轮消息卡在 pending 永不消费（旧死锁问题）。
	if acquired {
		s.ReleaseAILock(ctx, event.SessionID)
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
	if err := s.hubRepo.Create(ctx, hub); err != nil {
		return err
	}
	// 同步会话到统一收件箱（inbox_conversations），否则 unifiedInbox/list 看不到桥接聊天。
	// 仅同步 inbound 方向（outbound 历史回填由 persistHistoryMessage 处理）。
	if s.inboxSvc != nil && hub.Direction == "inbound" {
		// 注意：用 context.Background() 而非 ctx——ctx 随 WS 连接生命周期取消，
		// 而 AI 推理耗时较长，连接抖动会致 UpsertFromHubMessage 报 context canceled，
		// 使会话同步失败（消息已落库 message_hub 却未在收件箱出现）。收件箱同步是
		// 独立副作用，不应受连接取消影响。
		if _, err := s.inboxSvc.UpsertFromHubMessage(context.Background(), hub); err != nil {
			logger.Warnf("[Inbox] 桥接消息同步统一收件箱失败(session=%s): %v", event.SessionID, err)
		}
	}
	return nil
}

// PersistBridgeHistory 仅持久化历史/回填消息，不触发 AI 路由。
//
// 用途（需求⑤ 多用户历史 / 需求③ outbound 落库）：
//   - 页面加载时回填的存量私信（客户侧 inbound / 自己侧 outbound）
//   - 本扩展回写到网页的 AI 回复（outbound，标记为 AI 回复）
//
// 与 HandleIngressMessage 的关键区别：不获取 AI 锁、不投递 pending、不通知 AgentRuntime，
// 从而避免「回填空历史误触发 AI」与「自己回复被再次推理造成自回环」。
func (s *InboxIngressService) PersistBridgeHistory(ctx context.Context, event *model.MessageEvent, direction string) error {
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return err
	}
	if direction == "" {
		direction = "inbound"
	}
	return s.persistHistoryMessage(ctx, event, direction)
}

// ListFailedOutbound 查询某账号在某桥接渠道下"出站且失败"的消息（离线降级落库，待补发）。
// 供桥接扩展重连时自动重投（P1-7 修复：离线消息不再永久 failed）。
func (s *InboxIngressService) ListFailedOutbound(ctx context.Context, channel, accountID string) ([]*model.MessageHub, error) {
	if s.hubRepo == nil {
		return nil, nil
	}
	list, _, err := s.hubRepo.ListByHubQuery(ctx, repository.HubListQuery{
		Platform:  channel,
		AccountID: accountID,
		Direction: "outbound",
		Status:    "failed",
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// MarkOutboundDelivered 将离线补发成功的出站消息标记为已送达，
// 避免重复补发与坐席 UI 长期显示 failed。
func (s *InboxIngressService) MarkOutboundDelivered(ctx context.Context, hub *model.MessageHub) error {
	if s.hubRepo == nil || hub == nil {
		return nil
	}
	hub.Status = "delivered"
	return s.hubRepo.Update(ctx, hub)
}

// persistHistoryMessage 持久化消息，Direction 由调用方显式传入（区别于 persistMessage 硬编码 inbound）。
func (s *InboxIngressService) persistHistoryMessage(ctx context.Context, event *model.MessageEvent, direction string) error {
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
		Direction:      direction,
		MsgType:        event.MsgType,
		SenderID:       event.SenderID,
		SenderName:     event.SenderName,
		ReceiverID:     event.ReceiverID,
		Content:        event.Content,
		MediaURL:       event.MediaURL,
		ConversationID: event.ConversationID,
		IsGroup:        event.IsGroup,
		GroupID:        event.GroupID,
		// outbound 方向视为 AI/坐席发出的回复
		IsAIReply: direction == "outbound",
		AIAgent:   event.AIAgent,
		IsRead:    direction == "outbound",
		SentAt:    event.Timestamp,
	}
	if event.Extra != nil {
		extra := model.JSONMap{}
		for k, v := range event.Extra {
			extra[k] = v
		}
		hub.Extra = extra
		// 桥接离线失败消息在 Extra 中携带 status=failed，落到独立可查询列便于重连补发（P1-7）
		if v, ok := event.Extra["status"].(string); ok && v != "" {
			hub.Status = v
		}
	}
	if err := s.hubRepo.Create(ctx, hub); err != nil {
		return err
	}
	// 同步会话到统一收件箱（inbox_conversations），使 unifiedInbox/list 能看到桥接聊天。
	// inbound 计入未读；outbound 不计入（与飞书/企微一致）。
	if s.inboxSvc != nil {
		// 用 context.Background() 而非 ctx：避免随 WS 连接取消导致同步失败（见 persistMessage 注释）。
		if _, err := s.inboxSvc.UpsertFromHubMessage(context.Background(), hub); err != nil {
			logger.Warnf("[Inbox] 桥接历史消息同步统一收件箱失败(conv=%s): %v", event.ConversationID, err)
		}
	}
	return nil
}
