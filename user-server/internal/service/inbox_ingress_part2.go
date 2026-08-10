// 拆分自 inbox_ingress.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
	// 统一收件中间件（2026-08-09，批次版）：与单条版一致，渠道+发送者+内容 服务端权威去重 / 自他判定。
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
			Msg("[Inbox] 中间件拦截（批次）：消息被去重/回环拦截，不穿透业务层")
		return result, nil
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

// ---------------------------------------------------------------------------
// 统一收件中间件：渠道+发送者+内容 服务端权威去重 / 自他判定（拦截在业务层之前）
// ---------------------------------------------------------------------------

// resolveSenderKey 解析消息的物理发送者标识（用于去重键的发送者维度）。
//
//	优先级：SenderName > SenderID > ConversationID（兜底，按会话隔离避免跨客户碰撞）> "unknown"。
//	说明：上报数据在前端无法可靠获取发送者名称（自他消息不准确），但平台回传的 SenderID
//	是真实客户/账号的物理标识（可靠）；仅"自/他标签"不可信，故自他由服务端 DB 回查判定。
func (s *InboxIngressService) resolveSenderKey(event *model.MessageEvent) string {
	if event.SenderName != "" {
		return event.SenderName
	}
	if event.SenderID != "" {
		return event.SenderID
	}
	if event.ConversationID != "" {
		return event.ConversationID
	}
	return "unknown"
}

// resolveAccountID 从事件 Extra 中取账号（平台身份）。
func resolveAccountID(event *model.MessageEvent) string {
	if event.Extra != nil {
		if v, ok := event.Extra["account_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// senderKeyForDedup 计算消息的去重发送者键（渠道+发送者+内容 三元组的"发送者"维度）。
//
//	核心：自他判定服务端权威，不信任前端 sender_type。
//	- 平台自己发出的消息（sender_type=self/agent，即 AI/人工客服）一律归一为账号(platform 身份)，
//	  与出站(outbound)落库时填入的 senderKey=accountID 保持一致 → 回显时哈希匹配被识别为"自己消息"。
//	- 其他（真实客户/上报不可信消息）使用物理发送者标识（SenderName/SenderID），本身即区分不同客户。
func (s *InboxIngressService) senderKeyForDedup(event *model.MessageEvent) string {
	sk := s.resolveSenderKey(event)
	if event.SenderType == "self" || event.SenderType == "agent" {
		if acc := resolveAccountID(event); acc != "" {
			sk = acc
		}
	}
	return sk
}

// interceptInbound 统一收件中间件：在消息落库/触发 AI 之前，依据「渠道+发送者+内容」唯一去重依据
// 做服务端权威拦截，避免无效/重复/回环消息穿透业务层。
//
// 设计要点（对照用户诉求）：
//  1. 去重依据 = 渠道 + 发送者名称 + 消息内容（ContentHashWithSender），前端后端共用同一算法。
//  2. 自/他判定依赖数据库检查消息哈希，而非前端不可信的 sender_type：
//     平台自己发出的消息必然先以 direction='outbound' 落库（AI/人工回复，SenderID=账号）。
//     故"当前上报命中同 dedup_hash 的 outbound 行"即判定为自己消息回显（回环），拦截。
//  3. 关键修复：去重键含发送者 → 「客户复述了 AI 的原话」因发送者不同哈希不同，
//     不再被旧逻辑（仅按 platform+content 匹配 outbound）误判为回显而丢失客户消息。
//  4. AI 返回的才是真正自己消息；前端上报的可能是自己也可能是他人 → 经 DB 哈希回查分别处理。
//
// 返回值：Blocked=true 时调用方应直接 ack 跳过，不落库、不触发 AI。
func (s *InboxIngressService) interceptInbound(ctx context.Context, event *model.MessageEvent) (*IngressDecision, error) {
	if s.hubRepo == nil {
		// 无 DB 不拦截（保持原行为，不阻断业务）
		return &IngressDecision{}, nil
	}
	content := strings.TrimSpace(event.Content)
	if content == "" || event.Channel == "" {
		// 空内容消息不参与内容去重（保持原行为）
		return &IngressDecision{}, nil
	}

	senderKey := s.senderKeyForDedup(event)
	dedupHash := ContentHashWithSender(event.Channel, senderKey, content)

	// 主路径：DB 权威回查 dedup_hash（含发送者）。
	//   命中 outbound → 自己消息回显（回环）→ 拦截；
	//   命中 inbound  → 同发送者同内容已存在（真实重复）→ 交由短窗去重处理（此处不直接拦，避免误删自然复述）。
	if existing, err := s.hubRepo.GetByDedupHash(ctx, event.Channel, dedupHash); err == nil && existing != nil {
		if existing.Direction == "outbound" {
			return &IngressDecision{Blocked: true, IsSelfEcho: true, Reason: "self-echo(outbound dedup_hash match)"}, nil
		}
	}

	// 兜底（防御纵深）：AI 回复与 DB 落库内容可能有空格/换行差异导致精确哈希失配 → 回环失效。
	//   归一化(去所有空白)后按 platform+content 匹配 outbound：
	//   仅当上报发送者与出站发送者一致（或缺失不可区分 / 被明确标 self/agent）才判回显，
	//   真实客户复述（发送者明显不同）则放行，避免误删客户消息。
	if ob, err := s.hubRepo.GetByPlatformContentNormalized(ctx, event.Channel, content); err == nil && ob != nil {
		// 命中出站(outbound)即平台自己曾发出过相同内容。判定为自/他回显需满足以下之一：
		//   - 上报发送者与出站发送者一致；
		//   - 任一侧发送者缺失（不可区分）→ 保守拦截，宁可误拦也不让回环触发 AI 死循环；
		//   - 上报被明确标 self/agent。
		// 出站消息在生产环境均由 persistMessage 写入 DedupHash 与 SenderID，sender 缺失仅见于
		// 历史/手工数据，此分支专门兜住这类"内容相同但缺发送者"的回显。
		if event.SenderID == "" || ob.SenderID == "" || event.SenderID == ob.SenderID ||
			event.SenderType == "self" || event.SenderType == "agent" {
			return &IngressDecision{Blocked: true, IsSelfEcho: true, Reason: "self-echo(normalized content+sender match)"}, nil
		}
	}

	// 辅助防护：同(渠道,发送者,内容)短时(5min)重复投递去重（防同一条上报被重复落库穿透业务层）。
	//   采用短窗而非永久去重：超过窗口的同内容会放行，保留客户自然的重复复述。
	if s.cache != nil {
		dupKey := InboxSenderContentDedupKey + dedupHash
		if ok, derr := s.cache.SetNX(ctx, dupKey, "1", InboxContentDedupTTL); derr == nil && !ok {
			return &IngressDecision{Blocked: true, IsDup: true, Reason: "duplicate(channel+sender+content) within window"}, nil
		}
	}

	return &IngressDecision{}, nil
}

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
	// ReceiverID 兜底（出站）：桥接扩展侧发送出站消息时，偶发将 receiver_id 写成发送方账号
	// （与 sender_id 相同）或留空，导致 message_hub.receiver_id 既非客户也非空。出站消息的接收方
	// 即客户，统一收件箱按 (platform, account_id, customer_id=receiver_id) 匹配客户会话，
	// receiver_id 错写成账号会导致关联到账号而非客户，造成 sync_gap 误报。故缺省取 conversation_id
	// （客户标识）。仅当 receiver_id 为空或等于发送方账号时兜底，避免误改合法群收件人。
	if direction == "outbound" && event.ConversationID != "" && (event.ReceiverID == "" || event.ReceiverID == event.SenderID) {
		event.ReceiverID = event.ConversationID
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
	// 统一收件去重哈希（渠道+发送者+内容）：落库即写入，供 interceptInbound 服务端权威自/他判定与回环拦截。
	// 发送者键复用 senderKeyForDedup（self/agent 归一为账号，与出站行一致），确保回显时哈希可被识别。
	hub.DedupHash = ContentHashWithSender(event.Channel, s.senderKeyForDedup(event), strings.TrimSpace(event.Content))
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

	// 非侵入钩子：消息成功落库后，异步投递线索发掘（不阻塞/不入侵核心业务）。
	// 用 persisted 标记仅在落库成功时触发一次；defer + recover 保证任何异常都不影响主链路。
	var persisted bool
	defer func() {
		if !persisted || hub == nil || s.leadMiningSvc == nil {
			return
		}
		func() {
			defer func() { _ = recover() }()
			s.leadMiningSvc.Enqueue(hub)
		}()
	}()

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
			persisted = true
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
		persisted = true
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
//
//	前端 history 项 event_id = contentHash（mh:xxxxxxxx），与服务端 AI outbound 的 MsgID
//	（= ContentHashMsgID）逐字节一致。命中 GetByMsgID → 跳过入库。
//	旧实现历史上下文回填路径完全未做 msg_id 去重，扩展 patrol 把 AI 回复放进 history 重新
//	上报时会被当作新 inbound 入库（direction 错乱），触发新 AI 回复 → 无限循环。
//	与 handleIngressSingleForBatch 钩子2 同源：唯一差异是不触发 AI。
func (s *InboxIngressService) PersistBridgeHistory(ctx context.Context, event *model.MessageEvent, direction string) error {
	if err := s.NormalizeEvent(ctx, event); err != nil {
		return err
	}
	if direction == "" {
		direction = "inbound"
	}
	// ReceiverID 兜底（出站）：桥接扩展侧发送出站消息时，偶发将 receiver_id 写成发送方账号
	// （与 sender_id 相同）或留空，导致 message_hub.receiver_id 既非客户也非空。出站消息的接收方
	// 即客户，统一收件箱按 (platform, account_id, customer_id=receiver_id) 匹配客户会话，
	// receiver_id 错写成账号会导致关联到账号而非客户，造成 sync_gap 误报。故缺省取 conversation_id
	// （客户标识）。仅当 receiver_id 为空或等于发送方账号时兜底，避免误改合法群收件人。
	if direction == "outbound" && event.ConversationID != "" && (event.ReceiverID == "" || event.ReceiverID == event.SenderID) {
		event.ReceiverID = event.ConversationID
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
