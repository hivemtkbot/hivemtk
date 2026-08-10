package service

// role.go 角色管理服务
//
// 五层架构归属：L3 业务逻辑层
//
// 设计依据（详见 docs/architecture/MENU_PERMISSION_PLAN.md v3.1 §3.2）：
//   - 角色定义为 model.SystemRoles 常量（不持久化为独立表）
//   - 角色管理 = 只读展示 + 成员数统计 + 成员列表
//   - 业务校验失败返回 fmt.Errorf("...: %w", ErrInvalidInput)
//   - 不调 db，只调 repository
//
// 业务能力：
//   - ListRoles：列出 3 档系统角色（带成员数）
//   - GetRole：按 code 取角色详情
//   - ListMembersByRole：按角色分页查询成员

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// RoleWithCount 角色 + 成员数（DTO）
type RoleWithCount struct {
	model.SystemRole
	MemberCount int64 `json:"member_count"`
}

// RoleService 角色管理服务
type RoleService struct {
	userRepo repository.SystemUserRepository
}

// NewRoleService 构造
func NewRoleService() *RoleService {
	return &RoleService{
		userRepo: repository.NewSystemUserRepository(),
	}
}

// NewRoleServiceWithRepo 注入 repository（便于测试）
func NewRoleServiceWithRepo(repo repository.SystemUserRepository) *RoleService {
	return &RoleService{userRepo: repo}
}

// ListRoles 列出全部系统角色 + 成员数
//
// 业务规则：
//   - 按 model.SystemRoleList 的固定顺序返回（admin → customer_service → staff）
//   - 成员数通过 CountByRole 实时统计
//   - 统计失败时记 0，不阻断列表（部分失败降级）
func (s *RoleService) ListRoles(ctx context.Context) ([]*RoleWithCount, error) {
	roles := make([]*RoleWithCount, 0, len(model.SystemRoleList))
	for _, r := range model.SystemRoleList {
		count, err := s.userRepo.CountByRole(ctx, r.Code)
		if err != nil {
			// 降级：统计失败不阻断列表，展示为 0
			count = 0
		}
		roles = append(roles, &RoleWithCount{
			SystemRole:  r,
			MemberCount: count,
		})
	}
	return roles, nil
}

// GetRole 按 code 取角色详情（带成员数）
//
// 错误：
//   - 角色 code 非法 → ErrInvalidInput
func (s *RoleService) GetRole(ctx context.Context, code string) (*RoleWithCount, error) {
	role := model.GetRoleByCode(code)
	if role == nil {
		return nil, fmt.Errorf("角色不存在: %w", ErrInvalidInput)
	}
	count, err := s.userRepo.CountByRole(ctx, code)
	if err != nil {
		return nil, err
	}
	return &RoleWithCount{
		SystemRole:  *role,
		MemberCount: count,
	}, nil
}

// ListMembersByRole 按角色分页查询成员
//
// 业务规则：
//   - 角色 code 非法 → ErrInvalidInput
//   - page < 1 → 1
//   - size <= 0 → 20，size > 100 → 100
//
// 返回值：成员列表（model.SystemUser）+ 总数
func (s *RoleService) ListMembersByRole(ctx context.Context, code string, page, size int) ([]*model.SystemUser, int64, error) {
	if !model.IsValidRole(code) {
		return nil, 0, fmt.Errorf("角色不存在: %w", ErrInvalidInput)
	}
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	users, total, err := s.userRepo.ListByRole(ctx, code, page, size)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}
