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

// SystemUserRepository 系统用户仓储接口
//
// 提供对 model.SystemUser 的完整 CRUD 与查询。
type SystemUserRepository interface {
	Create(ctx context.Context, u *model.SystemUser) error
	Update(ctx context.Context, u *model.SystemUser) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.SystemUser, error)
	GetByUsername(ctx context.Context, username string) (*model.SystemUser, error)
	List(ctx context.Context, page, pageSize int) ([]*model.SystemUser, int64, error)
	Count(ctx context.Context) (int64, error)
	UsernameExists(ctx context.Context, username string, excludeID uint) (bool, error)
	EmailExists(ctx context.Context, email string, excludeID uint) (bool, error)
	GetFirstAdminUsername(ctx context.Context) (string, error)
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

// Create 创建
func (r *systemUserRepo) Create(ctx context.Context, u *model.SystemUser) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// Update 更新
func (r *systemUserRepo) Update(ctx context.Context, u *model.SystemUser) error {
	return r.db.WithContext(ctx).Save(u).Error
}

// Delete 删除
func (r *systemUserRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.SystemUser{}, id).Error
}

// GetByID 按 ID 查询
func (r *systemUserRepo) GetByID(ctx context.Context, id uint) (*model.SystemUser, error) {
	var u model.SystemUser
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUsername 按用户名查询
func (r *systemUserRepo) GetByUsername(ctx context.Context, username string) (*model.SystemUser, error) {
	var u model.SystemUser
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// List 分页查询
func (r *systemUserRepo) List(ctx context.Context, page, pageSize int) ([]*model.SystemUser, int64, error) {
	var list []*model.SystemUser
	var total int64
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.WithContext(ctx).Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// Count 统计
func (r *systemUserRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UsernameExists 检查用户名是否存在
func (r *systemUserRepo) UsernameExists(ctx context.Context, username string, excludeID uint) (bool, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&model.SystemUser{}).Where("username = ?", username)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// EmailExists 检查邮箱是否存在
func (r *systemUserRepo) EmailExists(ctx context.Context, email string, excludeID uint) (bool, error) {
	if email == "" {
		return false, nil
	}
	var count int64
	q := r.db.WithContext(ctx).Model(&model.SystemUser{}).Where("email = ?", email)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetFirstAdminUsername 取首个超管用户名（按 id 升序）
func (r *systemUserRepo) GetFirstAdminUsername(ctx context.Context) (string, error) {
	var u model.SystemUser
	if err := r.db.WithContext(ctx).Where("role = ?", "admin").Order("id ASC").First(&u).Error; err != nil {
		return "", err
	}
	return u.Username, nil
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
