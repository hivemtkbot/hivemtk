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
	"marketing/internal/pkg/tracing"
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
	// InboxAILockTTL AI 串行化锁默认 15s（防并发重复触发，短锁）
	//   2026-08-05 重构（用户科学方案）：
	//   - AI 合并改在 batch 层处理（HandleIngressBatch），不再用 AI 锁+pending 复杂机制
	//   - AI 锁仅作短时并发守卫，防止同一会话多个 batch 并发触发 AI
	//   - 触发 AI 后立即释放锁（靠 DB 最后一条消息方向判断避免重复回复）
	InboxAILockTTL = 15 * time.Second
	// InboxPendingTTL 待处理消息队列 TTL（保留兼容，新方案不再使用 pending）
	InboxPendingTTL = 5 * time.Minute

	// IngestLockKey 同会话入库分布式排他锁（2026-08-06 新增）
	// key: hivemtk:lock:ingest:{conversationID}
	//   前端不断上报 / 多实例并发时，保证同一会话的消息入库串行，
	//   杜绝并发 TOCTOU 与重复入库（"处理完毕才放开"；分布式排它锁）。
	IngestLockKey = "hivemtk:lock:ingest:"
	// IngestLockTTL 入库锁 TTL（25s 兜底，正常处理远小于此；防进程崩溃遗留死锁）
	IngestLockTTL = 25 * time.Second

	// InboxContentDedupKey 内容 hash 去重 key 前缀（保留兼容，新方案改用 DB msg_id 去重）
	InboxContentDedupKey = "hivemtk:dedup:content:"
	InboxContentDedupTTL = 5 * time.Minute

	// InboxReplyWindow 回复判断窗口（5 分钟）
	//   - 5 分钟以内 + 最后一条是客户消息 → 触发 AI 回复
	//   - 5 分钟以外 → 仅落库不触发 AI（避免对历史存量消息逐一自动回复）
	InboxReplyWindow = 5 * time.Minute

	// InboxBackfillFutureTolerance 时序锚点判断的未来时间容差（5 秒）
	//   bridge 上报消息 timestamp 可能存在时钟漂移，5 秒以内视为正常追加
	//   超过 5 秒的未来消息 → 修正为 now()（避免消息排序错乱）
	InboxBackfillFutureTolerance = 5 * time.Second

	// InboxAIProcessingKey "AI 处理中" 标记 key 前缀（2026-08-05 新增）
	//
	// 解决"不断发消息给用户"根因：
	//   bridge 每秒扫描上报，msg_id 稳定 → 幂等跳过入库 ✅
	//   但 DB 最后一条还是 inbound（AI 异步推理未完成，outbound 还没落库）
	//   → 下次扫描又触发 AI → 重复发消息
	//
	// 修复：触发 AI 前设置 5min TTL 标记，AI 完成后（webhook.go sendOutbound）删除标记
	InboxAIProcessingKey = "hivemtk:ai_processing:"
	// InboxAIProcessingTTL AI 处理中标记 TTL（兜底，AI 完成后主动删除）
	//   2026-08-06 调整：从 30s 改为 2min
	//   - 原子 SetNX 已兜底并发重复触发；TTL 仅作崩溃兜底
	//   - 放宽到 2min 以覆盖较慢的推理模型（避免 TTL 提前过期→重检查误触发第二条）
	//   - AI 异常时最多 2min 内无法对该会话触发新 AI（由 sendOutbound 主动删除保证正常路径及时释放）
	InboxAIProcessingTTL = 2 * time.Minute

	// RecheckDelayAfterRelease 释放 AI 处理中标记后的重检查延迟
	//
	// 极限场景修复（AI 回复后用户新消息遗漏）：
	//   AI 推理期间用户发的新消息因 ai_processing 标记存在被跳过触发，
	//   AI 回复完成释放标记后需重新检查是否有未回复消息。
	//   延迟 800ms 是为了让 AI 推理期间到达的消息完成入库，再查 DB 最后一条方向。
	//   bridge 上报周期 1s，800ms 足以覆盖单次入库延迟（DB 写入通常 <50ms）。
	RecheckDelayAfterRelease = 800 * time.Millisecond
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
	return s.cache.SetNX(ctx, key, "busy", InboxAILockTTL)
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

// HandleIngressMessage 渠道消息统一入口（单条消息，供 WS 等单条入口调用）。
//
// 处理流程（2026-08-05 重构，用户科学方案）：
//  1. 标准化事件字段
//  2. 钩子1（sender_type 过滤）：self/agent → 直接丢弃，不入库不触发 AI
//  3. 检查人工接管锁（命中：仅落库，绕过 AI 路由）
//  4. 钩子2（入库判断）：按 msg_id 查 DB 是否已存在，存在 → 跳过
//  5. 持久化到 message_hub（含时序锚点判断）
//  6. 钩子3（回复判断）：查会话最后一条消息方向
//     - 最后一条 outbound（平台自己发的）→ 不回复
//     - 最后一条 inbound（客户发的）+ 5min 内 → 回复
//     - 最后一条 inbound + 5min 外 → 历史消息不回复
//  7. 触发 AI（单条调用，立即释放 AI 锁；批量合并由 HandleIngressBatch 处理）
//
// 注意：批量上报请用 HandleIngressBatch（按 conversation 分组 + batch 内合并 AI 回复）。
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

	// 钩子2.5（2026-08-07 第六轮修复）：服务端权威内容级去重。
	//   背景：前端 patrol 上报的消息 msg_id 在历史曾用 algo1（channel+conv+content），
	//   而服务端 ContentHashMsgID 已用 algo2（channel+content）。同内容生成不同 msg_id →
	//   钩子2 GetByMsgID 漏检 → 同内容 AI 回复被反复入库为 inbound → 触发循环 AI。
	//   修复：以 canonical contentHash (algo2) + 兜底按 platform+content 查重，无论 msg_id
	//   算法如何变化都视同"自/他回显"幂等跳过。
	if s.hubRepo != nil && event.Content != "" && event.Channel != "" {
		canonicalHash := ContentHashMsgID(event.Channel, event.ConversationID, event.Content)
		// 2026-08-07 第九轮修复：限本会话。algo2 下 canonicalHash 跨会话相同，
		// 跨会话命中会把其他客户发的相同内容误判为回显跳过。
		// AI 回环防护由第二道 GetByPlatformContent（限 outbound，跨会话）兜底。
		if existing, err := s.hubRepo.GetByContentHash(ctx, canonicalHash); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("canonical contentHash already exists in DB (self/echo); skip. existing msg_id=%s direction=%s", existing.MsgID, existing.Direction)
			return result, nil
		}
		if existing, err := s.hubRepo.GetByPlatformContent(ctx, event.Channel, event.Content); err == nil && existing != nil {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("platform+content already exists in DB (self/echo dedup); skip. existing msg_id=%s direction=%s", existing.MsgID, existing.Direction)
			return result, nil
		}
		// 归一化兜底（2026-08-07 修复）：DOM 中 AI 回复与 DB 落库内容可能有空格/换行差异，
		// 精确 md5 匹配失败 → 回环去重失效 → AI 回复被当客户 inbound 入库 → 触发循环 AI。
		// 去所有空白后比较，兼容 "安全。 需要" vs "安全。需要"。
		if existing, err := s.hubRepo.GetByPlatformContentNormalized(ctx, event.Channel, event.Content); err == nil && existing != nil {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("normalized platform+content match (self/echo dedup); skip. existing msg_id=%s direction=%s conv=%s", existing.MsgID, existing.Direction, existing.ConversationID)
			return result, nil
		}
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

// triggerAIForEvent 触发 AI 客服的公共方法（含 panic recover + 日志）
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

// ReleaseAIProcessingFlag 释放 "AI 处理中" 标记
//
// 由 webhook.go sendOutbound 在 AI 回复落库后调用，释放标记后下次扫描可触发新 AI
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

// withIngestLock 获取同一会话的入库分布式排他锁（SetNX），fn 处理完毕后释放。
//
// 用途（2026-08-06 用户诉求："前端在不断上报 → 连续重复入库 / 连续重复 AI 回复"，
//   用分布式排它锁保证处理完毕才放开）：
//   - 持有期间串行处理该会话的入库（时序锚点读取 + 跨表事务写入），
//     杜绝并发 TOCTOU 与重复入库（DB msg_id 唯一约束仍作最终兜底）。
//   - 获取失败（被其他实例/请求占用）则短暂重试；重试耗尽则跳过本次，
//     依赖前端重新上报 + DB 唯一约束保证最终幂等（不丢消息）。
//   - 无缓存后端（useMemo）时退化为直接执行（单实例安全，仍由 DB 唯一约束兜底）。
//
// 锁释放采用 token 校验（ReleaseLock 仅删除持有者自己的锁），避免误释放他人锁。
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

// RecheckUnrepliedAndTrigger AI 回复完成释放标记后，重新检查是否有未回复的客户消息。
//
// 极限场景修复（消息遗漏/孤儿消息）：
//
//	时序：用户消息1(inbound) → 触发AI（设置标记）→ AI推理中
//	      → 用户消息2(inbound)入库 → 查最后一条=inbound 但标记存在 → 跳过触发
//	      → AI回复消息1 → 释放标记 → 消息2成为"孤儿"无人回复
//
// 修复：释放标记后延迟 RecheckDelayAfterRelease（800ms）让期间消息入库，
//   再查 DB 最后一条方向：若仍为 inbound 且在 5min 窗口内，补触发一次 AI。
//
// 安全保障：
//   - 再次检查 ai_processing 标记：若释放后新一轮 AI 已被其他路径触发，则跳过
//   - 检查人工接管锁：AI 回复后用户可能转人工，不补触发
//   - 无 aiTrigger 时记录日志并跳过（不 panic）
//
// 调用方：webhook.go sendOutbound 释放标记后异步调用（go routine）。
// ctx 应为不受 sendOutbound 15s timeout 限制的独立 context。
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

// InboxIngressBatchResult 批量入站处理结果
type InboxIngressBatchResult struct {
	PerEvent    []*InboxIngressResult `json:"per_event"`     // 每条消息处理结果（与入参 events 索引对齐）
	TriggeredAI bool                  `json:"triggered_ai"` // 是否触发了 AI（batch 内合并触发）
	Reason      string                `json:"reason,omitempty"`
}

// HandleIngressBatch 批量入站处理（bridge HTTP 长轮询上报主入口）。
//
// 2026-08-05 重构（用户科学方案）：
//  1. 按 conversation_id 分组
//  2. 每组内逐条处理：sender_type 过滤 → msg_id 查 DB 去重 → 入库（含时序锚点判断）
//  3. 每组所有消息入库后，查 DB 最后一条消息方向：
//     - outbound（平台自己发的）→ 不回复
//     - inbound（客户发的）+ 5min 内 → 合并本组新增 inbound 消息内容，一次 AI 回复
//
// 用户诉求三要点：
//   - 入库依据：msg_id 查 DB 是否已存在（逐条检查，多条消息逐个入库）
//   - 回复依据：DB 最后一条消息方向（平台自己发的就不回复）
//   - AI 合并：多条 inbound 消息合并一次 AI 回复（不无限制给用户发消息）
func (s *InboxIngressService) HandleIngressBatch(ctx context.Context, events []*model.MessageEvent) (*InboxIngressBatchResult, error) {
	batchResult := &InboxIngressBatchResult{
		PerEvent: make([]*InboxIngressResult, len(events)),
	}
	if len(events) == 0 {
		return batchResult, nil
	}

	// 1. 按 conversation_id 分组（保留原索引以便回填 perEvent）
	type indexedEvent struct {
		idx   int
		event *model.MessageEvent
	}
	groups := make(map[string][]indexedEvent)
	for i, ev := range events {
		// 先 NormalizeEvent 补齐 ConversationID（兜底 Channel:account_id）
		if ev != nil {
			_ = s.NormalizeEvent(ctx, ev)
			convID := ev.ConversationID
			if convID == "" {
				convID = "_no_conv_"
			}
			groups[convID] = append(groups[convID], indexedEvent{idx: i, event: ev})
		}
	}

	// 2. 每组内逐条入库，记录新增 inbound 消息内容
	for convID, groupEvents := range groups {
		var newInboundContents []string
		var firstInboundEvent *model.MessageEvent // 用于 AI 触发的元数据（channel/accountID/senderID 等）
		for _, ie := range groupEvents {
			ev := ie.event
			// 调用单条处理（含 sender_type 过滤 / msg_id 去重 / 时序锚点 / 落库）
			// 注意：单条处理中的 AI 触发逻辑会被跳过（batch 末尾统一合并触发）
			r, err := s.handleIngressSingleForBatch(ctx, ev)
			batchResult.PerEvent[ie.idx] = r
			if err != nil {
				r.Reason = fmt.Sprintf("batch handle error: %v", err)
				continue
			}
			// 收集新增 inbound 消息内容（用于 batch 内合并 AI 回复）
			//   仅 customer 消息（非 self/agent/system）才合并触发 AI
			if r.Accepted && r.QueuedForAI && ev.SenderType != "self" && ev.SenderType != "agent" && ev.SenderType != "system" {
				newInboundContents = append(newInboundContents, ev.Content)
				if firstInboundEvent == nil {
					firstInboundEvent = ev
				}
			}
		}

		// 3. 本组消息全部入库后，查 DB 最后一条消息方向决定是否触发 AI
		if len(newInboundContents) == 0 || firstInboundEvent == nil {
			continue
		}
		// 跳过人工接管锁的会话
		humanLocked, _ := s.IsSessionHumanLocked(ctx, firstInboundEvent.SessionID)
		if humanLocked {
			continue
		}
		// 查 DB 最后一条消息方向
		if s.hubRepo != nil {
			unreplied, withinWindow, err := s.hubRepo.HasUnrepliedCustomerMessage(ctx, convID, InboxReplyWindow)
			if err != nil {
				logger.Ctx(ctx).Error().Err(err).
					Str("conv_id", convID).
					Msg("[Inbox][Batch] 查询最后一条消息方向失败，保守不触发 AI")
				continue
			}
			if !unreplied {
				logger.Ctx(ctx).Info().
					Str("conv_id", convID).
					Int("new_inbound_count", len(newInboundContents)).
					Msg("[Inbox][Batch] 最后一条是平台自己发的（outbound），不触发 AI")
				continue
			}
			if !withinWindow {
				logger.Ctx(ctx).Info().
					Str("conv_id", convID).
					Int("new_inbound_count", len(newInboundContents)).
					Msg("[Inbox][Batch] 最后一条 inbound 超过 5 分钟，历史消息不触发 AI")
				continue
			}
		}

		// 2026-08-05 修复"不断发消息"根因：
		//   AI 推理是异步的，从触发到 outbound 落库有几秒延迟
		//   期间 bridge 每秒扫描，DB 最后一条还是 inbound → 重复触发 AI
		//   修复：检查 "AI 处理中" 标记，存在则跳过（标记 TTL 5min，AI 完成后主动删除）
		if s.cache != nil {
			aiKey := InboxAIProcessingKey + convID
			if exists, _ := s.cache.Exists(ctx, aiKey); exists {
				logger.Ctx(ctx).Info().
					Str("conv_id", convID).
					Msg("[Inbox][Batch] AI 处理中（标记存在），跳过本次触发避免重复回复")
				continue
			}
		}

		// 4. 合并本组新增 inbound 消息，一次 AI 回复
		//    用户诉求："AI 可以合并用户的多个对话消息 合并一次发送"
		//    合并内容用换行分隔，AI 一次性回复覆盖所有新增 inbound 消息
		mergedEvent := *firstInboundEvent
		if len(newInboundContents) > 1 {
			mergedEvent.Content = strings.Join(newInboundContents, "\n")
			mergedEvent.EventID = uuid.NewString() // 合并后用新 eventID
			logger.Ctx(ctx).Info().
				Str("channel", mergedEvent.Channel).
				Str("conv_id", convID).
				Int("merged_count", len(newInboundContents)).
				Int("merged_content_len", len(mergedEvent.Content)).
				Msg("[Inbox][Batch] 合并多条 inbound 消息触发一次 AI")
		}
		s.triggerAIForEvent(ctx, &mergedEvent)
		batchResult.TriggeredAI = true
		batchResult.Reason = fmt.Sprintf("batch: %d messages merged, 1 AI trigger", len(newInboundContents))
	}
	return batchResult, nil
}

// handleIngressSingleForBatch 单条消息处理（batch 内部调用，跳过 AI 触发，由 batch 末尾统一合并触发）。
//
// 与 HandleIngressMessage 的区别：
//   - 不触发 AI（返回 QueuedForAI=true 标记，由 HandleIngressBatch 末尾合并触发）
//   - 保留 sender_type 过滤 / msg_id 去重 / 时序锚点 / 落库 等所有其他逻辑
func (s *InboxIngressService) handleIngressSingleForBatch(ctx context.Context, event *model.MessageEvent) (*InboxIngressResult, error) {
	result := &InboxIngressResult{
		SessionID: event.SessionID,
	}
	// 钩子1（2026-08-06 重构：服务端权威自/他判定，不再信任前端 sender_type）
	//   与 HandleIngressMessage 一致：ingest 入口数据不可信前端自/他标签，
	//   改用"内容是否命中本会话平台下发(outbound)"判定。命中 → SELF 回显跳过；
	//   未命中 → 强制 CUSTOMER（用户消息）走正常链路。
	isSystemMsg := event.SenderType == "system"

	// 检查人工接管锁
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

	// 钩子2：msg_id / content_hash 查 DB 去重（限本会话）
	//   2026-08-07 第九轮修复：限本会话。algo2 下同 channel+content 的 msg_id 相同，
	//   跨会话命中会把其他客户发的相同内容（如 XHS 系统提示）误跳过。
	//   AI 回环防护由钩子2.5 第二道 GetByPlatformContent（限 outbound，跨会话）兜底。
	if s.hubRepo != nil {
		// 主：event_id 命中（DOM id / 前端兜底 hash / 历史入库的消息）
		if event.EventID != "" {
			if existing, err := s.hubRepo.GetByMsgID(ctx, event.EventID); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
				result.Accepted = true
				result.QueuedForAI = false
				result.Reason = "msg_id already exists in DB; idempotent skip"
				return result, nil
			}
		}
		// 兜底：content_hash 命中。
		// 前端 _canonicalMsgId 统一使用 FNV-1a contentHash 作为 event_id，
		// 与服务端 ContentHashMsgID 逐字节一致。AI 出站消息 MsgID 即该值，
		// 前端 patrol 扫描到 AI 回显重新上报时 msg_id 相同 → GetByMsgID 命中 → 在此幂等跳过。
		if ch, ok := event.Extra["content_hash"].(string); ok && ch != "" {
			if existing, err := s.hubRepo.GetByMsgID(ctx, ch); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
				result.Accepted = true
				result.QueuedForAI = false
				result.Reason = "content_hash already exists in DB (platform outbound echo); idempotent skip"
				return result, nil
			}
		}
	}

	// 钩子2.5（2026-08-07 第六轮修复，防「同内容 AI 回复被 patrol 错乱反复入库为 inbound」）：
	//   背景：前端 patrol 上报的消息 msg_id 在历史曾用 algo1（channel+conv+content），
	//   而服务端 ContentHashMsgID 已用 algo2（channel+content）。同内容生成不同 msg_id →
	//   钩子2 GetByMsgID 漏检 → 同内容 AI 回复被反复入库为 inbound → 触发循环 AI。
	//   实际案例（2026-08-07 16:36:30）：给小红薯69C69EDE 的 AI 回复 mh:eef526c5（algo2）被
	//   前端 patrol 错乱上报为 mh:620ec4d5（algo1），msg_id 不同 → 钩子2 漏检 → 入库为
	//   小红薯会话 inbound → 触发新一轮 AI → 同样的回复内容再次被 patrol 错乱上报
	//   → 入库为「旭」会话（"其他客户"）inbound → 用户报告"缺下发给了所有人"。
	//
	//   修复：服务端权威去重——以 canonical contentHash (algo2) 为准，无论 msg_id 算法如何变化，
	//   都先查「本平台已有同 content 的任意消息」→ 命中 → 视为「自/他回显」→ 幂等跳过。
	//   2026-08-07 第九轮修复：第一道 GetByContentHash 限本会话（algo2 下跨会话同 canonicalHash）。
	//   AI 回环防护由第二道 GetByPlatformContent（限 outbound，跨会话）兜底。
	if s.hubRepo != nil && event.Content != "" && event.Channel != "" {
		canonicalHash := ContentHashMsgID(event.Channel, event.ConversationID, event.Content)
		// 第一道：按 canonical contentHash 直查（限本会话）
		if existing, err := s.hubRepo.GetByContentHash(ctx, canonicalHash); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("canonical contentHash already exists in DB (self/echo); skip. existing msg_id=%s direction=%s", existing.MsgID, existing.Direction)
			return result, nil
		}
		// 第二道：按 platform + content 直查（兜底，处理 canonicalHash 命中但方向语义不符/竞态等边界）
		if existing, err := s.hubRepo.GetByPlatformContent(ctx, event.Channel, event.Content); err == nil && existing != nil {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("platform+content already exists in DB (self/echo dedup); skip. existing msg_id=%s direction=%s", existing.MsgID, existing.Direction)
			return result, nil
		}
		// 归一化兜底（2026-08-07 修复）：DOM 中 AI 回复与 DB 落库内容可能有空格/换行差异，
		// 精确 md5 匹配失败 → 回环去重失效 → AI 回复被当客户 inbound 入库 → 触发循环 AI。
		// 去所有空白后比较，兼容 "安全。 需要" vs "安全。需要"。
		if existing, err := s.hubRepo.GetByPlatformContentNormalized(ctx, event.Channel, event.Content); err == nil && existing != nil {
			result.Accepted = true
			result.QueuedForAI = false
			result.Reason = fmt.Sprintf("normalized platform+content match (self/echo dedup); skip. existing msg_id=%s direction=%s conv=%s", existing.MsgID, existing.Direction, existing.ConversationID)
			return result, nil
		}
	}

	// 入库（含时序锚点判断）
	if err := s.persistMessage(ctx, event); err != nil {
		return result, fmt.Errorf("持久化消息失败: %w", err)
	}
	result.Accepted = true

	// 系统消息：仅落库，不触发 AI
	if isSystemMsg {
		result.QueuedForAI = false
		result.Reason = "sender_type=system; persisted only"
		return result, nil
	}

	// outbound 消息（self/agent，平台自己发的）：仅落库，不触发 AI
	//   注意：self/agent 已在钩子1过滤，这里兜底处理 Extra 中携带的 direction 字段
	if event.SenderType == "self" || event.SenderType == "agent" {
		result.QueuedForAI = false
		result.Reason = "sender_type=self/agent; persisted only (平台自己发的不触发 AI)"
		return result, nil
	}

	// inbound 消息（customer）：标记 QueuedForAI=true，由 batch 末尾合并触发
	result.QueuedForAI = true
	result.Reason = "batched; will be merged and triggered at batch end"
	return result, nil
}

// persistMessage 持久化消息到 message_hub 表（含时序锚点判断）。
//
// 时序处理（2026-08-05 用户诉求："上报的聊天记录可能存在时序不对（历史堆积），
//   如果插入一定要看 到底在锚点之前插入还是锚点之后插入"）：
//   - 锚点 = 会话已存在消息的最新 sent_at（DB 中该 conversation 的最后一条消息时间）
//   - timestamp 零值 → 用 now()（无法判断时序，视为当前消息）
//   - timestamp 未来超过 5 秒容差 → 修正为 now()（时钟漂移，避免排序错乱）
//   - timestamp < 锚点 → 历史堆积消息（backfill），保留原 timestamp，
//     DB 按 sent_at 排序时自然落到锚点之前（不污染"最后一条消息方向"判断）
//   - timestamp >= 锚点 → 正常追加（锚点之后）
//
// 注意：direction 不再硬编码为 inbound，按 event.Direction 透传（兼容 outbound 历史回填）。
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
	// 时序锚点判断：修正 timestamp
	now := time.Now()
	sentAt := event.Timestamp
	if sentAt.IsZero() {
		sentAt = now
	} else if sentAt.After(now.Add(InboxBackfillFutureTolerance)) {
		// 未来时间超过 5 秒容差 → 修正为 now()（时钟漂移）
		logger.Ctx(ctx).Info().
			Str("channel", event.Channel).
			Str("conv_id", event.ConversationID).
			Str("event_id", event.EventID).
			Time("orig_timestamp", event.Timestamp).
			Msg("[Inbox] 时序修正：timestamp 未来超过容差，修正为 now()")
		sentAt = now
	}
	// direction：根据 SenderType 推断（self/agent → outbound，其他 → inbound）
	//   MessageEvent 没有 Direction 字段，SenderType 已表明发送者身份
	direction := "inbound"
	if event.SenderType == "self" || event.SenderType == "agent" {
		direction = "outbound"
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
		IsAIReply:      event.IsAIReply,
		AIAgent:        event.AIAgent,
		IsRead:         false,
		SentAt:         sentAt,
		Extra:          nil,
	}
	// 全链路追踪：为消息分配 trace_id（inbound 起始新 trace；outbound 复用同会话最近 inbound 的 trace）
	if hub.TraceID == "" {
		if hub.Direction == "inbound" {
			hub.TraceID = tracing.LinkInboundTraceID(ctx, hub.ConversationID)
		} else {
			hub.TraceID = tracing.LinkOutboundTraceID(ctx, hub.ConversationID)
		}
	}
	if event.Extra != nil {
		extra := model.JSONMap{}
		for k, v := range event.Extra {
			extra[k] = v
		}
		hub.Extra = extra
	}
	if len(event.History) > 0 {
		if hub.Extra == nil {
			hub.Extra = model.JSONMap{}
		}
		hub.Extra["history"] = event.History
	}

	// 分布式排他锁：同一会话并发入库串行，保证入库不重复（前端不断上报 / 多实例时幂等）
	//   锁内完成：时序锚点读取 + 跨表事务写入；处理完毕才释放（ReleaseLock 仅删除持有者自己的锁）。
	//   DB msg_id 唯一约束仍作最终兜底（并发极小概率下锁获取失败后降级直接执行）。
	_, err := s.withIngestLock(ctx, event.ConversationID, func() error {
		// 历史堆积判断：timestamp 早于会话锚点 → backfill 消息（保留原 timestamp，DB 排序自动处理）
		if s.hubRepo != nil && event.ConversationID != "" {
			if anchor, aerr := s.hubRepo.GetLastByConversation(ctx, event.ConversationID); aerr == nil && anchor != nil {
				if sentAt.Before(anchor.SentAt) {
					logger.Ctx(ctx).Info().
						Str("channel", event.Channel).
						Str("conv_id", event.ConversationID).
						Str("event_id", event.EventID).
						Time("msg_timestamp", sentAt).
						Time("anchor_timestamp", anchor.SentAt).
						Msg("[Inbox] 时序锚点：消息 timestamp 早于锚点，标记为 backfill 历史堆积")
				}
			}
		}
		// 五层架构修复：跨表事务写入（message_hub + inbox_conversations 原子）
		// 注意：用 context.Background() 而非入参 ctx——ctx 随 WS 连接生命周期取消，
		// 而消息落库 + 收件箱同步不应被连接抖动打断（与原设计保持一致）。
		if s.inboxSvc != nil && hub.Direction == "inbound" {
			ingestTimer := tracing.StartSpan()
			if _, uerr := s.inboxSvc.UpsertFromHubMessageTx(context.Background(), hub, s.hubRepo); uerr != nil {
				if isDuplicateKey(uerr) {
					logger.Warnf("[Inbox] message_hub duplicate msg_id (idempotent skip): msg_id=%s session=%s",
						event.EventID, event.SessionID)
					return nil
				}
				return uerr
			}
			// 节点1 上报接入：入参（渠道/账号/会话/发送者/内容）→ 出参（落库 id/status）
			tracing.RecordNode(ctx, tracing.NodeSpan{
				TraceID:        hub.TraceID,
				ConversationID: hub.ConversationID,
				AccountID:      hub.AccountID,
				Channel:        hub.Platform,
				Node:           tracing.NodeIngest,
				Direction:      hub.Direction,
				MsgID:          hub.MsgID,
				Input: map[string]any{
					"channel":     event.Channel,
					"account_id":  accountID,
					"conv_id":     event.ConversationID,
					"sender_id":   event.SenderID,
					"sender_type": event.SenderType,
					"event_id":    event.EventID,
					"content_len": len(event.Content),
					"direction":   hub.Direction,
				},
				Output: map[string]any{
					"msg_id": hub.MsgID,
					"status": hub.Status,
					"id":     hub.ID,
				},
				DurationMs: ingestTimer.ElapsedMs(),
				Expected:   "客户消息落库 message_hub + 同步 inbox_conversations（平台统一收件箱可见）",
				Status:     tracing.StatusOk,
			})
			// 节点4 收件箱同步：上报接入事务内已原子完成，单独记一个节点便于链路可视化
			tracing.RecordNode(ctx, tracing.NodeSpan{
				TraceID:        hub.TraceID,
				ConversationID: hub.ConversationID,
				AccountID:      hub.AccountID,
				Channel:        hub.Platform,
				Node:           tracing.NodeInboxSync,
				Direction:      hub.Direction,
				MsgID:          hub.MsgID,
				Input:          map[string]any{"conv_id": event.ConversationID},
				Output:         map[string]any{"synced": true, "id": hub.ID},
				Expected:       "inbox_conversations 已建立/更新（避免 sync_gap：桥接活跃但平台收件箱看不到）",
				Status:         tracing.StatusOk,
			})
			return nil
		}
		if cerr := s.hubRepo.Create(ctx, hub); cerr != nil {
			if isDuplicateKey(cerr) {
				logger.Warnf("[Inbox] message_hub duplicate msg_id (idempotent skip): msg_id=%s session=%s",
					event.EventID, event.SessionID)
				return nil
			}
			return cerr
		}
		// 节点1 上报接入（outbound 历史回填 / inboxSvc 未注入路径，无收件箱同步）
		tracing.RecordNode(ctx, tracing.NodeSpan{
			TraceID:        hub.TraceID,
			ConversationID: hub.ConversationID,
			AccountID:      hub.AccountID,
			Channel:        hub.Platform,
			Node:           tracing.NodeIngest,
			Direction:      hub.Direction,
			MsgID:          hub.MsgID,
			Input: map[string]any{
				"channel":     event.Channel,
				"account_id":  accountID,
				"conv_id":     event.ConversationID,
				"sender_id":   event.SenderID,
				"sender_type": event.SenderType,
				"event_id":    event.EventID,
				"content_len": len(event.Content),
				"direction":   hub.Direction,
			},
			Output: map[string]any{
				"msg_id": hub.MsgID,
				"status": hub.Status,
				"id":     hub.ID,
			},
			Expected: "消息落库 message_hub",
			Status:   tracing.StatusOk,
		})
		return nil
	})
	return err
}

// PersistBridgeHistory 仅持久化历史/回填消息，不触发 AI 路由。
//
// 用途（需求⑤ 多用户历史 / 需求③ outbound 落库）：
//   - 页面加载时回填的存量私信（客户侧 inbound / 自己侧 outbound）
//   - 本扩展回写到网页的 AI 回复（outbound，标记为 AI 回复）
//
// 与 HandleIngressMessage 的关键区别：不获取 AI 锁、不投递 pending、不通知 AgentRuntime，
// 从而避免「回填空历史误触发 AI」与「自己回复被再次推理造成自回环」。
//
// 钩子2（2026-08-07 审计修复，防「AI 回复被 patrol 回显入库为 inbound」循环触发 AI）：
//   前端 history 项 event_id = contentHash（mh:xxxxxxxx），与服务端 AI outbound 的 MsgID
//   （= ContentHashMsgID）逐字节一致。命中 GetByMsgID → 跳过入库。
//   旧实现历史上下文回填路径完全未做 msg_id 去重，扩展 patrol 把 AI 回复放进 history 重新
//   上报时会被当作新 inbound 入库（direction 错乱），触发新 AI 回复 → 无限循环。
//   与 handleIngressSingleForBatch 钩子2 同源：唯一差异是不触发 AI。
func (s *InboxIngressService) PersistBridgeHistory(ctx context.Context, event *model.MessageEvent, direction string) error {
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return err
	}
	if direction == "" {
		direction = "inbound"
	}
	// 钩子2：msg_id 去重（与 handleIngressSingleForBatch 行为一致，限本会话）。
	// event_id 即前端 _canonicalMsgId = contentHash = 服务端 ContentHashMsgID 输出值。
	// 命中 → 已存在（inbound 或 outbound）→ 幂等跳过；未命中 → 走原落库。
	// 2026-08-07 第九轮修复：限本会话。algo2 下同 channel+content 的 msg_id 相同，
	// 跨会话命中会把其他客户发的相同内容误跳过。
	if s.hubRepo != nil && event.EventID != "" {
		if existing, err := s.hubRepo.GetByMsgID(ctx, event.EventID); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
			logger.Ctx(ctx).Info().
				Str("module", "bridge").
				Str("event_id", event.EventID).
				Str("existing_direction", existing.Direction).
				Str("conv_id", event.ConversationID).
				Str("channel", event.Channel).
				Str("sender_id", event.SenderID).
				Msg("[Inbox] PersistBridgeHistory 钩子2 命中：msg_id 已存在，幂等跳过（防回环）")
			return nil
		}
	}
	// 钩子2.5（2026-08-07 第六轮修复）：服务端权威内容级去重。
	//   与 handleIngressSingleForBatch 钩子2.5 同源：无论前端 msg_id 算法如何变化（algo1/algo2），
	//   只要本平台已有同 content 的任意消息就视为"自/他回显"，幂等跳过——避免同内容 AI 回复
	//   被 patrol 错乱反复入库为 inbound 触发循环 AI。
	//   2026-08-07 第九轮修复：第一道 GetByContentHash 限本会话（algo2 下跨会话同 canonicalHash）。
	//   AI 回环防护由第二道 GetByPlatformContent（限 outbound，跨会话）兜底。
	if s.hubRepo != nil && event.Content != "" && event.Channel != "" {
		canonicalHash := ContentHashMsgID(event.Channel, event.ConversationID, event.Content)
		if existing, err := s.hubRepo.GetByContentHash(ctx, canonicalHash); err == nil && existing != nil && existing.ConversationID == event.ConversationID {
			logger.Ctx(ctx).Info().
				Str("module", "bridge").
				Str("canonical_hash", canonicalHash).
				Str("existing_msg_id", existing.MsgID).
				Str("existing_direction", existing.Direction).
				Str("conv_id", event.ConversationID).
				Str("channel", event.Channel).
				Str("sender_id", event.SenderID).
				Msg("[Inbox] PersistBridgeHistory 钩子2.5 命中：canonical contentHash 已存在，幂等跳过（防回环）")
			return nil
		}
		if existing, err := s.hubRepo.GetByPlatformContent(ctx, event.Channel, event.Content); err == nil && existing != nil {
			logger.Ctx(ctx).Info().
				Str("module", "bridge").
				Str("existing_msg_id", existing.MsgID).
				Str("existing_direction", existing.Direction).
				Str("conv_id", event.ConversationID).
				Str("channel", event.Channel).
				Str("sender_id", event.SenderID).
				Msg("[Inbox] PersistBridgeHistory 钩子2.5 命中：platform+content 已存在，幂等跳过（防回环）")
			return nil
		}
		// 归一化兜底（2026-08-07 第十轮修复）：与 HandleIngressMessage / handleIngressSingleForBatch
		// 钩子2.5 第三道对齐。DOM 中 AI 回复与 DB 落库内容存在空格/换行/emoji 编码差异时，
		// 精确 md5 匹配失败 → 回环去重失效 → AI 回复被当客户 inbound 入库 → 触发循环 AI。
		// 去所有空白后比较，兼容 "您好！\n\n- 🛍️" (DB) vs "您好！ - 🛍️" (DOM)。
		// 此前 PersistBridgeHistory 路径缺失此钩子，导致 620+ 条 AI 话术被回采为 inbound pending。
		if existing, err := s.hubRepo.GetByPlatformContentNormalized(ctx, event.Channel, event.Content); err == nil && existing != nil {
			logger.Ctx(ctx).Info().
				Str("module", "bridge").
				Str("existing_msg_id", existing.MsgID).
				Str("existing_direction", existing.Direction).
				Str("existing_conv_id", existing.ConversationID).
				Str("conv_id", event.ConversationID).
				Str("channel", event.Channel).
				Str("sender_id", event.SenderID).
				Msg("[Inbox] PersistBridgeHistory 钩子2.5 命中：normalized platform+content 已存在，幂等跳过（防回环）")
			return nil
		}
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

// DeliverOutbound 持久化一条出站消息（如人工座席经桥接代发）到 message_hub(direction=outbound, status=pending)，
// 由桥接扩展 GET /api/bridge/outbox 拉取并转发到网页（2026-08-06 三通道架构）。
//
// 替代已废弃的内存 httpReplyBuffer 长轮询路径：ingest 改为即时返回后该 buffer 不再被读取，
// 若人工回复仍走 buffer 会静默丢失。本方法直接落库为待下发消息，与 AI 回复（webhook.go sendOutbound 桥接分支）
// 走同一下发队列，保证可靠投递。
func (s *InboxIngressService) DeliverOutbound(ctx context.Context, h *model.MessageHub) error {
	if h == nil {
		return errors.New("nil hub message")
	}
	if h.Direction == "" {
		h.Direction = "outbound"
	}
	if h.Status == "" {
		h.Status = "pending"
	}
	if h.SentAt.IsZero() {
		h.SentAt = time.Now()
	}
	if h.TraceID == "" {
		h.TraceID = tracing.LinkOutboundTraceID(ctx, h.ConversationID)
	}
	if err := s.hubRepo.Create(ctx, h); err != nil {
		return err
	}
	// 节点3 出站入队（人工/代发路径）：入参（渠道/账号/会话/内容）→ 出参（pending 出站 id）
	tracing.RecordNode(ctx, tracing.NodeSpan{
		TraceID:        h.TraceID,
		ConversationID: h.ConversationID,
		AccountID:      h.AccountID,
		Channel:        h.Platform,
		Node:           tracing.NodeOutboundEnqueue,
		Direction:      h.Direction,
		MsgID:          h.MsgID,
		Input: map[string]any{
			"channel":     h.Platform,
			"account_id":  h.AccountID,
			"conv_id":     h.ConversationID,
			"content_len": len(h.Content),
			"direction":   h.Direction,
			"is_ai_reply": h.IsAIReply,
		},
		Output: map[string]any{
			"msg_id": h.MsgID,
			"status": h.Status,
			"id":     h.ID,
		},
		Expected: "手动/代发回复落库 outbox(status=pending)，待下行出库",
		Status:   tracing.StatusOk,
	})
	// 同步到统一收件箱会话（inbox_conversations.last_message），使人工代发在 unifiedInbox 可见。
	// outbound 不计入未读（与飞书/企微一致），镜像 PersistBridgeHistory 的同步逻辑。
	if s.inboxSvc != nil {
		if _, err := s.inboxSvc.UpsertFromHubMessage(context.Background(), h); err != nil {
			logger.Warnf("[Inbox] 人工代发同步统一收件箱失败(conv=%s): %v", h.ConversationID, err)
		}
	}
	return nil
}

// ListPendingOutbound 查询某账号在某桥接渠道下"出站且待下发"的消息（下发队列）。
// 供桥接扩展独立轮询（通道C·下发轮询）拉取后转发到对应网页渠道。
// 仅返回 status='pending' 的出站消息；已 delivered 的被前端确认后排除，failed 的走离线补发。
func (s *InboxIngressService) ListPendingOutbound(ctx context.Context, channel, accountID string) ([]*model.MessageHub, error) {
	if s.hubRepo == nil {
		return nil, nil
	}
	list, _, err := s.hubRepo.ListByHubQuery(ctx, repository.HubListQuery{
		Platform:  channel,
		AccountID: accountID,
		Direction: "outbound",
		Status:    "pending",
		OrderBy:   "id ASC",
		PageSize:  50,
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ListPendingOutboundLimit 同 ListPendingOutbound，但支持自定义每页条数（前端 outboxBatchSize 控制）。
func (s *InboxIngressService) ListPendingOutboundLimit(ctx context.Context, channel, accountID string, limit int) ([]*model.MessageHub, error) {
	if s.hubRepo == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	list, _, err := s.hubRepo.ListByHubQuery(ctx, repository.HubListQuery{
		Platform:  channel,
		AccountID: accountID,
		Direction: "outbound",
		Status:    "pending",
		OrderBy:   "id ASC",
		PageSize:  limit,
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// AckOutboundDelivered 将扩展确认已下发的出站消息标记为 delivered（通道B·状态上报）。
// 仅对归属当前 (channel, accountID) 的 msg_id 生效，防止越权标记他人消息。
// 返回成功标记的数量（幂等：已 delivered 的不计入错误）。
func (s *InboxIngressService) AckOutboundDelivered(ctx context.Context, channel, accountID string, msgIDs []string) (int, error) {
	if s.hubRepo == nil || len(msgIDs) == 0 {
		return 0, nil
	}
	ok := 0
	ackTimer := tracing.StartSpan()
	for _, id := range msgIDs {
		if id == "" {
			continue
		}
		hub, err := s.hubRepo.GetByMsgID(ctx, id)
		if err != nil || hub == nil {
			continue
		}
		// 归属校验：只能确认本渠道/本账号的出站消息
		if hub.Platform != channel || hub.AccountID != accountID || hub.Direction != "outbound" {
			continue
		}
		if hub.Status == "delivered" {
			ok++
			continue
		}
		hub.Status = "delivered"
		if uerr := s.hubRepo.Update(ctx, hub); uerr != nil {
			logger.Ctx(ctx).Warn().Err(uerr).Str("module", "bridge").
				Str("msg_id", id).Msg("[Inbox] AckOutboundDelivered 更新失败")
			continue
		}
		// 节点6 送达确认：入参（渠道/账号/msg_id）→ 出参（delivered）
		tracing.RecordNode(ctx, tracing.NodeSpan{
			TraceID:        hub.TraceID,
			ConversationID: hub.ConversationID,
			AccountID:      hub.AccountID,
			Channel:        hub.Platform,
			Node:           tracing.NodeDeliveredAck,
			Direction:      hub.Direction,
			MsgID:          hub.MsgID,
			Input: map[string]any{
				"channel":    channel,
				"account_id": accountID,
				"msg_id":     id,
			},
			Output: map[string]any{
				"status": "delivered",
				"id":     hub.ID,
			},
			DurationMs: ackTimer.ElapsedMs(),
			Expected:   "pending → delivered（桥接已成功转发到网页）",
			Status:     tracing.StatusOk,
		})
		ok++
	}
	return ok, nil
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

// isPlatformMessage 及相关函数已于 2026-08-07 删除。
// 消息去重现由前端统一生成 contentHash (FNV-1a mh:xxxxxxxx) 作为 msg_id，
// 服务端 GetByMsgID 匹配即跳过。不再依赖 sender_type 或内容精确匹配。
//
// 架构原则（用户指定）：
//   前端不可信自/他判定 → 所有消息 sender_type='customer'，统一上报。
//   服务端通过 msg_id(内容哈希) 全局去重：命中 = 已有消息 = 跳过；未命中 = 新消息 = 用户发的。

// truncateForLog 截断字符串用于日志输出（避免日志过长）。
func truncateForLog(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

// accountIDOf 已于 2026-08-07 删除（无生产调用者）。

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
