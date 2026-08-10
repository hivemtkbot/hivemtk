package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// UserBlacklistRepository 用户黑名单仓储
type UserBlacklistRepository struct {
	db *gorm.DB
}

// NewUserBlacklistRepository 创建黑名单仓储（使用全局 DB）
func NewUserBlacklistRepository() *UserBlacklistRepository {
	return &UserBlacklistRepository{db: _db.GetDB()}
}

// NewUserBlacklistRepositoryWithDB 使用指定 DB 创建仓储（测试用）
func NewUserBlacklistRepositoryWithDB(db *gorm.DB) *UserBlacklistRepository {
	return &UserBlacklistRepository{db: db}
}

// Add 添加黑名单记录
func (r *UserBlacklistRepository) Add(ctx context.Context, b *model.UserBlacklist) error {
	if b == nil {
		return errors.New("blacklist record is nil")
	}
	if b.UserID == "" {
		return errors.New("user_id is required")
	}
	// 幂等：若已存在 active 记录则更新其 reason / expires_at
	var existing model.UserBlacklist
	err := r.db.Where("user_id = ? AND platform = ? AND active = ?", b.UserID, b.Platform, true).First(&existing).Error
	if err == nil {
		existing.Reason = b.Reason
		existing.Source = b.Source
		existing.OperatorID = b.OperatorID
		existing.OperatorName = b.OperatorName
		existing.SessionID = b.SessionID
		existing.ExpiresAt = b.ExpiresAt
		existing.UpdatedAt = time.Now()
		return r.db.Save(&existing).Error
	}
	return r.db.Create(b).Error
}

// IsBlacklisted 判断用户是否在黑名单中（active + 未过期）
func (r *UserBlacklistRepository) IsBlacklisted(ctx context.Context, userID string, platform model.Platform) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var row model.UserBlacklist
	err := r.db.Where("user_id = ? AND platform = ? AND active = ?", userID, platform, true).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if isExpired(&row) {
		// 过期 → 软删除
		_ = r.db.Model(&row).Update("active", false).Error
		return false, nil
	}
	return true, nil
}

// Remove 取消拉黑（软删除：active=false）
func (r *UserBlacklistRepository) Remove(ctx context.Context, userID string, platform model.Platform) error {
	return r.db.Model(&model.UserBlacklist{}).
		Where("user_id = ? AND platform = ? AND active = ?", userID, platform, true).
		Update("active", false).Error
}

// ListActive 查询当前生效的黑名单（active + 未过期）
func (r *UserBlacklistRepository) ListActive(ctx context.Context, page, pageSize int) ([]*model.UserBlacklist, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	var (
		rows  []*model.UserBlacklist
		total int64
	)
	q := r.db.Model(&model.UserBlacklist{}).Where("active = ?", true)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// isExpired 判断黑名单记录是否已过期
// 业务规则：
//   - ExpiresAt 为 nil：永久
//   - ExpiresAt 非 nil 且晚于 now：未过期
//   - ExpiresAt 非 nil 且早于或等于 now：已过期
func isExpired(b *model.UserBlacklist) bool {
	if b == nil {
		return false
	}
	if b.ExpiresAt == nil {
		return false
	}
	return !b.ExpiresAt.After(time.Now())
}
