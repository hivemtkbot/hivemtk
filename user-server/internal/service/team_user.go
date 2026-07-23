package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/repository"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// TeamUserService 团队用户服务
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context，透传至 repository 层。
type TeamUserService struct {
	repo		repository.TeamUserRepository
	roleRepo	repository.TeamRoleRepository
	logRepo		repository.OperationLogRepository
	jwtSecret	string
}

// NewTeamUserService 创建团队用户服务实例
func NewTeamUserService() *TeamUserService {
	return &TeamUserService{
		repo:		repository.NewTeamUserRepository(),
		roleRepo:	repository.NewTeamRoleRepository(),
		logRepo:	repository.NewOperationLogRepository(),
		jwtSecret:	getJWTSecret(),
	}
}

// CreateTeamUserRequest 创建用户请求
type CreateTeamUserRequest struct {
	Username	string	`json:"username" binding:"required,min=3,max=50"`
	Password	string	`json:"password" binding:"required,min=6"`
	Name		string	`json:"name"`
	Email		string	`json:"email"`
	Phone		string	`json:"phone"`
	Role		string	`json:"role" binding:"required"`
	Avatar		string	`json:"avatar"`
}

// UpdateTeamUserRequest 更新用户请求
type UpdateTeamUserRequest struct {
	Name		string	`json:"name"`
	Email		string	`json:"email"`
	Phone		string	`json:"phone"`
	Role		string	`json:"role"`
	Avatar		string	`json:"avatar"`
	Status		*int	`json:"status"`
	DataScope	*int	`json:"data_scope,omitempty"`		// A 域 P1-4：数据范围（指针区分 0 值）
	DepartmentID	*uint	`json:"department_id,omitempty"`	// A 域 P1-4：所属部门 ID
	TeamID		*uint	`json:"team_id,omitempty"`		// A 域 P1-4：所属团队 ID
	CustomDeptIDs	string	`json:"custom_dept_ids,omitempty"`	// A 域 P1-4：data_scope=4 时的部门白名单（JSON）
}

// TeamChangePasswordRequest 修改密码请求
type TeamChangePasswordRequest struct {
	OldPassword	string	`json:"old_password" binding:"required"`
	NewPassword	string	`json:"new_password" binding:"required,min=6"`
}

// TeamUserLoginRequest 登录请求
type TeamUserLoginRequest struct {
	Username	string	`json:"username" binding:"required"`
	Password	string	`json:"password" binding:"required"`
}

// TeamUserLoginResponse 登录响应
type TeamUserLoginResponse struct {
	Token	string		`json:"token"`
	User	*model.TeamUser	`json:"user"`
}

// TeamUserListResponse 用户列表响应
type TeamUserListResponse struct {
	List		[]*model.TeamUser	`json:"list"`
	Total		int64			`json:"total"`
	Page		int			`json:"page"`
	PageSize	int			`json:"page_size"`
}

// Create 创建用户
// 独立部署版本：移除 merchantID 作用域，用户名/邮箱全局唯一。
// P1-6 修复：Service 层加入权限断言（仅 admin 可创建 TeamUser）
func (s *TeamUserService) Create(ctx context.Context, req *CreateTeamUserRequest, operatorID uint, operatorRole string, operatorIP string) (*model.TeamUser, error) {
	// P1-6：Service 层权限断言（防止前端绕过/中间件遗漏）
	if err := AssertCanOperateTeamUser(operatorID, operatorRole, "create", 0); err != nil {
		return nil, err
	}

	// 检查用户名是否已存在
	exists, err := s.repo.UsernameExists(ctx, req.Username, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("用户名已存在")
	}

	// 检查邮箱是否已存在
	if req.Email != "" {
		exists, err = s.repo.EmailExists(ctx, req.Email, 0)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("邮箱已被使用")
		}
	}

	// 验证角色（独立部署：role 全局唯一）
	_, err = s.roleRepo.GetByCode(ctx, req.Role)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 角色不存在，检查是否为系统角色
			systemRoles := model.SystemRoles
			isValidRole := false
			for _, r := range systemRoles {
				if r.Code == req.Role {
					isValidRole = true
					break
				}
			}
			if !isValidRole {
				return nil, errors.New("无效的角色")
			}
			// 系统角色无需存在于数据库中
		}
	}

	// 加密密码
	hashedPassword, err := bcrypt.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &model.TeamUser{
		Username:	req.Username,
		Password:	hashedPassword,
		Name:		req.Name,
		Email:		req.Email,
		Phone:		req.Phone,
		Role:		req.Role,
		Avatar:		req.Avatar,
		Status:		model.TeamUserStatusActive,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	// 记录操作日志
	s.logOperation(ctx, operatorID, req.Username, "create", "user", fmt.Sprintf("%d", user.ID), "", user, operatorIP)

	return user, nil
}

// Update 更新用户
// 独立部署版本：移除 merchantID 作用域校验。
// P1-6 修复：Service 层加入权限断言
func (s *TeamUserService) Update(ctx context.Context, userID uint, req *UpdateTeamUserRequest, operatorID uint, operatorRole string, operatorIP string) (*model.TeamUser, error) {
	// P1-6：Service 层权限断言
	if err := AssertCanOperateTeamUser(operatorID, operatorRole, "update", userID); err != nil {
		return nil, err
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	oldUser := *user

	// 更新字段
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" {
		// 检查邮箱是否已被使用
		exists, err := s.repo.EmailExists(ctx, req.Email, userID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("邮箱已被使用")
		}
		user.Email = req.Email
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	if req.Status != nil {
		user.Status = model.TeamUserStatus(*req.Status)
	}

	// A 域 P1-4：行级权限字段（指针类型：nil=不更新，0=显式置 0）
	if req.DataScope != nil {
		user.DataScope = *req.DataScope
	}
	if req.DepartmentID != nil {
		user.DepartmentID = *req.DepartmentID
	}
	if req.TeamID != nil {
		user.TeamID = *req.TeamID
	}
	if req.CustomDeptIDs != "" {
		user.CustomDeptIDs = req.CustomDeptIDs
	} else if req.DataScope != nil && *req.DataScope != int(model.TeamDataScopeCustom) {
		// data_scope 切换到非 custom 时清空白名单
		user.CustomDeptIDs = ""
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	// 记录操作日志
	s.logOperation(ctx, operatorID, user.Username, "update", "user", fmt.Sprintf("%d", user.ID), oldUser, user, operatorIP)

	return user, nil
}

// Delete 删除用户
// 独立部署版本：移除 merchantID 作用域校验。
// P1-6 修复：Service 层加入权限断言
func (s *TeamUserService) Delete(ctx context.Context, userID uint, operatorID uint, operatorRole string, operatorIP string) error {
	// P1-6：Service 层权限断言
	if err := AssertCanOperateTeamUser(operatorID, operatorRole, "delete", userID); err != nil {
		return err
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// 不能删除自己
	if user.ID == operatorID {
		return errors.New("不能删除自己的账户")
	}

	// 不能删除最后一个管理员
	if user.Role == "admin" {
		count, err := s.repo.CountByRole(ctx, "admin")
		if err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("不能删除最后一个管理员")
		}
	}

	if err := s.repo.Delete(ctx, userID); err != nil {
		return err
	}

	// 记录操作日志
	s.logOperation(ctx, operatorID, user.Username, "delete", "user", fmt.Sprintf("%d", userID), user, nil, operatorIP)

	return nil
}

// GetByID 根据ID获取用户
// 独立部署版本：移除 merchantID 校验。
func (s *TeamUserService) GetByID(ctx context.Context, userID uint) (*model.TeamUser, error) {
	return s.repo.GetByID(ctx, userID)
}

// GetList 获取用户列表
// 独立部署版本：不再按 merchant 过滤。
func (s *TeamUserService) GetList(ctx context.Context, page, pageSize int) (*TeamUserListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	users, total, err := s.repo.GetAll(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &TeamUserListResponse{
		List:		users,
		Total:		total,
		Page:		page,
		PageSize:	pageSize,
	}, nil
}

// Login 用户登录
// 独立部署版本：用户名全局唯一，无需 merchantID 限定。
func (s *TeamUserService) Login(ctx context.Context, req *TeamUserLoginRequest, ip string) (*TeamUserLoginResponse, error) {
	user, err := s.repo.GetByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户名或密码错误")
		}
		return nil, err
	}

	// 检查状态
	if user.Status != model.TeamUserStatusActive {
		return nil, errors.New("账户已被禁用")
	}

	// 验证密码
	if bcrypt.CheckPassword(user.Password, req.Password) != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 生成JWT Token
	token, err := s.generateToken(ctx, user)
	if err != nil {
		return nil, err
	}

	// 更新最后登录信息
	_ = s.repo.UpdateLastLogin(ctx, user.ID, ip)

	// 记录登录日志
	s.logOperation(ctx, user.ID, user.Username, "login", "auth", "", nil, nil, ip)

	return &TeamUserLoginResponse{
		Token:	token,
		User:	user,
	}, nil
}

// ChangePassword 修改密码
// 独立部署版本：移除 merchantID 校验。
func (s *TeamUserService) ChangePassword(ctx context.Context, userID uint, req *TeamChangePasswordRequest, ip string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if bcrypt.CheckPassword(user.Password, req.OldPassword) != nil {
		return errors.New("旧密码错误")
	}

	// 加密新密码
	hashedPassword, err := bcrypt.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	user.Password = hashedPassword
	if err := s.repo.Update(ctx, user); err != nil {
		return err
	}

	// 记录操作日志
	s.logOperation(ctx, userID, user.Username, "change_password", "auth", "", nil, nil, ip)

	return nil
}

// ResetPassword 重置密码
// 独立部署版本：移除 merchantID 校验。
// P1-6 修复：Service 层加入权限断言（仅 admin 可重置 TeamUser 密码）
func (s *TeamUserService) ResetPassword(ctx context.Context, userID uint, newPassword string, operatorID uint, operatorRole string, operatorIP string) error {
	// P1-6：Service 层权限断言
	if err := AssertCanOperateTeamUser(operatorID, operatorRole, "reset_password", userID); err != nil {
		return err
	}

	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// 加密新密码
	hashedPassword, err := bcrypt.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.Password = hashedPassword
	if err := s.repo.Update(ctx, user); err != nil {
		return err
	}

	// 记录操作日志
	s.logOperation(ctx, operatorID, user.Username, "reset_password", "user", fmt.Sprintf("%d", userID), nil, nil, operatorIP)

	return nil
}

// generateToken 生成JWT Token
// 独立部署版本：移除 merchant_id claim。
func (s *TeamUserService) generateToken(ctx context.Context, user *model.TeamUser) (string, error) {
	claims := jwt.MapClaims{
		"user_id":	user.ID,
		"username":	user.Username,
		"role":		user.Role,
		"exp":		time.Now().Add(72 * time.Hour).Unix(),
		"iat":		time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// logOperation 记录操作日志
// 独立部署版本：移除 merchantID 参数。
//
// P1-4 改造：优先通过 EventBus 异步发布（解耦、非阻塞），
// 若全局事件总线未初始化（如单元测试环境），则 fallback 到同步直接写入。
//
// 2026-07-22 方向E：新增 ctx 参数透传至底层 repository。
func (s *TeamUserService) logOperation(ctx context.Context, userID uint, username, action, module, resourceID string, oldValue, newValue any, ip string) {
	// 优先使用 EventBus（异步、解耦）
	if event.GetGlobalBus() != nil {
		event.Publish(event.TopicOperationLog, event.OperationLogPayload{
			UserID:		userID,
			Username:	username,
			Action:		action,
			Module:		module,
			Resource:	module,
			ResourceID:	resourceID,
			OldValue:	oldValue,
			NewValue:	newValue,
			IP:		ip,
		})
		return
	}

	// Fallback: 直接同步写入（测试环境或事件总线未初始化时）
	oldValueJSON, _ := json.Marshal(oldValue)
	newValueJSON, _ := json.Marshal(newValue)

	logEntry := &model.OperationLog{
		UserID:		userID,
		Username:	username,
		Action:		action,
		Module:		module,
		Resource:	module,
		ResourceID:	resourceID,
		OldValue:	string(oldValueJSON),
		NewValue:	string(newValueJSON),
		IP:		ip,
	}

	_ = s.logRepo.Create(ctx, logEntry)
}

// getJWTSecret 获取JWT密钥
func getJWTSecret() string {
	// 从配置或环境变量获取
	secret := "your-secret-key"	// 默认值，生产环境应从配置读取
	return secret
}

// TeamRoleService 团队角色服务
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context。
type TeamRoleService struct {
	repo repository.TeamRoleRepository
}

// NewTeamRoleService 创建团队角色服务实例
func NewTeamRoleService() *TeamRoleService {
	return &TeamRoleService{
		repo: repository.NewTeamRoleRepository(),
	}
}

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Code		string		`json:"code" binding:"required"`
	Name		string		`json:"name" binding:"required"`
	Permissions	[]string	`json:"permissions"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	Name		string		`json:"name"`
	Permissions	[]string	`json:"permissions"`
	Status		*int		`json:"status"`
}

// GetList 获取角色列表
// 独立部署版本：返回所有角色（不再按 merchant 隔离）。
func (s *TeamRoleService) GetList(ctx context.Context) ([]*model.TeamRole, error) {
	return s.repo.GetSystemRoles(ctx)
}

// Create 创建角色
// 独立部署版本：role 全局唯一。
func (s *TeamRoleService) Create(ctx context.Context, req *CreateRoleRequest) (*model.TeamRole, error) {
	// 检查编码是否已存在
	_, err := s.repo.GetByCode(ctx, req.Code)
	if err == nil {
		return nil, errors.New("角色编码已存在")
	}

	// 序列化权限
	permissionsJSON, _ := json.Marshal(req.Permissions)

	role := &model.TeamRole{
		Code:		req.Code,
		Name:		req.Name,
		Permissions:	string(permissionsJSON),
		IsSystem:	false,
	}

	if err := s.repo.Create(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}

// Update 更新角色
// 独立部署版本：移除 merchantID 校验。
func (s *TeamRoleService) Update(ctx context.Context, roleID uint, req *UpdateRoleRequest) (*model.TeamRole, error) {
	role, err := s.repo.GetByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	if role.IsSystem {
		return nil, errors.New("系统角色不能修改")
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Permissions != nil {
		permissionsJSON, _ := json.Marshal(req.Permissions)
		role.Permissions = string(permissionsJSON)
	}
	if req.Status != nil {
		role.Status = *req.Status
	}

	if err := s.repo.Update(ctx, role); err != nil {
		return nil, err
	}

	return role, nil
}

// Delete 删除角色
// 独立部署版本：移除 merchantID 校验。
func (s *TeamRoleService) Delete(ctx context.Context, roleID uint) error {
	role, err := s.repo.GetByID(ctx, roleID)
	if err != nil {
		return err
	}

	if role.IsSystem {
		return errors.New("系统角色不能删除")
	}

	return s.repo.Delete(ctx, roleID)
}

// GetPermissions 获取所有权限
func (s *TeamRoleService) GetPermissions(ctx context.Context) map[string]string {
	return model.SystemPermissions
}

// PermissionService 权限服务
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context。
type PermissionService struct {
	roleRepo repository.TeamRoleRepository
}

// NewPermissionService 创建权限服务实例
func NewPermissionService() *PermissionService {
	return &PermissionService{
		roleRepo: repository.NewTeamRoleRepository(),
	}
}

// CheckPermission 检查权限
// 独立部署版本：仅根据 role 检查权限，移除 merchantID 作用域参数。
func (s *PermissionService) CheckPermission(ctx context.Context, roleCode, permission string) bool {
	// 管理员拥有所有权限
	if roleCode == "admin" {
		return true
	}

	// 获取角色
	role, err := s.roleRepo.GetByCode(ctx, roleCode)
	if err != nil {
		// 使用系统默认角色权限
		for _, r := range model.SystemRoles {
			if r.Code == roleCode {
				return s.checkPermissionInList(ctx, r.Permissions, permission)
			}
		}
		return false
	}

	return s.checkPermissionInList(ctx, role.Permissions, permission)
}

// checkPermissionInList 检查权限是否在列表中
func (s *PermissionService) checkPermissionInList(ctx context.Context, permissionsJSON, permission string) bool {
	var permissions []string
	if err := json.Unmarshal([]byte(permissionsJSON), &permissions); err != nil {
		return false
	}

	for _, p := range permissions {
		if p == "*" {
			return true	// 拥有所有权限
		}
		if p == permission {
			return true
		}
		// 支持通配符匹配，如 cards.* 匹配 cards.view, cards.create 等
		if strings.HasSuffix(p, ".*") {
			prefix := strings.TrimSuffix(p, ".*")
			if strings.HasPrefix(permission, prefix+".") {
				return true
			}
		}
	}

	return false
}

// GetUserPermissions 获取用户的所有权限
// 独立部署版本：移除 merchantID 作用域参数。
func (s *PermissionService) GetUserPermissions(ctx context.Context, roleCode string) ([]string, error) {
	if roleCode == "admin" {
		return []string{"*"}, nil
	}

	role, err := s.roleRepo.GetByCode(ctx, roleCode)
	if err != nil {
		// 使用系统默认角色权限
		for _, r := range model.SystemRoles {
			if r.Code == roleCode {
				var permissions []string
				json.Unmarshal([]byte(r.Permissions), &permissions)
				return permissions, nil
			}
		}
		return nil, err
	}

	var permissions []string
	json.Unmarshal([]byte(role.Permissions), &permissions)
	return permissions, nil
}
