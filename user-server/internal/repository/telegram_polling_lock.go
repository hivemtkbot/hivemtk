package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	_db "hivemtk-user/internal/pkg/db"
)

// ErrPollingLockDBNil DB 句柄未初始化（service 启动期或测试中可能遇到）
var ErrPollingLockDBNil = errors.New("polling lock: db is nil")

// PollingLockStaleThreshold 锁过期阈值（超过此时间未心跳视为僵尸锁）
//
// 与 service.PollingLockStaleThreshold 保持一致；调整需同步更新两边。
const PollingLockStaleThreshold = 60 * time.Second

// PollingLockInfo 锁查询结果（含当前 owner + 最后心跳时间，用于诊断）
type PollingLockInfo struct {
	Owner         string
	LastHeartbeat *time.Time
}

// TelegramPollingLockRepository Telegram Polling 分布式锁仓库
type TelegramPollingLockRepository struct {
	db *gorm.DB
}

// NewTelegramPollingLockRepository 创建 Telegram Polling 锁仓库实例
//
// 绑定全局默认 DB（与项目其他 repository 一致）。
func NewTelegramPollingLockRepository() *TelegramPollingLockRepository {
	return &TelegramPollingLockRepository{db: _db.GetDB()}
}

// NewTelegramPollingLockRepositoryWithDB 创建指定 DB 连接的锁仓库
//
// 用于 service 依赖注入 / 多 DB 切换 / 测试场景。
func NewTelegramPollingLockRepositoryWithDB(db *gorm.DB) *TelegramPollingLockRepository {
	if db == nil {
		return &TelegramPollingLockRepository{db: _db.GetDB()}
	}
	return &TelegramPollingLockRepository{db: db}
}

// SetDB 注入 db（用于测试 / 多 DB 切换）
func (r *TelegramPollingLockRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// GetDB 返回仓库绑定的 DB（兼容历史 service.db 用法，仅用于构造期）
func (r *TelegramPollingLockRepository) GetDB(ctx context.Context) *gorm.DB {
	return r.db
}

// TryAcquirePollingLock 原子抢占 Telegram 账号的 polling 锁
//
// 抢占条件（满足任一即可）：
//  1. 锁空闲（polling_owner = ” 或 NULL）
//  2. 锁过期（polling_heartbeat_at < now - 60s），视为僵尸锁，可被抢占
//  3. 锁就是 workerID 持有（支持 worker 内部重启续约）
//
// 返回值：
//   - acquired=true → 抢占成功，调用方应启动 worker 并周期性调用 HeartbeatPollingLock
//   - acquired=false → 抢占失败（被其他进程持有且未过期），调用方应放弃启动 polling
//
// 注意：r.db 为 nil 时直接返回 acquired=false + ErrPollingLockDBNil，
// 保护性降级，避免 panic。
func (r *TelegramPollingLockRepository) TryAcquirePollingLock(
	ctx context.Context,
	workerID string,
	accountID uint,
) (acquired bool, info PollingLockInfo, err error) {
	if r == nil || r.db == nil {
		return false, PollingLockInfo{}, ErrPollingLockDBNil
	}
	staleBefore := time.Now().Add(-PollingLockStaleThreshold)

	res := r.db.WithContext(ctx).Exec(
		`UPDATE telegram_accounts
		 SET polling_owner = ?, polling_heartbeat_at = ?
		 WHERE id = ?
		   AND (polling_owner = '' OR polling_owner IS NULL
		        OR polling_owner = ?
		        OR (polling_heartbeat_at IS NOT NULL AND polling_heartbeat_at < ?))`,
		workerID, time.Now(), accountID, workerID, staleBefore,
	)
	if res.Error != nil {
		return false, PollingLockInfo{}, res.Error
	}
	if res.RowsAffected == 0 {
		cur, qErr := r.getLockInfo(ctx, accountID)
		if qErr != nil {
			return false, PollingLockInfo{}, nil
		}
		return false, cur, nil
	}
	return true, PollingLockInfo{Owner: workerID, LastHeartbeat: nil}, nil
}

// HeartbeatPollingLock 续约锁：仅当 owner 仍是 workerID 时才更新心跳
//
// 续约失败（lockLost=true）表示锁已被其他进程抢占（心跳间隔 > 60s 时发生，
// 典型场景：本进程卡死被抢占，或人工介入）。调用方收到 lockLost=true 后
// 应停止 worker 并清理资源，避免重复 polling。
func (r *TelegramPollingLockRepository) HeartbeatPollingLock(
	ctx context.Context,
	workerID string,
	accountID uint,
) (lockLost bool, err error) {
	if r == nil || r.db == nil {
		return true, ErrPollingLockDBNil
	}
	res := r.db.WithContext(ctx).Exec(
		`UPDATE telegram_accounts
		 SET polling_heartbeat_at = ?
		 WHERE id = ? AND polling_owner = ?`,
		time.Now(), accountID, workerID,
	)
	if res.Error != nil {
		return true, res.Error
	}
	return res.RowsAffected == 0, nil
}

// ReleasePollingLock 释放锁：仅当 owner 仍是 workerID 时才清空
//
// 进程退出 / 主动停止 polling 时调用。即使 owner 已被其他进程抢占，
// 本函数也只清自己的 owner，不会影响其他活跃 worker。
func (r *TelegramPollingLockRepository) ReleasePollingLock(
	ctx context.Context,
	workerID string,
	accountID uint,
) error {
	if r == nil || r.db == nil {
		return ErrPollingLockDBNil
	}
	res := r.db.WithContext(ctx).Exec(
		`UPDATE telegram_accounts
		 SET polling_owner = '', polling_heartbeat_at = NULL
		 WHERE id = ? AND polling_owner = ?`,
		accountID, workerID,
	)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// IsPollingLockHeldByMe 检查某账号的 polling 锁是否由指定 workerID 持有
//
// 用于状态端点 / 调试。
func (r *TelegramPollingLockRepository) IsPollingLockHeldByMe(
	ctx context.Context,
	workerID string,
	accountID uint,
) bool {
	if r == nil || r.db == nil {
		return false
	}
	info, err := r.getLockInfo(ctx, accountID)
	if err != nil {
		return false
	}
	return info.Owner == workerID
}

// getLockInfo 查询当前锁的 owner + 最后心跳（私有辅助）
func (r *TelegramPollingLockRepository) getLockInfo(ctx context.Context, accountID uint) (PollingLockInfo, error) {
	if r == nil || r.db == nil {
		return PollingLockInfo{}, ErrPollingLockDBNil
	}
	var row struct {
		PollingOwner       string
		PollingHeartbeatAt *time.Time
	}
	qErr := r.db.WithContext(ctx).
		Table("telegram_accounts").
		Select("polling_owner, polling_heartbeat_at").
		Where("id = ?", accountID).
		Scan(&row).Error
	if qErr != nil {
		return PollingLockInfo{}, qErr
	}
	return PollingLockInfo{Owner: row.PollingOwner, LastHeartbeat: row.PollingHeartbeatAt}, nil
}
