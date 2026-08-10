// 拆分自 inbox_ingress.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

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
	// 批量翻转入站 pending 行（归属由 repo 层 WHERE 的 platform/account_id 保证；仅处理 pending，failed 为终态）。
	affected, err := s.hubRepo.AckOutboundDeliveredBatch(ctx, channel, accountID, msgIDs)
	if err != nil {
		return 0, err
	}
	// 追踪节点：每个 msg_id 记录一条送达确认（用于可视化），上下文取首个匹配行（若有）。
	ackTimer := tracing.StartSpan()
	for _, id := range msgIDs {
		if id == "" {
			continue
		}
		hub, _ := s.hubRepo.GetByMsgID(ctx, id) // 仅作追踪上下文；可能为 nil（如历史 msg_id）
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
