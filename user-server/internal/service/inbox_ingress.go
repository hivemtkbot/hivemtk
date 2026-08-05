package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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

	// InboxContentDedupKey 内容 hash 去重 key 前缀
	// 2026-08-05 架构重构：Bridge 端不再做内容指纹去重，所有消息都上报到统一收信中心，
	// 由服务端在入库前用 内容 hash 判断是否重复（重复则丢弃）。
	// key: hivemtk:dedup:content:{channel}:{accountID}:{conversationID}:{contentHash}
	InboxContentDedupKey = "hivemtk:dedup:content:"
	// InboxContentDedupTTL 内容 hash 去重窗口（5 分钟）
	//   - 5 分钟内相同内容的客户消息视为重复（网络重发/扩展重连/兜底扫描重复上报）
	//   - 5 分钟后相同内容允许再次触发（用户可能真的再次发送相同消息）
	InboxContentDedupTTL = 5 * time.Minute

	// InboxReplyWindow 回复判断窗口（5 分钟）
	//   2026-08-05 架构重构（用户诉求）：
	//   - Bridge 不判断是否需要 AI 回复
	//   - 服务端判断：会话最后一条客户消息是否在 5 分钟以内
	//   - 5 分钟以内 + 未回复 → 触发 AI 回复
	//   - 5 分钟以外 → 仅落库不触发 AI（避免对历史存量消息逐一自动回复）
	InboxReplyWindow = 5 * time.Minute
)

// AITrigger 入站消息触发 AI 客服的抽象。
//
// 解耦 InboxIngressService 与具体 AI 编排实现，避免 service -> bridge 的导入环。
// 网页桥接场景由 WebhookService 实现（复用与 web 私信同源的同步主链路）；
// 单元测试可注入 fake 以验证“新消息触发 AI / 历史消息不触发 AI”的语义。
//
// senderName / isGroup / groupID / groupName 供群聊场景透传：
//   群聊中客户消息的 sender_id 被聚合为群 id，AI 编排需额外知道“这条消息是谁发的”
//   （senderName）以及会话是否为群（isGroup/groupID/groupName），否则群成员身份丢失、
//   AI 无法 @ 成员/按成员个性化回复。
type AITrigger interface {
	TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string, opts ...TriggerInboundOption)
}

// TriggerInboundOption 入站触发附加元数据（可选，向后兼容单参调用）。
// 群聊/多轮历史所需字段以函数式选项透传，避免破坏既有调用方签名。
type TriggerInboundOption func(*TriggerInboundMeta)

// TriggerInboundMeta 透传给 AI 编排的附加元数据
type TriggerInboundMeta struct {
	SenderName string
	IsGroup    bool
	GroupID    string
	GroupName  string
}

// WithSenderName 透传消息发送者昵称（群聊必填：区分群成员）
func WithSenderName(name string) TriggerInboundOption {
	return func(m *TriggerInboundMeta) { m.SenderName = name }
}

// WithGroup 透传群聊元数据
func WithGroup(groupID, groupName string) TriggerInboundOption {
	return func(m *TriggerInboundMeta) {
		m.IsGroup = true
		m.GroupID = groupID
		m.GroupName = groupName
	}
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
// 处理流程（2026-08-05 钩子机制重构）：
//  1. 标准化事件字段
//  2. 钩子1（sender_type 过滤）：self/agent → 直接丢弃，不入库不触发 AI
//  3. 检查人工接管锁（命中：仅落库，绕过 AI 路由）
//  4. 钩子2（去重）：内容 hash + 5min TTL SetNX，重复 → 直接丢弃
//  5. 持久化到 message_hub
//  6. 钩子3（回复判断）：查 message_hub 最后一条消息方向 + 5 分钟窗口
//     - 最后一条 inbound + 5min 内 → 触发 AI
//     - 最后一条 outbound（已回复） → 不触发
//     - 最后一条 inbound + 5min 外 → 历史消息不触发
func (s *InboxIngressService) HandleIngressMessage(ctx context.Context, event *model.MessageEvent) (*InboxIngressResult, error) {
	result := &InboxIngressResult{
		SessionID: event.SessionID,
	}
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return result, err
	}
	result.SessionID = event.SessionID

	// 钩子1：sender_type 过滤
	//   用户诉求："bridge 上行的消息里面携带了自己发送还是他人发送，
	//             进入统一消息中心之前利用钩子机制判断他人发的消息 + 消息未入库 满足则入库"
	//   - sender_type="self" / "agent" → 自己/坐席发的，直接丢弃，不入库不触发 AI
	//   - sender_type="system" → 系统消息，仅落库，不触发 AI
	//   - sender_type="customer" 或 "" → 客户消息，走完整链路
	senderType := event.SenderType
	if senderType == "" && event.Extra != nil {
		// 兼容老调用方（sender_type 在 Extra 里）
		if v, ok := event.Extra["sender_type"].(string); ok {
			senderType = v
		}
	}
	if senderType == "self" || senderType == "agent" {
		result.Accepted = false
		result.QueuedForAI = false
		result.Reason = "sender_type=self/agent; dropped (自己/坐席发的消息不入库)"
		logger.Ctx(ctx).Info().
			Str("channel", event.Channel).
			Str("conv_id", event.ConversationID).
			Str("event_id", event.EventID).
			Str("sender_type", senderType).
			Msg("[Inbox] 钩子1：sender_type=self/agent，丢弃不入库")
		return result, nil
	}
	isSystemMsg := senderType == "system"

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

	// 钩子2：内容 hash 去重（在落库前拦截）
	//   key: hivemtk:dedup:content:{channel}:{accountID}:{conversationID}:{contentHash}
	//   5 分钟内相同内容的客户消息视为重复 → 直接丢弃，不落库，不触发 AI
	//   （网络重发/扩展重连/兜底扫描重复上报会被这里拦截）
	//
	// 注意（用户诉求 "重复则丢弃，第一条入库"）：
	//   - 第 1 条消息 SetNX 成功 → 落库 + 触发 AI
	//   - 第 2..N 条相同内容 SetNX 失败 → 直接 return，不调用 persistMessage
	//   - 这样 message_hub 中每个会话的同一内容只保留 1 条记录
	contentHash := contentHashOf(event.Content)
	if s.cache != nil && contentHash != "" {
		dedupKey := fmt.Sprintf("%s%s:%s:%s:%s",
			InboxContentDedupKey,
			event.Channel,
			accountIDOf(event),
			event.ConversationID,
			contentHash,
		)
		ok, _ := s.cache.SetNX(context.Background(), dedupKey, "1", InboxContentDedupTTL)
		if !ok {
			// 5 分钟内已收到相同内容消息 → 直接丢弃，不落库
			if acquired {
				s.ReleaseAILock(ctx, event.SessionID)
			}
			result.QueuedForAI = false
			result.Reason = "duplicate content within 5min; dropped"
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Str("content_hash", contentHash).
				Msg("[Inbox] 钩子2：内容 hash 重复，丢弃不落库")
			return result, nil
		}
	}

	// 持久化到 message_hub（仅非重复内容才落库）
	if err := s.persistMessage(ctx, event); err != nil {
		if acquired {
			s.ReleaseAILock(ctx, event.SessionID)
		}
		return result, fmt.Errorf("持久化消息失败: %w", err)
	}

	// 系统消息：仅落库，不触发 AI（系统消息不需要 AI 回复）
	if isSystemMsg {
		if acquired {
			s.ReleaseAILock(ctx, event.SessionID)
		}
		result.QueuedForAI = false
		result.Reason = "sender_type=system; persisted only (系统消息不触发 AI)"
		logger.Ctx(ctx).Info().
			Str("channel", event.Channel).
			Str("conv_id", event.ConversationID).
			Str("event_id", event.EventID).
			Msg("[Inbox] 系统消息：仅落库不触发 AI")
		return result, nil
	}

	// 钩子3：回复判断（未回复 + 5 分钟以内 → 触发 AI）
	//   用户诉求："五分钟以内 + 未回复 满足则触发 aiagent 回复"
	//   查询 message_hub 中该会话最后一条消息方向：
	//     - 最后一条 inbound（客户发的） + 5min 内 → 未回复，触发 AI
	//     - 最后一条 outbound（已回复） → 不触发
	//     - 最后一条 inbound + 5min 外 → 历史消息不触发
	if s.hubRepo != nil {
		unreplied, withinWindow, err := s.hubRepo.HasUnrepliedCustomerMessage(ctx, event.ConversationID, InboxReplyWindow)
		if err != nil {
			// 查询失败：保守视为已回复（不触发 AI，避免误触发）
			logger.Ctx(ctx).Error().Err(err).
				Str("conv_id", event.ConversationID).
				Msg("[Inbox] 钩子3：查询未回复状态失败，保守不触发 AI")
			if acquired {
				s.ReleaseAILock(ctx, event.SessionID)
			}
			result.QueuedForAI = false
			result.Reason = "query unreplied status failed; not triggering AI"
			return result, nil
		}
		if !unreplied {
			// 已回复：不触发 AI
			if acquired {
				s.ReleaseAILock(ctx, event.SessionID)
			}
			result.QueuedForAI = false
			result.Reason = "session already replied; not triggering AI"
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Msg("[Inbox] 钩子3：会话已回复，不触发 AI")
			return result, nil
		}
		if !withinWindow {
			// 未回复但超过 5 分钟：历史消息不触发
			if acquired {
				s.ReleaseAILock(ctx, event.SessionID)
			}
			result.QueuedForAI = false
			result.Reason = "unreplied but outside 5min window; not triggering AI (历史消息)"
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Msg("[Inbox] 钩子3：未回复但超过 5 分钟，历史消息不触发 AI")
			return result, nil
		}
		// 未回复 + 5min 内 → 触发 AI
		result.QueuedForAI = true
		result.Reason = "unreplied within 5min; trigger AI"
		logger.Ctx(ctx).Info().
			Str("channel", event.Channel).
			Str("conv_id", event.ConversationID).
			Str("event_id", event.EventID).
			Msg("[Inbox] 钩子3：未回复 + 5min 内，触发 AI")
	} else {
		// 无 hubRepo（测试场景）：保持原行为（触发 AI），避免回归
		result.QueuedForAI = true
		result.Reason = "AI lock acquired; trigger AI customer service"
	}

	// 触发 AI 客服（异步推理，不阻塞 WS 回包）
	accountID := "default"
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			accountID = v
		}
	}
	if s.aiTrigger != nil {
		// 透传 sender_name / 群聊元数据：群聊中客户消息 sender_id 聚合为群 id，
		// AI 编排需知道“谁在群中发言”及会话群属性（见 AITrigger 接口注释）。
		opts := make([]TriggerInboundOption, 0, 2)
		if event.SenderName != "" {
			opts = append(opts, WithSenderName(event.SenderName))
		}
		if event.IsGroup {
			opts = append(opts, WithGroup(event.GroupID, groupNameOf(event)))
		}
		// 修复（2026-08-05）：加 panic recover 兜底 + 显式 start 日志。
		// 历史 bug（小红书 2268 现场）：5 条 inbound 入库 message_hub，0 个 customer_sessions，
		// 0 条 AI 回复——aiTrigger 链路某处 silent 失败（panic / 配置缺失 / goroutine 提前退出）。
		// 这里在第一道屏障加 recover + start 日志，便于通过 [Inbox] / [Webhook] 关键字
		// grep 整条链路是否真正开始/在哪一步断开。
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Ctx(ctx).Error().
						Interface("panic", r).
						Str("channel", event.Channel).
						Str("account_id", accountID).
						Str("conv_id", event.ConversationID).
						Str("event_id", event.EventID).
						Str("sender", event.SenderID).
						Msg("[Inbox] aiTrigger.TriggerInboundAI panic recovered — AI 链路已断开，请查 root cause")
				}
			}()
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("account_id", accountID).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Str("sender", event.SenderID).
				Int("content_len", len(event.Content)).
				Msg("[Inbox] aiTrigger.TriggerInboundAI start")
			s.aiTrigger.TriggerInboundAI(ctx, event.Channel, accountID, event.ConversationID, event.SenderID, event.Content, event.EventID, opts...)
		}()
	} else {
		// 升级 Warn → Error：缺 aiTrigger 是配置/装配 bug，不应仅 warn
		logger.Ctx(ctx).Error().
			Str("channel", event.Channel).
			Str("account_id", accountID).
			Str("session_id", event.SessionID).
			Msg("[Inbox] aiTrigger 未配置 — 桥接入站消息不会创建 customer_sessions / 不会生成 AI 回复。请检查 router.Setup() 中 bridgeIngressSvc.SetAITrigger(webhookSvc) 是否在 bridge WS 注册前调用。")
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
	// 会话级多轮历史落库（点3）：event.History 已由 toMessageEvent 透传，
	// 冗余进 message_hub.Extra 供统一收件箱展示/可观测（AI 上下文由 session_messages 重建）。
	if len(event.History) > 0 {
		if hub.Extra == nil {
			hub.Extra = model.JSONMap{}
		}
		hub.Extra["history"] = event.History
	}
	// 五层架构修复：原 s.db.WithContext(ctx).Create(hub).Error 违反
	// "service 不可直接访问 DB" 约束，已下沉到 MessageHubRepository.Create
	//
	// 修复（2026-08-05 审计 P1）：原 persistMessage 分两步——
	//   1) s.hubRepo.Create(ctx, hub)
	//   2) s.inboxSvc.UpsertFromHubMessage(ctx, hub)
	// 两步跨表无事务，inbox_conversations 写失败时仅 Warn 日志，导致"消息在 message_hub
	// 但收件箱看不到"的极端不一致。改为通过 inboxSvc.UpsertFromHubMessageTx 走 hubRepo.CreateWithInboxTx
	// 把两表写入包在同一 DB 事务内，任一失败整体回滚。
	//
	// 注意：仍用 context.Background() 而非入参 ctx——ctx 随 WS 连接生命周期取消，
	// 而消息落库 + 收件箱同步不应被连接抖动打断（与原设计保持一致）。
	if s.inboxSvc != nil && hub.Direction == "inbound" {
		// inbound 方向：走跨表事务版本（message_hub + inbox_conversations 原子写入）
		if _, err := s.inboxSvc.UpsertFromHubMessageTx(context.Background(), hub, s.hubRepo); err != nil {
			// 幂等：MsgID 唯一键冲突说明该消息已落库（扩展断线重发 / 历史重复回填），
			// 视为成功，避免日志刷错与 WS 上行被当作失败。与 webhook.go 对 join/left
			// 事件的 UNIQUE/duplicate 处理口径一致。
			if isDuplicateKey(err) {
				logger.Warnf("[Inbox] message_hub duplicate msg_id (idempotent skip): msg_id=%s session=%s",
					event.EventID, event.SessionID)
				return nil
			}
			// 其他错误：事务已回滚（message_hub 未落库），返回错误让上层处理
			return err
		}
		return nil
	}
	// 非 inbound 方向（如 outbound 历史回填由 persistHistoryMessage 处理）：
	// 仅落 message_hub，不写 inbox_conversations（保持原行为）
	if err := s.hubRepo.Create(ctx, hub); err != nil {
		if isDuplicateKey(err) {
			logger.Warnf("[Inbox] message_hub duplicate msg_id (idempotent skip): msg_id=%s session=%s",
				event.EventID, event.SessionID)
			return nil
		}
		return err
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
		// 幂等：MsgID 唯一键冲突说明该消息已落库（重扫 / 断线重发），视为成功，
		// 避免日志刷错与"历史回填失败"误报。与 persistMessage 口径一致。
		//
		// 修复（2026-08-05 审计 P1）：与 persistMessage 同步加 Warn 日志，
		// 便于审计 persistFailedOutbound 重投时 eventID 复用导致的重复频率。
		if isDuplicateKey(err) {
			logger.Warnf("[Inbox] message_hub duplicate msg_id (history idempotent skip): msg_id=%s session=%s",
				event.EventID, event.ConversationID)
			return nil
		}
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

// isDuplicateKey 判断是否为唯一键冲突（Postgres: duplicate key value on ...）。
// 用于消息落库幂等：同一 MsgID（event_id）重发/重扫时视为已落库，不报错。
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		errors.Is(err, gorm.ErrDuplicatedKey)
}

// contentHashOf 计算消息内容的 SHA-256 hash（用于 5 分钟内内容去重）。
// 返回前 16 字符的十六进制字符串（128 位足够区分重复内容，key 不会过长）。
//
// 2026-08-05 架构重构：Bridge 端不再做内容指纹去重，由服务端统一判断。
func contentHashOf(content string) string {
	if content == "" {
		return ""
	}
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}

// accountIDOf 从 MessageEvent 提取 account_id（用于内容 hash 去重 key）。
func accountIDOf(event *model.MessageEvent) string {
	if event.Extra == nil {
		return "default"
	}
	if v, ok := event.Extra["account_id"].(string); ok && v != "" {
		return v
	}
	return "default"
}

// groupNameOf 从 MessageEvent 提取群名：优先 GroupID 对应的 GroupName 字段（事件模型
// 无 GroupName 时回退 Extra 冗余），保证群聊 AI 编排能拿到群名。
func groupNameOf(event *model.MessageEvent) string {
	if event == nil {
		return ""
	}
	if event.Extra != nil {
		if v, ok := event.Extra["group_name"]; ok {
			if s, _ := v.(string); s != "" {
				return s
			}
		}
	}
	return ""
}
