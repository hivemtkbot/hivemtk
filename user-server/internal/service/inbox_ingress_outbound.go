package service

import (
	"context"

	"errors"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/tracing"
)

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

// InboxOutboundClaimTimeout inflight 行最大存活：超过则被 ClaimPendingOutbound 惰性回收为 pending。
// 须大于单轮「下发+ack」时延（前端 pollInterval≈3s、发送+网络往返通常 < 数秒），留足余量。
const InboxOutboundClaimTimeout = 30 * time.Second

// ClaimPendingOutbound 服务端权威认领下发消息（根除重复转发）。详见 repository.ClaimPendingOutbound。
// 由桥接 GetBridgeOutbox 调用，取代旧的「仅 SELECT pending 不排他」查询。
func (s *InboxIngressService) ClaimPendingOutbound(ctx context.Context, channel, accountID string, limit int) ([]*model.MessageHub, error) {
	if s.hubRepo == nil {
		return nil, nil
	}
	rows, err := s.hubRepo.ClaimPendingOutbound(ctx, channel, accountID, limit, InboxOutboundClaimTimeout)
	if err != nil {
		return nil, err
	}
	out := make([]*model.MessageHub, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// AckOutboundDelivered 将扩展确认已下发的出站消息标记为 delivered（通道B·状态上报）。
// 仅对归属当前 (channel, accountID) 的 msg_id 生效，防止越权标记他人消息。
// 返回本次实际翻转为 delivered 的行数（幂等：已 delivered 的不计入）。
//
// 注意：msg_id 由 (channel+content) 生成，同一内容可出现在多个会话（复合唯一索引 (msg_id, conversation_id)
// 允许同 msg_id 跨会话存储）。因此必须批量更新该 (channel, account_id) 下所有匹配 msg_id 的 pending 行，
// 而非仅 GetByMsgID 返回的单行——否则跨会话的重复出站消息会永远停留在 pending，污染 stuck_unreachable 监控。
func (s *InboxIngressService) AckOutboundDelivered(ctx context.Context, channel, accountID string, msgIDs []string) (int, error) {
	if s.hubRepo == nil || len(msgIDs) == 0 {
		return 0, nil
	}

	affected, err := s.hubRepo.AckOutboundDeliveredBatch(ctx, channel, accountID, msgIDs)
	if err != nil {
		return 0, err
	}

	ackTimer := tracing.StartSpan()
	for _, id := range msgIDs {
		if id == "" {
			continue
		}
		hub, _ := s.hubRepo.GetByMsgID(ctx, id)
		var traceID, convID string
		if hub != nil {
			traceID, convID = hub.TraceID, hub.ConversationID
		}
		tracing.RecordNode(ctx, tracing.NodeSpan{
			TraceID:        traceID,
			ConversationID: convID,
			AccountID:      accountID,
			Channel:        channel,
			Node:           tracing.NodeDeliveredAck,
			Direction:      "outbound",
			MsgID:          id,
			Input: map[string]any{
				"channel":    channel,
				"account_id": accountID,
				"msg_id":     id,
			},
			Output: map[string]any{
				"status": "delivered",
			},
			DurationMs: ackTimer.ElapsedMs(),
			Expected:   "pending → delivered（桥接已成功转发到网页）",
			Status:     tracing.StatusOk,
		})
	}
	return int(affected), nil
}

// AckOutboundItem 单条 msg_id 的 ack 处理结果（2026-08-15 P3-D）。
//
// 状态语义：
//   - "acked"        此前为 pending/inflight，本次已翻转为 delivered
//   - "duplicate"    此前已为 delivered，本次幂等跳过（前端应从本地重试队列移除并停止重发）
//   - "not_found"    本渠道账号下不存在该 msg_id（前端应停止重发，可能被 GC 回收或归属错）
//
// 错误码语义：
//   - ""              无错误
//   - "ownership_mismatch" (预留) msg_id 归属其他 (platform, account_id)
type AckOutboundItem struct {
	MsgID  string `json:"msg_id"`
	Status string `json:"status"` // acked | duplicate | not_found
}

// AckOutboundResult 批量 ack 结果（P3-D 详细化 + P4 二次审核 6.2 区分 affected 与 acked_items）。
//
// 字段语义（2026-08-15 P4）：
//   - AffectedCount: SQL UPDATE 实际翻转为 delivered 的行数（跨会话同名 msg_id 时可能 > AckedItemsCount）
//   - AckedItemsCount: items 中 status='acked' 的元素数（= 真正"被本次 ack 命中"的 msg_id 数）
//   - DuplicateCount: 此前已为 delivered 的 msg_id 数（幂等跳过）
//   - NotFoundCount: 不存在的 msg_id 数
//   - Items: 按入参 msgIDs 顺序逐条结果（顺序契约由代码保证并由 p3d-contract.test.js 验证）
type AckOutboundResult struct {
	AffectedCount   int              `json:"affected_count"`     // SQL 行级受影响（= len(updatedIDs)）
	AckedItemsCount int              `json:"acked_items_count"`  // msg_id 维度 ack 命中数
	DuplicateCount  int              `json:"duplicate_count"`    // 幂等跳过
	NotFoundCount   int              `json:"not_found_count"`    // 不存在
	Items           []AckOutboundItem `json:"items"`
}

// AckOutboundDeliveredDetailed 批量 ack 详细版（P3-D + P4 二次审核修复）。
//
// 2026-08-15 头脑风暴二次论证 P3-D：
//   服务端按 (channel, account, msg_id) 去重，按 per-msg-id 状态分类返回。
//
// 2026-08-15 P4 二次审核修复（修复 2.1 / 3.1 / 6.2 / 7.4 / 2.3 / 1.1）：
//   1. hubRepo nil 直接返回 error（修复 3.1：原"全量空 result"会让前端误判为成功）
//   2. 用 UPDATE ... RETURNING 单 SQL 原子"分类 + 翻转"（修复 2.1：原"先查后更"非原子，
//      当其他 worker 在两步间抢先翻转为 delivered 时，本 worker 仍把 msg_id 标 acked，但
//      SQL 实际 affected=0——acked 与 affected 矛盾）
//   3. 增加 AckedItemsCount 字段，区分"行级 affected"与"msg_id 级 acked"（修复 6.2）
//   4. msg_id 入参去重（修复 7.4：["m1","m1","m1"] 时只算 1 次）
//   5. duplicate 状态记 StatusSkipped 而非 StatusOk（修复 2.3：tracing 监控失真）
//   6. GetByMsgIDsInScope 已加 direction='outbound' 过滤（修复 1.1）
//
// 协议契约（与前端 downlink.js 配套）：
//   响应：{ affected_count, acked_items_count, duplicate_count, not_found_count, items: [{msg_id, status}] }
//   items 顺序严格对齐入参 msgIDs 顺序（去重后）。
func (s *InboxIngressService) AckOutboundDeliveredDetailed(ctx context.Context, channel, accountID string, msgIDs []string) (*AckOutboundResult, error) {
	result := &AckOutboundResult{
		Items: make([]AckOutboundItem, 0, len(msgIDs)),
	}
	// P4-3.1：hubRepo nil 必须返 error（前端 ackRes.status !== 'ok' → retriable）
	if s.hubRepo == nil {
		return nil, errors.New("ack failed: hub repo not configured")
	}
	if len(msgIDs) == 0 {
		return result, nil
	}

	// P4-7.4：入参 msg_id 去重（保留首次出现顺序）
	seen := make(map[string]struct{}, len(msgIDs))
	deduped := make([]string, 0, len(msgIDs))
	for _, id := range msgIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return result, nil
	}

	// 1) 拉取所有目标 msg_id 在 (channel, account_id, direction=outbound) 下的当前 status
	//    P4-1.1：GetByMsgIDsInScope 已加 direction='outbound' 过滤
	rows, err := s.hubRepo.GetByMsgIDsInScope(ctx, channel, accountID, deduped)
	if err != nil {
		return nil, err
	}
	hubByMsgID := make(map[string]*model.MessageHub, len(rows))
	for i := range rows {
		h := rows[i]
		hubByMsgID[h.MsgID] = &h
	}

	// 2) 分类：可翻转的（pending/inflight） vs 已 delivered（幂等跳过） vs 不存在
	toAck := make([]string, 0, len(deduped))
	for _, id := range deduped {
		h, exists := hubByMsgID[id]
		if !exists {
			result.Items = append(result.Items, AckOutboundItem{MsgID: id, Status: "not_found"})
			result.NotFoundCount++
			continue
		}
		if h.Status == "delivered" {
			result.Items = append(result.Items, AckOutboundItem{MsgID: id, Status: "duplicate"})
			result.DuplicateCount++
			continue
		}
		// pending / inflight 等其他状态都翻转为 delivered
		toAck = append(toAck, id)
	}

	// 3) 原子 RETURNING 单 SQL（修复 2.1：消除"先查后更"非原子性）
	updatedIDSet := make(map[string]struct{}, len(toAck))
	if len(toAck) > 0 {
		updatedIDs, affected, err := s.hubRepo.AckOutboundDeliveredBatchReturning(ctx, channel, accountID, toAck)
		if err != nil {
			return nil, err
		}
		result.AffectedCount = int(affected)
		for _, id := range updatedIDs {
			updatedIDSet[id] = struct{}{}
		}
	}

	// 4) 精确分类（基于 RETURNING 真实结果，而非查询时的 status）
	//    之前误分类为 "acked" 但实际未翻转的 msg_id（被其他 worker 抢先）现在归为 duplicate
	finalItems := make([]AckOutboundItem, 0, len(result.Items))
	ackedItemsCount := 0
	for _, item := range result.Items {
		switch item.Status {
		case "acked":
			if _, ok := updatedIDSet[item.MsgID]; ok {
				// 确实翻了 → 保持 acked
				finalItems = append(finalItems, item)
				ackedItemsCount++
			} else {
				// 翻失败（被抢先）→ 降级为 duplicate
				finalItems = append(finalItems, AckOutboundItem{MsgID: item.MsgID, Status: "duplicate"})
				result.DuplicateCount++
			}
		default:
			// not_found / duplicate 直接保留
			finalItems = append(finalItems, item)
		}
	}
	result.Items = finalItems
	result.AckedItemsCount = ackedItemsCount

	// 5) tracing 节点：每条已处理 msg_id 记一条（修复 2.3：duplicate 记 StatusSkipped 不再 StatusOk）
	ackTimer := tracing.StartSpan()
	for _, item := range result.Items {
		if item.Status == "not_found" {
			continue
		}
		h := hubByMsgID[item.MsgID]
		var traceID, convID string
		if h != nil {
			traceID, convID = h.TraceID, h.ConversationID
		}
		var traceStatus string
		switch item.Status {
		case "acked":
			traceStatus = tracing.StatusOk
		case "duplicate":
			traceStatus = tracing.StatusSkipped
		default:
			traceStatus = tracing.StatusOk
		}
		tracing.RecordNode(ctx, tracing.NodeSpan{
			TraceID:        traceID,
			ConversationID: convID,
			AccountID:      accountID,
			Channel:        channel,
			Node:           tracing.NodeDeliveredAck,
			Direction:      "outbound",
			MsgID:          item.MsgID,
			Input: map[string]any{
				"channel":    channel,
				"account_id": accountID,
				"msg_id":     item.MsgID,
				"ack_status": item.Status,
			},
			Output: map[string]any{
				"status": "delivered",
			},
			DurationMs: ackTimer.ElapsedMs(),
			Expected:   "pending → delivered（桥接已成功转发到网页）",
			Status:     traceStatus,
		})
	}
	return result, nil
}
