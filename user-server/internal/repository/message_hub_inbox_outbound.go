package repository

import (
	"context"

	"fmt"

	"time"

	"hivemtk-user/internal/model"
)

func (r *MessageHubRepository) AckOutboundDeliveredBatch(ctx context.Context, channel, accountID string, msgIDs []string) (int64, error) {
	if r.db == nil || len(msgIDs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.MessageHub{}).
		Where("platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status IN ('pending','inflight')", channel, accountID, msgIDs).
		Update("status", "delivered")
	return res.RowsAffected, res.Error
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

// GetByMsgIDsInScope 批量查询 (platform, account_id, msg_id IN [...]) 的 message_hub 行（P3-D）。
//
// 用于 ack 详细化：先批量查每条 msg_id 的当前 status，再分类执行更新 / 幂等跳过 / 不存在。
// 限制 (platform, account_id) 避免越权查询其他账号的出站消息。
func (r *MessageHubRepository) GetByMsgIDsInScope(ctx context.Context, platform, accountID string, msgIDs []string) ([]model.MessageHub, error) {
	if r == nil || r.db == nil || len(msgIDs) == 0 || platform == "" || accountID == "" {
		return nil, nil
	}
	rows := make([]model.MessageHub, 0, len(msgIDs))
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND msg_id IN ?", platform, accountID, msgIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
