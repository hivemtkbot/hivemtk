package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// TeamUserRepository 团队用户仓库接口
// 独立部署版本：移除 merchantID 作用域参数，团队用户全局唯一。
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context，
// 配合 r.db.WithContext(ctx) 透传 trace / timeout / cancel。
type TeamUserRepository interface {
	Create(ctx context.Context, user *model.TeamUser) error
	GetByID(ctx context.Context, id uint) (*model.TeamUser, error)
	GetByUsername(ctx context.Context, username string) (*model.TeamUser, error)
	GetAll(ctx context.Context, page, pageSize int) ([]*model.TeamUser, int64, error)
	Update(ctx context.Context, user *model.TeamUser) error
	Delete(ctx context.Context, id uint) error
	UpdateLastLogin(ctx context.Context, id uint, ip string) error
	UsernameExists(ctx context.Context, username string, excludeID uint) (bool, error)
	EmailExists(ctx context.Context, email string, excludeID uint) (bool, error)
	Count(ctx context.Context) (int64, error)
	CountByRole(ctx context.Context, role string) (int64, error)
}

type teamUserRepo struct {
	db *gorm.DB
}

// NewTeamUserRepository 创建团队用户仓库实例
func NewTeamUserRepository() TeamUserRepository {
	return &teamUserRepo{db: _db.GetDB()}
}

func (r *teamUserRepo) Create(ctx context.Context, user *model.TeamUser) error {
	// 使用 Omit 来确保所有字段都被插入，包括零值
	// GORM 的 default 标签会导致零值被忽略，因此需要显式插入
	return r.db.WithContext(ctx).Omit("CreatedAt", "UpdatedAt").Create(user).Error
}

func (r *teamUserRepo) GetByID(ctx context.Context, id uint) (*model.TeamUser, error) {
	var user model.TeamUser
	err := r.db.WithContext(ctx).First(&user, id).Error
	return &user, err
}

func (r *teamUserRepo) GetByUsername(ctx context.Context, username string) (*model.TeamUser, error) {
	var user model.TeamUser
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *teamUserRepo) GetAll(ctx context.Context, page, pageSize int) ([]*model.TeamUser, int64, error) {
	var users []*model.TeamUser
	var total int64

	if err := r.db.WithContext(ctx).Model(&model.TeamUser{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := r.db.WithContext(ctx).Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users).Error
	return users, total, err
}

func (r *teamUserRepo) Update(ctx context.Context, user *model.TeamUser) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *teamUserRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.TeamUser{}, id).Error
}

func (r *teamUserRepo) UpdateLastLogin(ctx context.Context, id uint, ip string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.TeamUser{}).Where("id = ?", id).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	}).Error
}

func (r *teamUserRepo) UsernameExists(ctx context.Context, username string, excludeID uint) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&model.TeamUser{}).Where("username = ?", username)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (r *teamUserRepo) EmailExists(ctx context.Context, email string, excludeID uint) (bool, error) {
	if email == "" {
		return false, nil
	}
	var count int64
	query := r.db.WithContext(ctx).Model(&model.TeamUser{}).Where("email = ?", email)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (r *teamUserRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TeamUser{}).Count(&count).Error
	return count, err
}

func (r *teamUserRepo) CountByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TeamUser{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

// TeamRoleRepository 团队角色仓库接口
// 独立部署版本：移除 merchantID 作用域参数，团队角色全局唯一。
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context。
type TeamRoleRepository interface {
	Create(ctx context.Context, role *model.TeamRole) error
	GetByID(ctx context.Context, id uint) (*model.TeamRole, error)
	GetByCode(ctx context.Context, code string) (*model.TeamRole, error)
	GetSystemRoles(ctx context.Context) ([]*model.TeamRole, error)
	Update(ctx context.Context, role *model.TeamRole) error
	Delete(ctx context.Context, id uint) error
}

type teamRoleRepo struct {
	db *gorm.DB
}

// NewTeamRoleRepository 创建团队角色仓库实例
func NewTeamRoleRepository() TeamRoleRepository {
	return &teamRoleRepo{db: _db.GetDB()}
}

func (r *teamRoleRepo) Create(ctx context.Context, role *model.TeamRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *teamRoleRepo) GetByID(ctx context.Context, id uint) (*model.TeamRole, error) {
	var role model.TeamRole
	err := r.db.WithContext(ctx).First(&role, id).Error
	return &role, err
}

func (r *teamRoleRepo) GetByCode(ctx context.Context, code string) (*model.TeamRole, error) {
	var role model.TeamRole
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&role).Error
	return &role, err
}

func (r *teamRoleRepo) GetByMerchantAndCode(ctx context.Context, code string) (*model.TeamRole, error) {
	var role model.TeamRole
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&role).Error
	return &role, err
}

func (r *teamRoleRepo) GetSystemRoles(ctx context.Context) ([]*model.TeamRole, error) {
	var roles []*model.TeamRole
	err := r.db.WithContext(ctx).Order("is_system DESC, created_at ASC").Find(&roles).Error
	return roles, err
}

func (r *teamRoleRepo) Update(ctx context.Context, role *model.TeamRole) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *teamRoleRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.TeamRole{}, id).Error
}

