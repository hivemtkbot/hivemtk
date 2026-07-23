package repository

// system_user.go 系统用户仓储
//
// 五层架构归属：L4 数据访问层
// 表：system_users
// 私域独立部署：无 merchant_id 字段
//
// 阶段 4（人员管理）扩展：
//   - 新增 ListByRole / CountByRole / CountAdmins
//   - 新增 DeleteSafe（拒绝删除最后一个 admin）
//   - 新增 SetEnabled（启用/禁用账号）
//
// 阶段 2 重构：原 systemUserRepo 寄生在 password_history_repository.go，
// 本文件独立承载 SystemUserRepository 接口与实现，避免混杂。

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
)

// ErrLastAdmin 系统至少需要保留一个 admin 账号
//
// 业务语义：DeleteSafe 在检测到要删除的是 admin 且当前 admin 总数 == 1 时，
// 返回该错误供上层转换为 4xx 响应。
var ErrLastAdmin = errors.New("system: cannot delete the last admin")

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

	// ListByRole 按 role 查询（分页 + 总数），供角色管理模块使用
	ListByRole(ctx context.Context, role string, page, size int) ([]*model.SystemUser, int64, error)
	// CountByRole 统计某角色成员数（角色管理模块使用）
	CountByRole(ctx context.Context, role string) (int64, error)
	// CountAdmins 统计 admin 数量（DeleteSafe 业务前置判断用）
	CountAdmins(ctx context.Context) (int64, error)
	// DeleteSafe 删除账号（拒绝删除最后一个 admin），返回 ErrLastAdmin
	DeleteSafe(ctx context.Context, id uint) error
	// SetEnabled 启用/禁用账号（按 id 写 enabled 字段）
	SetEnabled(ctx context.Context, id uint, enabled bool) error
	// CountEnabledAdmins 统计当前启用的 admin 数量（授权管理模块启停/禁用守卫）
	CountEnabledAdmins(ctx context.Context) (int64, error)
	// UpdatePassword 直接以 bcrypt 密文更新密码（授权管理重置密码）
	UpdatePassword(ctx context.Context, id uint, hashedPassword string) error
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

// ListByRole 按角色分页查询
func (r *systemUserRepo) ListByRole(ctx context.Context, role string, page, size int) ([]*model.SystemUser, int64, error) {
	var list []*model.SystemUser
	var total int64
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	q := r.db.WithContext(ctx).Model(&model.SystemUser{}).Where("role = ?", role)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// CountByRole 统计某角色成员数
func (r *systemUserRepo) CountByRole(ctx context.Context, role string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Where("role = ?", role).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountAdmins 统计 admin 数量
func (r *systemUserRepo) CountAdmins(ctx context.Context) (int64, error) {
	return r.CountByRole(ctx, "admin")
}

// DeleteSafe 删除账号（拒绝删除最后一个 admin）
//
// 业务规则：
//   - 当目标记录不存在时，gorm 会返回 ErrRecordNotFound，由上层处理。
//   - 当目标是 admin 且 admin 总数 == 1，返回 ErrLastAdmin（不执行删除）。
//   - 其它情况正常删除。
func (r *systemUserRepo) DeleteSafe(ctx context.Context, id uint) error {
	var target model.SystemUser
	if err := r.db.WithContext(ctx).First(&target, id).Error; err != nil {
		return fmt.Errorf("query target user: %w", err)
	}
	if target.Role == "admin" {
		count, err := r.CountAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	if err := r.db.WithContext(ctx).Delete(&model.SystemUser{}, id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// SetEnabled 启用/禁用账号
func (r *systemUserRepo) SetEnabled(ctx context.Context, id uint, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&model.SystemUser{}).
		Where("id = ?", id).
		Update("enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("update enabled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountEnabledAdmins 统计当前启用的 admin 数量
//
// 用于授权管理 service 守卫：
//   - 禁用 admin 前检查 enabledCount > 1（系统至少保留 1 个启用超管）
//   - 启用 admin 时无需检查（启用总是安全的）
func (r *systemUserRepo) CountEnabledAdmins(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.SystemUser{}).
		Where("role = ? AND enabled = ?", model.SystemUserRoleAdmin, true).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count enabled admins: %w", err)
	}
	return n, nil
}

// UpdatePassword 直接以 bcrypt 密文更新密码
//
// 业务规则：
//   - 由 service 层负责 Hash 密码（不在 repository 中加密）
//   - 当目标记录不存在时返回 gorm.ErrRecordNotFound，供 service 转换为 4xx
func (r *systemUserRepo) UpdatePassword(ctx context.Context, id uint, hashedPassword string) error {
	res := r.db.WithContext(ctx).Model(&model.SystemUser{}).
		Where("id = ?", id).
		Update("password", hashedPassword)
	if res.Error != nil {
		return fmt.Errorf("update password: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

var _ SystemUserRepository = (*systemUserRepo)(nil)
