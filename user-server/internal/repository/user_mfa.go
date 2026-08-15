package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// UserMFARepository 用户 MFA 仓储接口
//
// 五层架构合规:封装 user_mfa 表的 CRUD,避免 service 层直接调 db.GetDB()。
type UserMFARepository interface {
	GetByUserID(ctx context.Context, userID uint) (*model.UserMFA, error)
	FindByUserID(ctx context.Context, userID uint) (*model.UserMFA, error) 
	Create(ctx context.Context, mfa *model.UserMFA) error
	Update(ctx context.Context, mfa *model.UserMFA) error
	Save(ctx context.Context, mfa *model.UserMFA) error
	UpdateBackupCodes(ctx context.Context, userID uint, backupCodesJSON string) error
	UpdateLastUsed(ctx context.Context, userID uint, code string, now interface{}) error
}

// userMFARepository UserMFA 仓储实现
type userMFARepository struct {
	db *gorm.DB
}

// NewUserMFARepository 创建 UserMFA 仓储实例
func NewUserMFARepository() UserMFARepository {
	return &userMFARepository{db: _db.GetDB()}
}

// NewUserMFARepositoryWithDB 通过 *gorm.DB 创建 UserMFA 仓储实例(用于测试 / router 装配)
func NewUserMFARepositoryWithDB(gormDB *gorm.DB) UserMFARepository {
	return &userMFARepository{db: gormDB}
}

// GetByUserID 根据 userID 获取 UserMFA 记录
// 找不到记录返回 (nil, gorm.ErrRecordNotFound)
func (r *userMFARepository) GetByUserID(ctx context.Context, userID uint) (*model.UserMFA, error) {
	var mfa model.UserMFA
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		return nil, err
	}
	return &mfa, nil
}

// FindByUserID 与 GetByUserID 同义,语义上更接近"查找"
func (r *userMFARepository) FindByUserID(ctx context.Context, userID uint) (*model.UserMFA, error) {
	return r.GetByUserID(ctx, userID)
}

// Create 创建 UserMFA 记录
func (r *userMFARepository) Create(ctx context.Context, mfa *model.UserMFA) error {
	return r.db.WithContext(ctx).Create(mfa).Error
}

// Update 更新整条 UserMFA 记录(基于主键)
func (r *userMFARepository) Update(ctx context.Context, mfa *model.UserMFA) error {
	return r.db.WithContext(ctx).Save(mfa).Error
}

// Save 与 Update 同义
func (r *userMFARepository) Save(ctx context.Context, mfa *model.UserMFA) error {
	return r.db.WithContext(ctx).Save(mfa).Error
}

// UpdateBackupCodes 更新指定用户的备份码(JSON 数组,bcrypt 哈希)
func (r *userMFARepository) UpdateBackupCodes(ctx context.Context, userID uint, backupCodesJSON string) error {
	res := r.db.WithContext(ctx).Model(&model.UserMFA{}).
		Where("user_id = ?", userID).
		Update("backup_codes", backupCodesJSON)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("user_mfa 记录不存在,无法更新 backup_codes")
	}
	return nil
}

// UpdateLastUsed 更新最近一次使用的 TOTP 码及时间
func (r *userMFARepository) UpdateLastUsed(ctx context.Context, userID uint, code string, now interface{}) error {
	return r.db.WithContext(ctx).Model(&model.UserMFA{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_used_code": code,
			"last_used_at":   now,
		}).Error
}

