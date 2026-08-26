package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ConsumeQuotaAtomic 原子扣减企微账号日配额（W-7）。
//
// 原 WeComAccountHealthService.ConsumeQuota 为「读 used → 内存判断 → 写回 used+count」
// 的读改写模式，并发调用会互相覆盖导致超发。本方法用单条条件 UPDATE 原子完成
// 「校验 + 扣减」，由数据库行锁保证竞态安全：
//
//	UPDATE wecom_accounts
//	SET daily_msg_used = daily_msg_used + ?, total_sent = total_sent + ?, last_active_at = ?
//	WHERE id = ? AND status = 1 AND login_state <> 'banned'
//	  AND daily_msg_used + ? <= daily_msg_quota
//
// 返回 true 表示扣减成功；false 表示未命中（配额不足/封禁/禁用/不存在），不区分原因，
// 由 Service 层预检给出精确错误语义。
func (r *WeComAccountRepository) ConsumeQuotaAtomic(ctx context.Context, id uint, count int) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("wecom account repo is nil")
	}
	if id == 0 || count <= 0 {
		return false, nil
	}
	res := r.db.WithContext(ctx).Model(&model.WeComAccount{}).
		Where("id = ? AND status = 1 AND login_state <> ? AND daily_msg_used + ? <= daily_msg_quota",
			id, "banned", count).
		Updates(map[string]any{
			"daily_msg_used": gorm.Expr("daily_msg_used + ?", count),
			"total_sent":     gorm.Expr("total_sent + ?", count),
			"last_active_at": time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
