package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

func (r *MessageHubRepository) AckOutboundDeliveredBatch(ctx context.Context, channel, accountID string, msgIDs []string) (int64, error) {
	if r.db == nil || len(msgIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status IN ('pending','inflight')", channel, accountID, msgIDs).
		Update("status", model.BridgeAckStatusDelivered)
	return res.RowsAffected, res.Error
}

// AckOutboundDeliveredBatchReturning 原子 RETURNING（2026-08-15 P4 二次审核 2.1）。
//
// 用单条 SQL `UPDATE ... RETURNING msg_id` 一次完成"翻转 + 返回被翻转的 msg_id 集合"，
// 解决"先查后更"非原子性导致的 acked 计数虚高 / acked 与 affected 矛盾问题。
//
// 返回：
//   - updatedIDs: 本次实际被翻转为 delivered 的 msg_id 集合（去重）
//   - affectedRows: SQL RowsAffected（跨会话同名 msg_id 时可能 > len(updatedIDs)，因为 1 msg_id 命中多行）
//   - err: SQL 错误
//
// 设计依据：
//   2.1 高危问题（acked/affected 矛盾）：原实现先 GetByMsgIDsInScope 把 msg_id 标为 "acked"，
//     然后 AckOutboundDeliveredBatch 实际受影响 0 行时仍返回 acked，违反"acked 字段 = 实际 SQL affected rows"
//     契约。RETURNING 直接告诉调用方"哪些 msg_id 真的被翻了"，语义零歧义。
func (r *MessageHubRepository) AckOutboundDeliveredBatchReturning(ctx context.Context, channel, accountID string, msgIDs []string) (updatedIDs []string, affectedRows int64, err error) {
	if r.db == nil || len(msgIDs) == 0 {
		return nil, 0, nil
	}
	updatedIDs = make([]string, 0, len(msgIDs))
	const q = `UPDATE message_hub
		SET status = 'delivered', sent_at = now()
		WHERE platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status IN ('pending','inflight')
		RETURNING msg_id`
	tx := r.db.WithContext(ctx).Raw(q, channel, accountID, msgIDs)
	if err = tx.Scan(&updatedIDs).Error; err != nil {
		return nil, 0, err
	}
	affectedRows = int64(len(updatedIDs))
	return updatedIDs, affectedRows, nil
}

func (r *MessageHubRepository) ClaimPendingOutbound(ctx context.Context, channel, accountID string, limit int, claimTimeout time.Duration) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, nil
	}
	cutoff := time.Now().Add(-claimTimeout)

	if err := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND status = 'inflight' AND claimed_at IS NOT NULL AND claimed_at < ?", channel, accountID, cutoff).
		Updates(map[string]any{"status": "pending", "claimed_at": nil}).Error; err != nil {
		fmt.Printf("[ClaimPendingOutbound] 回收超时 inflight 失败（继续认领）: %v\n", err)
	}

	list := make([]model.MessageHub, 0, limit)
	const q = `UPDATE message_hub SET status = 'inflight', claimed_at = now()
		WHERE id IN (
			SELECT id FROM message_hub
			WHERE platform = ? AND account_id = ? AND direction = 'outbound' AND status = 'pending'
			ORDER BY id ASC LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`
	if err := r.db.WithContext(ctx).Raw(q, channel, accountID, limit).Scan(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetByMsgIDsInScope 批量查询 (platform, account_id, direction=outbound, msg_id IN [...]) 的 message_hub 行（P3-D / P4 二次审核 1.1）。
//
// 用于 ack 详细化：先批量查每条 msg_id 的当前 status，再分类执行更新 / 幂等跳过 / 不存在。
// 限制 (platform, account_id, direction='outbound') 避免：
//   1. 越权查询其他账号的出站消息
//   2. inbound 行（与 outbound 同 msg_id 的客户消息）混入 outbound 分类
//
// 2026-08-15 P4 二次审核 1.1 中危：原实现缺 direction 过滤 → 客户消息（inbound）与 AI 回复（outbound）
// 同 msg_id 时会被错误归类。现与 AckOutboundDeliveredBatch 保持对称。
func (r *MessageHubRepository) GetByMsgIDsInScope(ctx context.Context, platform, accountID string, msgIDs []string) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || len(msgIDs) == 0 || platform == "" || accountID == "" {
		return nil, nil
	}
	rows := make([]model.MessageHub, 0, len(msgIDs))
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ?", platform, accountID, msgIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetByMsgIDsInScopeWithConv 与 GetByMsgIDsInScope 类似，但支持可选 conversation_id 过滤（2026-08-15 P0-1）。
//
// 入参约定：
//   - conversationID == ""：等价于 GetByMsgIDsInScope（按 platform+account_id 范围）
//   - conversationID != ""：WHERE 增加 conversation_id = ?
//
// v2 协议下，前端会把每条 msg_id 关联到具体 conversation_id，服务端严格按 (platform, account_id, conversation_id)
// 范围查询，根除"跨会话同名 msg_id 一锅端"语义问题。
func (r *MessageHubRepository) GetByMsgIDsInScopeWithConv(ctx context.Context, platform, accountID, conversationID string, msgIDs []string) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || len(msgIDs) == 0 || platform == "" || accountID == "" {
		return nil, nil
	}
	rows := make([]model.MessageHub, 0, len(msgIDs))
	q := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ?", platform, accountID, msgIDs)
	if conversationID != "" {
		q = q.Where("conversation_id = ?", conversationID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetOutboundByPlatformSenderContent 按「平台 + 发送者名称 + 内容」查询服务端下发的 outbound 行（自回显权威判据）。
//
// 2026-08-15 补齐（fix 61af7bb 遗漏的仓储方法）：
//   - 桥接扩展把本账号 AI 出站回复从 DOM 抓取后重发，自/他声明不可信；
//     服务端以「真实下发的 outbound 内容」为唯一权威事实源，命中即判定为自身回显，拦截不落库。
//   - sender_name 为空时（极小概率）降级为 platform+content 兜底匹配。
//   - 仅匹配 direction='outbound'，避免把客户 inbound 消息误判为自身回显。
//   - 2026-08-17 修复回环漏判：AI 回复落库时 sender_name 恒为空（见 webhook_outbound.go），
//     而 bridge 回显上报时 sender_name 为对方昵称（getPeerName），二者 sender_name 必不相同 →
//     原精确匹配永久漏判 → 回环。修复：senderName 非空时匹配 (sender_name = ? OR sender_name = ''),
//     即 outbound 自身空 sender_name 也算命中；并加 2 小时窗口避免历史消息误杀。
//   - 2026-08-17 深查：精确 content = ? 仍漏判 DOM 拼接场景（同一 outbound 被抓两次拼成 2 倍长度，
//     或多条 AI 回复拼接）。增加包含匹配兜底：拉同会话近期 outbound，若 inbound content 完整包含
//     某条 outbound content 且长度差显著（inbound 更长），判定为回显拼接，拦截。
func (r *MessageHubRepository) GetOutboundByPlatformSenderContent(ctx context.Context, platform, senderName, content string) (*model.MessageHub, error) {
	return r.GetOutboundByPlatformSenderContentConv(ctx, platform, senderName, content, "")
}

// GetOutboundByPlatformSenderContentConv 同上但按 conversation_id 限定范围（包含匹配兜底必需）。
func (r *MessageHubRepository) GetOutboundByPlatformSenderContentConv(ctx context.Context, platform, senderName, content, conversationID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil || platform == "" || content == "" {
		return nil, nil
	}
	var row model.MessageHub
	q := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND content = ?", platform, content).
		Where("created_at > now() - interval '2 hours'").
		Order("id DESC")
	if conversationID != "" {
		q = q.Where("conversation_id = ?", conversationID)
	}
	if senderName != "" {
		q = q.Where("(sender_name = ? OR sender_name = '' OR sender_name IS NULL)", senderName)
	}
	if err := q.First(&row).Error; err == nil {
		return &row, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 包含匹配兜底：inbound content 完整包含某条近期 outbound content。
	// 仅在 conversationID 非空时执行（避免跨会话误杀）；要求 outbound content 长度 >= 10
	// 且 inbound content 长度 > outbound content 长度（即 inbound 是 outbound 的超集）。
	if conversationID == "" || len(content) < 20 {
		return nil, nil
	}
	var rows []model.MessageHub
	subQ := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND conversation_id = ?", platform, conversationID).
		Where("created_at > now() - interval '2 hours'").
		Where("length(content) >= 10").
		Order("id DESC").
		Limit(20)
	if err := subQ.Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		oc := rows[i].Content
		if len(oc) >= 10 && len(content) > len(oc) && strings.Contains(content, oc) {
			return &rows[i], nil
		}
	}
	return nil, nil
}

// AckOutboundDeliveredBatchReturningWithStatus 原子 RETURNING + 可配置终态（2026-08-15 P0-3 + P0-1）。
//
// 与 AckOutboundDeliveredBatchReturning 区别：
//   - terminalStatus: 目标终态（"delivered" | "failed"），不写死 delivered
//   - conversationID: 非空时 WHERE 增加 conversation_id = ? 限定 ack 范围（解决跨会话一锅端）
//   - status IN ('pending','inflight'): 仅翻可翻转的行；已为 terminalStatus 的不重入
//
// 用单条 SQL `UPDATE ... RETURNING msg_id` 一次完成"翻转 + 返回被翻转的 msg_id 集合"，
// 解决"先查后更"非原子性导致的 acked 计数虚高 / acked 与 affected 矛盾问题（P4 二次审核 2.1）。
func (r *MessageHubRepository) AckOutboundDeliveredBatchReturningWithStatus(
	ctx context.Context,
	channel, accountID, conversationID, terminalStatus string,
	msgIDs []string,
) (updatedIDs []string, affectedRows int64, err error) {
	if r.db == nil || len(msgIDs) == 0 {
		return nil, 0, nil
	}
	if terminalStatus != model.BridgeAckStatusDelivered && terminalStatus != model.BridgeAckStatusFailed {
		return nil, 0, fmt.Errorf("invalid terminal status: %q", terminalStatus)
	}
	updatedIDs = make([]string, 0, len(msgIDs))

	// SQL 拼接：conversation_id 过滤可选
	const qBase = `UPDATE message_hub
		SET status = ?, sent_at = now()
		WHERE platform = ? AND account_id = ? AND direction = 'outbound'
		  AND msg_id IN ? AND status IN ('pending','inflight')`
	q := qBase
	if conversationID != "" {
		q = qBase + " AND conversation_id = ?"
	}

	// 用 Raw + Scan 到 []string：RETURNING 集合大小 = 实际受影响行数
	var tx *gorm.DB
	if conversationID != "" {
		tx = r.db.WithContext(ctx).Raw(q+" RETURNING msg_id", terminalStatus, channel, accountID, msgIDs, conversationID)
	} else {
		tx = r.db.WithContext(ctx).Raw(q+" RETURNING msg_id", terminalStatus, channel, accountID, msgIDs)
	}
	if err = tx.Scan(&updatedIDs).Error; err != nil {
		return nil, 0, err
	}
	affectedRows = int64(len(updatedIDs))
	return updatedIDs, affectedRows, nil
}

// AnyExistsByMsgIDs 检查 msg_id 列表在指定 channel 下"任何账号"是否存在（2026-08-15 P0-8）。
//
// 用途：ack 详细化流程中，msg_id 在 (channel, account_id) 范围 not_found 时，
// 调用此方法确认其是否"被其他账号持有"——若是，分类为 not_in_scope（防越权探测）。
//
// 返回：map[msg_id]bool，true 表示存在（任何账号下任何方向的行）
func (r *MessageHubRepository) AnyExistsByMsgIDs(ctx context.Context, channel string, msgIDs []string) (map[string]bool, error) {
	if r == nil || r.db == nil || len(msgIDs) == 0 || channel == "" {
		return map[string]bool{}, nil
	}
	out := make(map[string]bool, len(msgIDs))
	for _, id := range msgIDs {
		out[id] = false
	}
	type idRow struct {
		MsgID string `gorm:"column:msg_id"`
	}
	var rows []idRow
	err := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Select("DISTINCT msg_id").
		Where("platform = ? AND msg_id IN ?", channel, msgIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.MsgID] = true
	}
	return out, nil
}

// FetchOutboundSince 查询指定 channel + account_id 下，id > sinceID 的 outbound 消息列表。
//
// 用途：SSE 初始拉取 + 轮询兜底，用于从 DB 恢复/补齐未读消息。
//   - sinceID = 0: 返回全部（限制 limit）
//   - sinceID > 0: 返回 id > sinceID 的记录（用于增量拉取）
//   - 按 id ASC 排序，保证时序
//
// 2026-08-18 Phase 1: 供 bridge outboxDBFetcher 使用，避免 SSE 依赖内存队列。
func (r *MessageHubRepository) FetchOutboundSince(ctx context.Context, channel, accountID string, sinceID uint64, limit int) ([]model.MessageHub, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []model.MessageHub
	q := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = 'outbound'", channel, accountID)
	if sinceID > 0 {
		q = q.Where("id > ?", sinceID)
	}
	q = q.Order("id ASC").Limit(limit)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

