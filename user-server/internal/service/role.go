package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
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
//   - 统计失败时记 0 + log，不阻断列表（部分失败降级）
//
// v3 审计 P2-14 修复：记 0 掩盖真实错误 → 补 log
func (s *RoleService) ListRoles(ctx context.Context) ([]*RoleWithCount, error) {
	roles := make([]*RoleWithCount, 0, len(model.SystemRoleList))
	for _, r := range model.SystemRoleList {
		count, err := s.userRepo.CountByRole(ctx, r.Code)
		if err != nil {
			// v3 审计 P2-14：记 0 掩盖真实错误
			logger.Warnf("[RoleService] 统计角色 %s 成员数失败，记 0: %v", r.Code, err)
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
