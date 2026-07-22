package repository

// password_history_repository.go 密码历史仓储
//
// 五层架构归属：L4 数据访问层
// 表：password_history
// 私域独立部署：无 merchant_id 字段

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// PasswordHistoryRepository 密码历史仓储接口
type PasswordHistoryRepository interface {
	// Create 创建一条历史记录
	Create(ctx context.Context, h *model.PasswordHistory) error

	// ListRecent 查询用户最近 N 条历史（按 changed_at DESC）
	ListRecent(ctx context.Context, userID uint, limit int) ([]model.PasswordHistory, error)

	// Latest 查询用户最近一次密码变更记录
	Latest(ctx context.Context, userID uint) (*model.PasswordHistory, error)
}

type passwordHistoryRepo struct {
	db *gorm.DB
}

// NewPasswordHistoryRepository 构造仓储（内部获取 db）
func NewPasswordHistoryRepository() PasswordHistoryRepository {
	return &passwordHistoryRepo{db: db.GetDB()}
}

// Create 创建一条历史记录
func (r *passwordHistoryRepo) Create(ctx context.Context, h *model.PasswordHistory) error {
	return r.db.WithContext(ctx).Create(h).Error
}

// ListRecent 查询用户最近 N 条历史
func (r *passwordHistoryRepo) ListRecent(ctx context.Context, userID uint, limit int) ([]model.PasswordHistory, error) {
	var list []model.PasswordHistory
	q := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("changed_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Latest 查询用户最近一次密码变更记录
func (r *passwordHistoryRepo) Latest(ctx context.Context, userID uint) (*model.PasswordHistory, error) {
	var h model.PasswordHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("changed_at DESC").
		First(&h).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}

// compile-time 接口断言
var _ PasswordHistoryRepository = (*passwordHistoryRepo)(nil)

// SystemUserRepository 简化版系统用户仓储
//
// 仅供密码策略服务用于读取用户的 created_at/updated_at 字段，
// 不参与其它业务方法，避免和现有 UserRepository（model.User，私域客户）
// 发生命名冲突。
type SystemUserRepository interface {
	// GetUpdatedAt 查询用户的 updated_at（密码过期回退时使用）
	GetUpdatedAt(ctx context.Context, userID uint) (*time.Time, error)
}

type systemUserRepo struct {
	db *gorm.DB
}

// NewSystemUserRepository 构造
func NewSystemUserRepository() SystemUserRepository {
	return &systemUserRepo{db: db.GetDB()}
}

// GetUpdatedAt 查询 updated_at
func (r *systemUserRepo) GetUpdatedAt(ctx context.Context, userID uint) (*time.Time, error) {
	var u model.SystemUser
	if err := r.db.WithContext(ctx).Select("updated_at").First(&u, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	t := u.UpdatedAt
	return &t, nil
}

var _ SystemUserRepository = (*systemUserRepo)(nil)
