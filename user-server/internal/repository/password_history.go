package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// PasswordHistoryRepository 密码历史仓储接口
type PasswordHistoryRepository interface {
	Create(ctx context.Context, h *model.PasswordHistory) error

	ListRecent(ctx context.Context, userID uint, limit int) ([]model.PasswordHistory, error)

	Latest(ctx context.Context, userID uint) (*model.PasswordHistory, error)
}

type passwordHistoryRepo struct {
	db *gorm.DB
}

// NewPasswordHistoryRepository 构造仓储（内部获取 db）
func NewPasswordHistoryRepository() PasswordHistoryRepository {
	return &passwordHistoryRepo{db: db.GetDB()}
}

func (r *passwordHistoryRepo) Create(ctx context.Context, h *model.PasswordHistory) error {
	return r.db.WithContext(ctx).Create(h).Error
}

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

var _ PasswordHistoryRepository = (*passwordHistoryRepo)(nil)
