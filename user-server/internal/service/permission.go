package service


import (
	"context"
	"errors"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// AuthorizationService 授权管理服务
//
// 与 service/permission_check.go 中 PermissionService 命名区分：
//   - PermissionService：业务权限点判断（频道权限/资源权限）
//   - AuthorizationService：admin 授权操作（启停/改密/审计）
type AuthorizationService struct {
	userRepo     repository.SystemUserRepository
	auditService *OperationLogService
}

// NewAuthorizationService 构造
func NewAuthorizationService() *AuthorizationService {
	return &AuthorizationService{
		userRepo:     repository.NewSystemUserRepository(),
		auditService: NewOperationLogService(),
	}
}

// NewAuthorizationServiceWithRepo 注入 repository（便于测试）
func NewAuthorizationServiceWithRepo(userRepo repository.SystemUserRepository, audit *OperationLogService) *AuthorizationService {
	return &AuthorizationService{userRepo: userRepo, auditService: audit}
}

// SetEnabled 启用/禁用账号
//
// 业务规则（v3.1 §3.4）：
//   - 不能启停自己（actorID == targetID → 拒绝）
//   - 禁用 admin 时系统至少保留 1 个启用超管（CountEnabledAdmins 守卫）
//   - 启用操作不受数量限制（启用总是安全的）
//   - 操作后自动写审计日志（module=role, action=user.enable / user.disable）
//
// 错误：
//   - 业务校验失败 → ErrInvalidInput
//   - 目标不存在 → ErrInvalidInput（不泄露存在性）
//   - 系统级错误 → 原样返回
func (s *AuthorizationService) SetEnabled(ctx context.Context, actorID, targetID uint, enabled bool) error {
	if actorID == targetID {
		return fmt.Errorf("不能启停自己的账号: %w", ErrInvalidInput)
	}

	target, err := s.userRepo.GetByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在: %w", ErrInvalidInput)
		}
		return err
	}

	if !enabled && target.Role == model.SystemUserRoleAdmin {
		enabledCount, err := s.userRepo.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if enabledCount <= 1 {
			return fmt.Errorf("系统至少需要保留一个启用的超管账号: %w", ErrInvalidInput)
		}
	}

	if err := s.userRepo.SetEnabled(ctx, targetID, enabled); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在: %w", ErrInvalidInput)
		}
		return err
	}

	action := "user.enable"
	if !enabled {
		action = "user.disable"
	}
	_ = s.writeAuditLog(ctx, actorID, targetID, target.Username, action,
		fmt.Sprintf("actor=%d -> target=%d (%s) enabled=%v", actorID, targetID, target.Username, enabled))

	return nil
}

// ResetPassword admin 重置密码
//
// 业务规则：
//   - 不能修改自己的密码（避免误锁；走 /api/auth/change-password）
//   - 密码强度：至少 8 位 + 大小写字母 + 数字（与 validatePassword 一致）
//   - 操作后自动写审计日志（action=user.reset_password）
func (s *AuthorizationService) ResetPassword(ctx context.Context, actorID, targetID uint, newPassword string) error {
	if actorID == targetID {
		return fmt.Errorf("不能重置自己的密码，请使用修改密码功能: %w", ErrInvalidInput)
	}

	if err := validatePassword(newPassword); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	target, err := s.userRepo.GetByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在: %w", ErrInvalidInput)
		}
		return err
	}

	hashed, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePassword(ctx, targetID, hashed); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在: %w", ErrInvalidInput)
		}
		return err
	}

	_ = s.writeAuditLog(ctx, actorID, targetID, target.Username, "user.reset_password",
		fmt.Sprintf("actor=%d -> target=%d (%s)", actorID, targetID, target.Username))

	return nil
}

// ListAuditLogsRequest 审计日志查询请求
type ListAuditLogsRequest struct {
	UserID   uint   `json:"user_id"`   
	Action   string `json:"action"`    
	Module   string `json:"module"`    
	Page     int    `json:"page"`      
	PageSize int    `json:"page_size"` 
}

// ListAuditLogsResponse 审计日志查询响应
type ListAuditLogsResponse struct {
	Total int64               `json:"total"`
	List  []*OperationLogView `json:"list"`
}

// ListAuditLogs 操作审计日志列表
//
// 默认仅查询 role 模块（授权管理相关）；若 Module 为空，限定 module IN ('role')。
// 分页参数兜底：page < 1 → 1，size <= 0 → 20，size > 100 → 100。
func (s *AuthorizationService) ListAuditLogs(ctx context.Context, req *ListAuditLogsRequest) (*ListAuditLogsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	size := req.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	filters := map[string]any{
		"module": "role", 
	}
	if req.UserID > 0 {
		filters["user_id"] = req.UserID
	}
	if req.Action != "" {
		filters["action"] = req.Action
	} else {
		filters["action"] = "user.%"
		delete(filters, "action")
	}

	logs, total, err := s.auditService.GetAll(ctx, page, size, filters)
	if err != nil {
		return nil, err
	}

	if req.Action != "" {
		filtered := make([]*OperationLogView, 0, len(logs))
		for _, l := range logs {
			if l.Action == req.Action {
				filtered = append(filtered, l)
			}
		}
		logs = filtered
		total = int64(len(logs))
	}

	return &ListAuditLogsResponse{
		Total: total,
		List:  logs,
	}, nil
}

// writeAuditLog 写操作审计日志
//
// 通过 OperationLogRepository.Create 直接落库；不依赖 auditService.Log
// 以避免循环依赖（auditService 内部依赖更多组件）。
func (s *AuthorizationService) writeAuditLog(ctx context.Context, actorID, targetID uint, targetUsername, action, detail string) error {
	log := &model.OperationLog{
		UserID:     actorID,
		Username:   targetUsername,
		Action:     action,
		Module:     "role", 
		Resource:   "system_user",
		ResourceID: fmt.Sprintf("%d", targetID),
		Detail:     detail,
	}
	return repository.NewOperationLogRepository().Create(ctx, log)
}

