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

// AckOutboundResult 批量 ack 结果（P3-D 详细化）。
type AckOutboundResult struct {
	AffectedCount int              `json:"affected_count"`
	DuplicateCount int             `json:"duplicate_count"`
	NotFoundCount int              `json:"not_found_count"`
	Items         []AckOutboundItem `json:"items"`
}

// AckOutboundDeliveredDetailed 批量 ack 详细版（P3-D）：返回每条 msg_id 的处理状态。
//
// 2026-08-15 头脑风暴二次论证 P3-D（ack 幂等协议 6/10 → 10/10）：
//   同类对比（whatsapp-web.js / WPPConnect）通过 event_id 去重防止重发；
//   Baileys 通过 msg.key.id + sendUniqueKey 双重去重。
//   本服务现状：AckOutboundDelivered 内部已幂等（仅更新 status IN pending/inflight），
//   但前端拿到的是 affected_count，无法区分：
//     - 哪些 msg_id 是"本次成功 ack"
//     - 哪些 msg_id 是"已 delivered 的重复 ack"
//     - 哪些 msg_id 是"不存在的（GC 回收 / 归属错）"
//   这导致前端重试器无法精确停止重发——可能让用户收到重复消息。
//
// 协议契约（与前端 downlink.js 配套）：
//   响应：{ acked, duplicate_count, not_found_count, items: [{msg_id, status}, ...] }
//   前端收到 duplicate 时：清空本地重试队列 + 停止重发；
//   收到 not_found 时：记录异常 + 停止重发（可能是归属错或被 GC）；
//   收到 acked 时：正常清理重试队列。
func (s *InboxIngressService) AckOutboundDeliveredDetailed(ctx context.Context, channel, accountID string, msgIDs []string) (*AckOutboundResult, error) {
	result := &AckOutboundResult{
		Items: make([]AckOutboundItem, 0, len(msgIDs)),
	}
	if s.hubRepo == nil || len(msgIDs) == 0 {
		return result, nil
	}

	// 1) 拉取所有目标 msg_id 在 (channel, account_id) 下的当前 status
	rows, err := s.hubRepo.GetByMsgIDsInScope(ctx, channel, accountID, msgIDs)
	if err != nil {
		return nil, err
	}
	statusByMsgID := make(map[string]string, len(rows))
	for _, h := range rows {
		statusByMsgID[h.MsgID] = h.Status
	}

	// 2) 分类：需要更新的（pending/inflight） vs 已 delivered（幂等跳过） vs 不存在
	toAck := make([]string, 0, len(msgIDs))
	for _, id := range msgIDs {
		if id == "" {
			continue
		}
		status, exists := statusByMsgID[id]
		if !exists {
			result.Items = append(result.Items, AckOutboundItem{MsgID: id, Status: "not_found"})
			result.NotFoundCount++
			continue
		}
		if status == "delivered" {
			result.Items = append(result.Items, AckOutboundItem{MsgID: id, Status: "duplicate"})
			result.DuplicateCount++
			continue
		}
		// pending / inflight 等其他状态都翻转为 delivered
		toAck = append(toAck, id)
		result.Items = append(result.Items, AckOutboundItem{MsgID: id, Status: "acked"})
	}

	// 3) 批量更新
	if len(toAck) > 0 {
		affected, err := s.hubRepo.AckOutboundDeliveredBatch(ctx, channel, accountID, toAck)
		if err != nil {
			return nil, err
		}
		result.AffectedCount = int(affected)
	}

	// 4) 节点9 ack 上报：每条已处理 msg_id 记一条节点（与 AckOutboundDelivered 保持一致）
	ackTimer := tracing.StartSpan()
	for _, item := range result.Items {
		if item.Status == "not_found" {
			continue
		}
		hub, _ := s.hubRepo.GetByMsgID(ctx, item.MsgID)
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
			MsgID:          item.MsgID,
			Input: map[string]any{
				"channel":     channel,
				"account_id":  accountID,
				"msg_id":      item.MsgID,
				"ack_status":  item.Status, // 节点侧区分幂等 / 首次
			},
			Output: map[string]any{
				"status": "delivered",
			},
			DurationMs: ackTimer.ElapsedMs(),
			Expected:   "pending → delivered（桥接已成功转发到网页）",
			Status:     tracing.StatusOk,
		})
	}
	return result, nil
}
