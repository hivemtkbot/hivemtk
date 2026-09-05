package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ErrSSOIdentityNotFound SSO 身份记录不存在
var ErrSSOIdentityNotFound = errors.New("sso: identity not found")

type SSOIdentityRepository interface {
	Create(ctx context.Context, identity *model.SSOIdentity) error
	GetByProviderSubject(ctx context.Context, provider, subject string) (*model.SSOIdentity, error)
	ListByUserID(ctx context.Context, userID uint) ([]*model.SSOIdentity, error)
	Delete(ctx context.Context, id uint) error
}

type ssoIdentityRepo struct {
	db *gorm.DB
}

// NewSSOIdentityRepository 构造
func NewSSOIdentityRepository(db *gorm.DB) SSOIdentityRepository {
	return &ssoIdentityRepo{db: db}
}

func (r *ssoIdentityRepo) Create(ctx context.Context, identity *model.SSOIdentity) error {
	return r.db.WithContext(ctx).Create(identity).Error
}

func (r *ssoIdentityRepo) GetByProviderSubject(ctx context.Context, provider, subject string) (*model.SSOIdentity, error) {
	var identity model.SSOIdentity
	if err := r.db.WithContext(ctx).
		Where("provider = ? AND subject = ?", provider, subject).
		First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSSOIdentityNotFound
		}
		return nil, err
	}
	return &identity, nil
}

func (r *ssoIdentityRepo) ListByUserID(ctx context.Context, userID uint) ([]*model.SSOIdentity, error) {
	var list []*model.SSOIdentity
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *ssoIdentityRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.SSOIdentity{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSSOIdentityNotFound
	}
	return nil
}

var _ SSOIdentityRepository = (*ssoIdentityRepo)(nil)
