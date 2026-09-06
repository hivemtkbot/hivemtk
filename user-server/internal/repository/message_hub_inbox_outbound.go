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
		Where("(CASE WHEN sent_at > '2000-01-01'::timestamptz THEN sent_at ELSE created_at END) > now() - interval '2 hours'").
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

	if conversationID == "" || len(content) < 20 {
		return nil, nil
	}
	var rows []model.MessageHub
	subQ := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND conversation_id = ?", platform, conversationID).
		Where("(CASE WHEN sent_at > '2000-01-01'::timestamptz THEN sent_at ELSE created_at END) > now() - interval '2 hours'").
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

// ListRecentOutboundInConv 列出本账号同会话近期 outbound（回环兜底检测用）。
//
// 按 sent_at 过滤（DOM 抓取回显的变体比对在 service 层做归一化后进行，
// 仓储只负责按物理时间窗取候选行，不掺内容判断）。
func (r *MessageHubRepository) ListRecentOutboundInConv(ctx context.Context, platform, accountID, conversationID string, since time.Time, limit int) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || platform == "" || conversationID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []model.MessageHub
	q := r.db.WithContext(ctx).
		Where("platform = ? AND direction = 'outbound' AND conversation_id = ?", platform, conversationID).
		Where("sent_at > ?", since).
		Order("id DESC").
		Limit(limit)
	if accountID != "" {
		q = q.Where("account_id = ?", accountID)
	}
	err := q.Find(&rows).Error
	return rows, err
}

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

	const qBase = `UPDATE message_hub
		SET status = ?, sent_at = now()
		WHERE platform = ? AND account_id = ? AND direction = 'outbound'
		  AND msg_id IN ? AND status IN ('pending','inflight')`
	q := qBase
	if conversationID != "" {
		q = qBase + " AND conversation_id = ?"
	}

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
