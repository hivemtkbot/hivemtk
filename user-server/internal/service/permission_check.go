package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"hivemtk-user/internal/pkg/utils/logger"
)

// SystemUser 角色常量（与 system_users.role CHECK 约束对齐）
const (
	SystemUserRoleAdmin           = "admin"
	SystemUserRoleCustomerService = "customer_service"
	SystemUserRoleStaff           = "staff"
)

// 兼容旧 TeamUser 角色名（过渡期使用）
const (
	LegacyTeamUserRoleAdmin   = "admin"
	LegacyTeamUserRoleManager = "manager"
	LegacyTeamUserRoleViewer  = "viewer"
)

// ErrServicePermissionDenied Service 层权限不足
var ErrServicePermissionDenied = errors.New("无权限执行此操作")

// ErrServiceRoleMissing Service 层缺少角色信息
var ErrServiceRoleMissing = errors.New("缺少操作者角色信息")

// IsValidSystemUserRoleCode 校验 system_user 角色是否合法
// 严格白名单：admin / customer_service / staff 三档
// 注意：manager / viewer 是 team_user 角色，不在此列
func IsValidSystemUserRoleCode(role string) bool {
	switch role {
	case SystemUserRoleAdmin, SystemUserRoleCustomerService, SystemUserRoleStaff:
		return true
	}
	return false
}

// RequireRole 要求操作者具备指定角色之一（任一即可）
//   - 严格模式：operatorRole 为空直接拒绝（防止绕过）
//   - 多角色匹配：传入的 allowed 任意一个满足即可
func RequireRole(operatorRole string, allowed ...string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	for _, r := range allowed {
		if operatorRole == r {
			return nil
		}
	}
	return ErrServicePermissionDenied
}

// RequireAdmin 要求操作者是 admin
func RequireAdmin(operatorRole string) error {
	return RequireRole(operatorRole, SystemUserRoleAdmin)
}

// RequireManagerOrAdmin 要求操作者是 manager/admin/customer_service 之一
//
// system_users.role 仅有 3 档，customer_service 视为可执行"经理级"操作。
// 兼容：旧 manager 角色 token 仍能通过（按历史契约）。
func RequireManagerOrAdmin(operatorRole string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	switch operatorRole {
	case SystemUserRoleAdmin, SystemUserRoleCustomerService, LegacyTeamUserRoleManager:
		return nil
	default:
		return ErrServicePermissionDenied
	}
}

// RequireNotViewer 要求操作者不是 viewer/staff（仅 admin/manager/customer_service 可执行）
func RequireNotViewer(operatorRole string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	if operatorRole == LegacyTeamUserRoleViewer || operatorRole == SystemUserRoleStaff {
		return ErrServicePermissionDenied
	}
	return nil
}

// RequirePermission 要求操作者具备指定权限
//   - 使用 PermissionService.CheckPermission
//   - 管理员（admin）默认放行
//
// # RequirePermission 校验操作者是否拥有指定权限
//
// 提示：旧版用 context.Background() 会丢失调用方 trace / cancel。
// v3 审计 P1-26 修复：必须由调用方显式传 ctx；如确有不需要 trace 的场景，
// 调用方传 context.Background() 即可（不再"自动"丢失）。
func RequirePermission(ctx context.Context, operatorRole, permission string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	ps := NewPermissionService()
	if !ps.CheckPermission(ctx, operatorRole, permission) {
		return ErrServicePermissionDenied
	}
	return nil
}

// IsWriteRole 判断角色是否具备写权限
//   - admin/customer_service 可写
//   - staff 仅读
func IsWriteRole(role string) bool {
	return role == SystemUserRoleAdmin || role == SystemUserRoleCustomerService
}

// IsReadOnlyRole 判断角色是否仅可读
func IsReadOnlyRole(role string) bool {
	return role == SystemUserRoleStaff || role == LegacyTeamUserRoleViewer
}

// AssertCanOperateSystemUser 断言操作者有权操作 SystemUser
// 规则：
//   - 仅 admin 可以创建/删除/重置其他 SystemUser
//   - customer_service/manager 可以更新其他人的基本资料（除角色外）
//   - staff/viewer 不能操作其他用户
//
// operatorID: 操作者 ID
// operatorRole: 操作者角色
// targetID: 目标用户 ID（0 表示创建）
// action: 操作类型（create/update/delete/reset_password/change_password）
func AssertCanOperateSystemUser(operatorID uint, operatorRole, action string, targetID uint) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	if operatorRole == SystemUserRoleStaff || operatorRole == LegacyTeamUserRoleViewer {
		return ErrServicePermissionDenied
	}
	if operatorRole == SystemUserRoleAdmin {
		return nil
	}
	if operatorRole == SystemUserRoleCustomerService || operatorRole == LegacyTeamUserRoleManager {
		switch action {
		case "update":
			return nil
		default:
			return ErrServicePermissionDenied
		}
	}
	return ErrServicePermissionDenied
}

// PermissionService 权限服务
//
// 所有方法第一参数为 ctx context.Context。
type PermissionService struct{}

// NewPermissionService 创建权限服务实例
func NewPermissionService() *PermissionService {
	return &PermissionService{}
}

// CheckPermission 检查权限
// 独立部署版本：仅根据 role 检查权限，移除 merchantID 作用域参数。
//
// D13：权限矩阵外置——优先读 ConfigParam "permission.role_permissions_json"
//（运营可改，TTL 60s 生效）；admin 恒全权不走配置（防自锁）；
// 解析失败回退内置 defaultRolePermissions（fail-safe）；
// role 不在配置表 = 运营显式删除 → fail-closed 拒绝（不回退内置，否则复活被删权限）；
// 非 admin 角色的 "*" 项解析时剥除并告警（防整体提权）。
func (s *PermissionService) CheckPermission(ctx context.Context, roleCode, permission string) bool {
	if roleCode == SystemUserRoleAdmin || roleCode == LegacyTeamUserRoleAdmin {
		return true
	}

	if cfg := GlobalConfigParam(); cfg != nil {
		if rolePerms, ok := cfg.rolePermissionsFor(ctx, roleCode); ok {
			if rolePerms == nil {
				// role 显式不在配置表 → fail-closed
				logger.Ctx(ctx).Warn().Str("role", roleCode).
					Msg("[Permission] role missing from external matrix, denied (fail-closed)")
				return false
			}
			return matchPermission(rolePerms, permission)
		}
	}

	rolePerms := defaultRolePermissions(roleCode)
	if rolePerms == nil {
		return false
	}
	return matchPermission(rolePerms, permission)
}

// rolePermissionsFor 从配置读取角色权限列表。
// 返回 (nil, true)  = 配置表存在但该角色未列 → fail-closed；
// 返回 (nil, false) = 配置未就绪（空串/坏 JSON）→ 调用方回退内置；
// 返回 (list, true) = 配置生效。
func (s *ConfigParamService) rolePermissionsFor(ctx context.Context, roleCode string) ([]string, bool) {
	raw := s.GetString(ctx, "permission", "role_permissions_json", "")
	if strings.TrimSpace(raw) == "" {
		return nil, false // 未配置 → 内置
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[Permission] role_permissions_json parse failed, fallback to builtin")
		return nil, false
	}
	perms, ok := m[roleCode]
	if !ok {
		return nil, true // 显式缺失 → fail-closed
	}
	// 安全：strip 非 admin 角色的 "*"（防整体提权）
	filtered := make([]string, 0, len(perms))
	for _, p := range perms {
		if p == "*" {
			logger.Ctx(ctx).Warn().Str("role", roleCode).
				Msg("[Permission] wildcard '*' stripped from non-admin role (security)")
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, true
}

// defaultRolePermissions 返回内置角色权限列表
// 取自原 model.SystemRoles 中 3 个内置角色的 Permissions JSON。
func defaultRolePermissions(roleCode string) []string {
	switch roleCode {
	case SystemUserRoleAdmin:
		return []string{"*"}
	case SystemUserRoleCustomerService, LegacyTeamUserRoleManager:
		return []string{
			"cards.*", "shortlinks.*", "clues.*", "autoreply.*",
		}
	case SystemUserRoleStaff, LegacyTeamUserRoleViewer:
		return []string{
			"cards.view", "shortlinks.view", "clues.view",
		}
	default:
		return nil
	}
}

// matchPermission 权限匹配（支持通配符 *.x）
func matchPermission(allowed []string, target string) bool {
	for _, p := range allowed {
		if p == "*" {
			return true
		}
		if p == target {
			return true
		}
		if strings.HasSuffix(p, ".*") {
			prefix := strings.TrimSuffix(p, ".*")
			if strings.HasPrefix(target, prefix+".") {
				return true
			}
		}
	}
	return false
}

// GetUserPermissions 获取用户的所有权限
// 独立部署版本：移除 merchantID 作用域参数。
func (s *PermissionService) GetUserPermissions(ctx context.Context, roleCode string) ([]string, error) {
	if roleCode == SystemUserRoleAdmin {
		return []string{"*"}, nil
	}
	if perms := defaultRolePermissions(roleCode); perms != nil {
		return perms, nil
	}
	return nil, errors.New("角色权限未定义")
}
