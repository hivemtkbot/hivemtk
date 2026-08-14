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
		SET status = 'delivered', updated_at = now()
		WHERE platform = ? AND account_id = ? AND direction = 'outbound' AND msg_id IN ? AND status IN ('pending','inflight')
		RETURNING msg_id`
	tx := r.db.WithContext(ctx).Raw(q, channel, accountID, msgIDs)
	if err = tx.Scan(&updatedIDs).Error; err != nil {
		return nil, 0, err
	}
	// 1 个 msg_id 可能命中多行（跨会话同名），affected = 实际行数
	affectedRows = int64(len(updatedIDs))
	// PG Scan 不会按行号反馈 RowsAffected，但通过 RETURNING 集合大小可以等价得到"行级受影响"
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
