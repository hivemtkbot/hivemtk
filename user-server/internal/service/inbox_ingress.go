package service

import (
	"context"

	"errors"

	"fmt"

	"sync"

	"time"

	"hivemtk-user/internal/cache"

	"hivemtk-user/internal/model"

	dbUtil "hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"

	"github.com/google/uuid"

	"gorm.io/gorm"
)

const (
	InboxHumanLockKey = "hivemtk:lock:human:"

	InboxAILockKey = "hivemtk:lock:ai_processing:"

	InboxPendingKey = "hivemtk:pending:"

	InboxLockTTL = 0

	InboxAILockTTL = 15 * time.Second

	InboxPendingTTL = 5 * time.Minute

	IngestLockKey = "hivemtk:lock:ingest:"

	IngestLockTTL = 25 * time.Second

	InboxContentDedupKey = "hivemtk:dedup:content:"

	InboxContentDedupTTL = 5 * time.Minute

	InboxSenderContentDedupKey = "hivemtk:dedup:sender-content:"

	InboxReplyWindow = 5 * time.Minute

	InboxBackfillFutureTolerance = 5 * time.Second

	InboxAIProcessingKey = "hivemtk:ai_processing:"

	InboxAIProcessingTTL = 2 * time.Minute

	RecheckDelayAfterRelease = 800 * time.Millisecond
)

type AITrigger interface {
	TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string, opts ...TriggerInboundOption)
}

type TriggerInboundOption func(*TriggerInboundMeta)

type TriggerInboundMeta struct {
	SenderName string
	IsGroup    bool
	GroupID    string
	GroupName  string
}

func WithSenderName(name string) TriggerInboundOption {
	return func(m *TriggerInboundMeta) { m.SenderName = name }
}

func WithGroup(groupID, groupName string) TriggerInboundOption {
	return func(m *TriggerInboundMeta) {
		m.IsGroup = true
		m.GroupID = groupID
		m.GroupName = groupName
	}
}

type InboxIngressResult struct {
	Accepted    bool   `json:"accepted"`      // 是否接受处理
	HumanLocked bool   `json:"human_locked"`  // 是否命中人工接管锁
	QueuedForAI bool   `json:"queued_for_ai"` // 是否已入队（拿到 AI 处理锁或加入待处理队列）
	SessionID   string `json:"session_id"`
	Reason      string `json:"reason,omitempty"` // 决策原因
}

type IngressDecision struct {
	Blocked    bool
	IsSelfEcho bool
	IsDup      bool
	Reason     string
}

type InboxIngressService struct {
	hubRepo   *repository.MessageHubRepository
	cache     cache.Cache
	mu        sync.Mutex
	triggerCh chan string   // 触发 AgentRuntime 处理通知（可选，保留兼容）
	aiTrigger AITrigger     // 入站消息触发 AI 客服的实现（桥接场景为 WebhookService）
	inboxSvc  *InboxService // 统一收件箱会话同步（桥接消息落库后同步到 inbox_conversations）
	// leadMiningSvc 线索发掘服务（非侵入异步）：消息落库成功后投递，绝不阻塞/入侵核心业务
	leadMiningSvc *Service
}

func NewInboxIngressService() *InboxIngressService {
	return NewInboxIngressServiceWithDB(dbUtil.GetDB(), nil)
}

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

func (s *InboxIngressService) TriggerChannel(ctx context.Context) <-chan string {
	return s.triggerCh
}

func (s *InboxIngressService) SetAITrigger(t AITrigger) {
	s.aiTrigger = t
}

func (s *InboxIngressService) SetInboxService(svc *InboxService) {
	s.inboxSvc = svc
}

func (s *InboxIngressService) SetLeadMining(svc *Service) {
	s.leadMiningSvc = svc
}

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

func (s *InboxIngressService) UnlockSessionForHuman(ctx context.Context, sessionID string) error {
	if s.cache == nil || sessionID == "" {
		return nil
	}
	_ = s.cache.Delete(ctx, InboxHumanLockKey+sessionID)
	_ = s.cache.Delete(ctx, InboxHumanLockKey+"reason:"+sessionID)
	return nil
}

func (s *InboxIngressService) tryAcquireAILock(ctx context.Context, sessionID string) (bool, error) {
	if s.cache == nil || sessionID == "" {
		return true, nil // 无缓存时降级为放行
	}
	key := InboxAILockKey + sessionID
	return s.cache.SetNX(ctx, key, "busy", InboxAILockTTL)
}

func (s *InboxIngressService) ReleaseAILock(ctx context.Context, sessionID string) {
	if s.cache == nil || sessionID == "" {
		return
	}
	_ = s.cache.Delete(ctx, InboxAILockKey+sessionID)
}

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

func (s *InboxIngressService) AppendPendingMessage(ctx context.Context, sessionID string, content string) error {
	if s.cache == nil || sessionID == "" {
		return nil
	}
	return s.cache.LPush(ctx, InboxPendingKey+sessionID, content, InboxPendingTTL)
}

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

func (s *InboxIngressService) HandleIngressMessage(ctx context.Context, event *model.MessageEvent) (*InboxIngressResult, error) {
	result := &InboxIngressResult{
		SessionID: event.SessionID,
	}
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return result, err
	}
	result.SessionID = event.SessionID

	// 钩子1（2026-08-06 重构：服务端权威自/他判定，不再信任前端 sender_type）
	//   ingest 入口（前端上报）的自/他标签不可信（小红书整行容器导致 isSelfMessage 失效）。
	//   改用"内容是否命中本会话平台下发(outbound)"判定：
	//     - 命中 → 平台自己的回显（SELF）→ 跳过入库与 AI（避免回环无限触发）
	//     - 未命中 → 强制视为用户消息（CUSTOMER），正常入库 + 触发 AI
	//   平台下发的消息（AI 生成 / 人工客服）必然先入库(direction=outbound)再下发，
	//   故内容命中既有 outbound 即可确定是平台自己的回显。
	isSystemMsg := event.SenderType == "system"

	// 卡位 1：检查人工接管锁
	humanLocked, _ := s.IsSessionHumanLocked(ctx, event.SessionID)
	if humanLocked {
		result.HumanLocked = true
		result.Accepted = true
		result.Reason = "session is human-locked; bypass AI routing"
		if err := s.persistMessage(ctx, event); err != nil {
			return result, fmt.Errorf("持久化消息失败: %w", err)
		}
		return result, nil
	}

	// 钩子2：入库判断（按 msg_id 查 DB 是否已存在，限本会话）
	//   用户诉求："消息上报保存是否入库依据是消息数据库是否存在"
	//   - 同会话已存在 → 幂等跳过
	//   - 跨会话同 msg_id（algo2 下同 channel+content）→ 不跳过，各自入库
	//   2026-08-05 优化：方向冲突检测
	//     bridge 上报老数据时，AI 回复已落库为 outbound，
	//     若 bridge 重复上报方向不同（inbound），以 DB 现有方向为准
	//   2026-08-07 第九轮修复：限本会话。algo2 下同 channel+content 的 msg_id 相同，
	//     跨会话命中会把其他客户发的相同内容（如 XHS 系统提示"已连续聊天3天"）误跳过。
	//     AI 回环防护由钩子2.5 第二道 GetByPlatformContent（限 outbound）兜底。
	if s.hubRepo != nil {
		existing, err := s.hubRepo.GetByMsgID(ctx, event.EventID)
		if err == nil && existing != nil && existing.ConversationID == event.ConversationID {
			result.Accepted = true
			result.QueuedForAI = false
			// 方向冲突检测：DB 已有方向 vs 本次推断方向
			incomingDir := "inbound"
			if event.SenderType == "self" || event.SenderType == "agent" {
				incomingDir = "outbound"
			}
			if existing.Direction != incomingDir && existing.Direction != "" {
				logger.Ctx(ctx).Warn().
					Str("event_id", event.EventID).
					Str("conv_id", event.ConversationID).
					Str("db_direction", existing.Direction).
					Str("incoming_direction", incomingDir).
					Msg("[Inbox] 钩子2：msg_id 已存在且方向冲突，以 DB 方向为准（幂等跳过）")
				result.Reason = "msg_id exists with different direction; DB direction preserved"
			} else {
				result.Reason = "msg_id already exists in DB; idempotent skip"
			}
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Msg("[Inbox] 钩子2：msg_id 已存在，幂等跳过")
			return result, nil
		}
		if err == nil && existing != nil && existing.ConversationID != event.ConversationID {
			logger.Ctx(ctx).Info().
				Str("event_id", event.EventID).
				Str("conv_id", event.ConversationID).
				Str("existing_conv_id", existing.ConversationID).
				Msg("[Inbox] 钩子2：msg_id 跨会话命中（algo2 同 channel+content），不跳过，各自入库")
		}
	}

	// 统一收件中间件（2026-08-09）：渠道+发送者+内容 服务端权威去重 / 自他判定。
	//   取代旧「仅按 platform+content 匹配」的回环去重——旧逻辑会把"客户复述了 AI 的原话"
	//   误判为回显而丢失客户消息。新逻辑把发送者纳入去重键（ContentHashWithSender），
	//   并依赖 DB 回查 message_hub 出站(outbound)行判定"自己消息"，实现：
	//     - 自己消息回显（AI/人工回复经渠道回写）→ 拦截，防 AI 二次触发死循环（回环）；
	//     - 同(渠道,发送者,内容)短时重复投递 → 拦截，防重复落库穿透业务层。
	//   自/他判定不信任前端不可靠的 sender_type，由服务端 DB 哈希回查权威判定。
	if decision, derr := s.interceptInbound(ctx, event); derr != nil {
		logger.Ctx(ctx).Warn().Err(derr).Str("event_id", event.EventID).Msg("[Inbox] interceptInbound 出错，放行（不阻断业务）")
	} else if decision != nil && decision.Blocked {
		result.Accepted = true
		result.QueuedForAI = false
		result.Reason = fmt.Sprintf("intercepted by middleware: %s (self_echo=%v dup=%v)", decision.Reason, decision.IsSelfEcho, decision.IsDup)
		logger.Ctx(ctx).Info().
			Str("event_id", event.EventID).
			Bool("self_echo", decision.IsSelfEcho).
			Bool("dup", decision.IsDup).
			Str("reason", decision.Reason).
			Msg("[Inbox] 中间件拦截：消息被去重/回环拦截，不穿透业务层")
		return result, nil
	}

	// 持久化到 message_hub（含时序锚点判断，见 persistMessage）
	if err := s.persistMessage(ctx, event); err != nil {
		return result, fmt.Errorf("持久化消息失败: %w", err)
	}
	result.Accepted = true

	// 系统消息：仅落库，不触发 AI
	if isSystemMsg {
		result.QueuedForAI = false
		result.Reason = "sender_type=system; persisted only (系统消息不触发 AI)"
		logger.Ctx(ctx).Info().
			Str("channel", event.Channel).
			Str("conv_id", event.ConversationID).
			Str("event_id", event.EventID).
			Msg("[Inbox] 系统消息：仅落库不触发 AI")
		return result, nil
	}

	// 钩子3：回复判断（查会话最后一条消息方向）
	//   用户诉求："是否回消息依据是最后一条是不是平台自己发的 是则不发送"
	//   - 最后一条 outbound（平台自己发的）→ 不回复
	//   - 最后一条 inbound（客户发的）+ 5min 内 → 回复
	//   - 最后一条 inbound + 5min 外 → 历史消息不回复
	if s.hubRepo != nil {
		unreplied, withinWindow, err := s.hubRepo.HasUnrepliedCustomerMessage(ctx, event.ConversationID, InboxReplyWindow)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str("conv_id", event.ConversationID).
				Msg("[Inbox] 钩子3：查询最后一条消息方向失败，保守不触发 AI")
			result.QueuedForAI = false
			result.Reason = "query last message direction failed; not triggering AI"
			return result, nil
		}
		if !unreplied {
			result.QueuedForAI = false
			result.Reason = "last message is outbound (平台自己发的); not triggering AI"
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Msg("[Inbox] 钩子3：最后一条是平台自己发的，不触发 AI")
			return result, nil
		}
		if !withinWindow {
			result.QueuedForAI = false
			result.Reason = "last inbound outside 5min window; not triggering AI (历史消息)"
			logger.Ctx(ctx).Info().
				Str("channel", event.Channel).
				Str("conv_id", event.ConversationID).
				Str("event_id", event.EventID).
				Msg("[Inbox] 钩子3：最后一条 inbound 超过 5 分钟，历史消息不触发 AI")
			return result, nil
		}
	}

	// 触发 AI（单条调用，立即释放 AI 锁）
	//   注意：批量上报请用 HandleIngressBatch（按 conversation 分组 + batch 内合并 AI 回复）
	s.triggerAIForEvent(ctx, event)
	result.QueuedForAI = true
	result.Reason = "trigger AI customer service"
	return result, nil
}

func (s *InboxIngressService) triggerAIForEvent(ctx context.Context, event *model.MessageEvent) {
	accountID := "default"
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			accountID = v
		}
	}
	if s.aiTrigger == nil {
		logger.Ctx(ctx).Error().
			Str("channel", event.Channel).
			Str("account_id", accountID).
			Str("session_id", event.SessionID).
			Msg("[Inbox] aiTrigger 未配置 — 桥接入站消息不会创建 customer_sessions / 不会生成 AI 回复。请检查 router.Setup() 中 bridgeIngressSvc.SetAITrigger(webhookSvc) 是否在 bridge WS 注册前调用。")
		return
	}
	opts := make([]TriggerInboundOption, 0, 2)
	if event.SenderName != "" {
		opts = append(opts, WithSenderName(event.SenderName))
	}
	if event.IsGroup {
		opts = append(opts, WithGroup(event.GroupID, groupNameOf(event)))
	}
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
		// 2026-08-06 原子分布式排他：仅当无人正在触发时才设置标记并触发，
		// 杜绝并发/重复上报导致"连续回复两条"
		//   前端不断上报时多个 batch 并发进入，旧逻辑 Set+Exists 非原子会双触发；
		//   改用 SetNX：仅首个成功设置者触发 AI，其余（并发/重报）命中已存在标记直接跳过。
		//   AI 完成后由 webhook.go sendOutbound 主动删除标记。
		if s.cache != nil && event.ConversationID != "" {
			aiKey := InboxAIProcessingKey + event.ConversationID
			acquired, lerr := s.cache.SetNX(ctx, aiKey, "1", InboxAIProcessingTTL)
			if lerr != nil {
				logger.Ctx(ctx).Warn().Err(lerr).
					Str("conv_id", event.ConversationID).
					Msg("[Inbox] 设置 AI 处理中标记失败（放行本次触发，但可能重复触发）")
			} else if !acquired {
				logger.Ctx(ctx).Info().
					Str("conv_id", event.ConversationID).
					Msg("[Inbox] AI 已在进行中（分布式排他命中），跳过本次触发避免重复回复")
				return
			}
		}
		s.aiTrigger.TriggerInboundAI(ctx, event.Channel, accountID, event.ConversationID, event.SenderID, event.Content, event.EventID, opts...)
	}()
}

func (s *InboxIngressService) ReleaseAIProcessingFlag(ctx context.Context, conversationID string) {
	if s == nil || s.cache == nil || conversationID == "" {
		return
	}
	aiKey := InboxAIProcessingKey + conversationID
	if err := s.cache.Delete(ctx, aiKey); err != nil {
		logger.Ctx(ctx).Warn().Err(err).
			Str("conv_id", conversationID).
			Msg("[Inbox] 释放 AI 处理中标记失败")
	}
}

func (s *InboxIngressService) withIngestLock(ctx context.Context, conversationID string, fn func() error) (bool, error) {
	if s.cache == nil || conversationID == "" {
		return true, fn()
	}
	key := IngestLockKey + conversationID
	token := uuid.NewString()
	const retries = 4
	for i := 0; i < retries; i++ {
		ok, err := s.cache.SetNX(ctx, key, token, IngestLockTTL)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).
				Str("conv_id", conversationID).
				Msg("[Ingest] 锁后端异常，退化为直接执行（DB 唯一约束兜底去重）")
			return true, fn()
		}
		if ok {
			defer s.cache.ReleaseLock(ctx, key, token)
			if ferr := fn(); ferr != nil {
				return true, ferr
			}
			return true, nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	logger.Ctx(ctx).Warn().
		Str("conv_id", conversationID).
		Msg("[Ingest] 入库锁被占用（重试耗尽），跳过本次（前端将重新上报，DB 唯一约束保证幂等）")
	return false, nil
}

func (s *InboxIngressService) RecheckUnrepliedAndTrigger(ctx context.Context, conversationID, sessionID string) {
	if s == nil || conversationID == "" {
		return
	}
	// 延迟让 AI 推理期间到达的消息完成入库
	select {
	case <-time.After(RecheckDelayAfterRelease):
	case <-ctx.Done():
		return
	}

	// 1. 检查人工接管锁（AI 回复后用户可能转人工）
	if sessionID != "" {
		if humanLocked, _ := s.IsSessionHumanLocked(ctx, sessionID); humanLocked {
			logger.Ctx(ctx).Info().
				Str("conv_id", conversationID).
				Str("session_id", sessionID).
				Msg("[Inbox][Recheck] 会话已被人工接管，跳过补触发")
			return
		}
	}

	// 2. 查 DB 最后一条消息方向
	if s.hubRepo == nil {
		return
	}
	unreplied, withinWindow, err := s.hubRepo.HasUnrepliedCustomerMessage(ctx, conversationID, InboxReplyWindow)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).
			Str("conv_id", conversationID).
			Msg("[Inbox][Recheck] 查询未回复消息失败，跳过补触发")
		return
	}
	if !unreplied {
		// 最后一条是 outbound（AI/人工已回复）→ 无遗漏消息
		return
	}
	if !withinWindow {
		// 最后一条 inbound 超 5min → 历史消息不补触发
		return
	}

	// 3. 再次检查 ai_processing 标记（防止与新一轮触发竞态）
	if s.cache != nil {
		aiKey := InboxAIProcessingKey + conversationID
		if exists, _ := s.cache.Exists(ctx, aiKey); exists {
			logger.Ctx(ctx).Info().
				Str("conv_id", conversationID).
				Msg("[Inbox][Recheck] 新一轮 AI 已在处理中，跳过补触发")
			return
		}
	}

	// 4. 获取最后一条客户消息内容，构造触发事件
	last, err := s.hubRepo.GetLastInboundByConversation(ctx, conversationID)
	if err != nil || last == nil {
		logger.Ctx(ctx).Warn().Err(err).
			Str("conv_id", conversationID).
			Msg("[Inbox][Recheck] 获取最后一条客户消息失败，跳过补触发")
		return
	}

	// 5. 构造 MessageEvent 并触发 AI
	ev := &model.MessageEvent{
		EventID:        uuid.NewString(),
		Channel:        last.Platform,
		ConversationID: conversationID,
		SessionID:      sessionID,
		SenderID:       last.SenderID,
		SenderName:     last.SenderName,
		Content:        last.Content,
		MsgType:        last.MsgType,
		IsGroup:        last.IsGroup,
		GroupID:        last.GroupID,
		Timestamp:      last.SentAt,
		Extra:          map[string]any{"account_id": last.AccountID},
	}
	logger.Ctx(ctx).Info().
		Str("conv_id", conversationID).
		Str("event_id", ev.EventID).
		Str("sender", last.SenderID).
		Msg("[Inbox][Recheck] 检测到 AI 推理期间遗漏的未回复客户消息，补触发 AI")
	s.triggerAIForEvent(ctx, ev)
}

type InboxIngressBatchResult struct {
	PerEvent    []*InboxIngressResult `json:"per_event"`    // 每条消息处理结果（与入参 events 索引对齐）
	TriggeredAI bool                  `json:"triggered_ai"` // 是否触发了 AI（batch 内合并触发）
	Reason      string                `json:"reason,omitempty"`
}
