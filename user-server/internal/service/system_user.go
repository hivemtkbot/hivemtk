package service

import (
	"errors"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username           string `json:"username" binding:"required"`
	Password           string `json:"password" binding:"required"`
	Email              string `json:"email"`
	RealName           string `json:"real_name"`
	Role               string `json:"role" binding:"required,oneof=admin user"`
	Status             int    `json:"status"`
	MustChangePassword bool   `json:"must_change_password"` // 首次登录是否必须改密
}

// SystemUserResponse 系统用户响应
// (定义在 auth.go 中)

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	RealName string `json:"real_name"`
	Role     string `json:"role" binding:"omitempty,oneof=admin user"`
	Status   int    `json:"status"`
}

// SystemUserService 系统用户服务
type SystemUserService struct{}

// NewSystemUserService 创建系统用户服务实例
func NewSystemUserService() *SystemUserService {
	return &SystemUserService{}
}

// CountUsers 统计系统用户数量
func (s *SystemUserService) CountUsers() (int64, error) {
	var count int64
	if err := db.GetDB().Model(&model.SystemUser{}).Count(&count).Error; err != nil {
		logger.Error(err, "统计用户数量失败")
		return 0, err
	}
	return count, nil
}

// GetFirstAdminUsername 取系统中第一个超管用户名（按创建时间升序）
// 用于"半初始化"自愈：旧构建 init-admin 把超管写库却未回写 install.lock，
// 导致 HasInstallLockAdmin()=false、init-complete 永远报"请先创建超管"。
// 此时从 DB 取出已有超管用户名，补写 install.lock，使初始化流程可继续。
func (s *SystemUserService) GetFirstAdminUsername() (string, error) {
	var user model.SystemUser
	if err := db.GetDB().
		Where("role = ?", "admin").
		Order("created_at ASC").
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		logger.Error(err, "查询首个超管失败")
		return "", err
	}
	return user.Username, nil
}

// GetUsers 获取用户列表
func (s *SystemUserService) GetUsers(page, pageSize int) ([]*SystemUserResponse, int64, error) {
	// 获取数据库连接
	database := db.GetDB()

	// 计算总数
	var total int64
	if err := database.Model(&model.SystemUser{}).Count(&total).Error; err != nil {
		logger.Error(err, "获取用户总数失败")
		return nil, 0, errors.New("获取用户列表失败")
	}

	// 查询用户列表
	var users []model.SystemUser
	offset := (page - 1) * pageSize
	if err := database.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users).Error; err != nil {
		logger.Error(err, "查询用户列表失败")
		return nil, 0, errors.New("获取用户列表失败")
	}

	// 转换为响应格式
	var responses []*SystemUserResponse
	for _, user := range users {
		responses = append(responses, s.toUserResponse(&user))
	}

	return responses, total, nil
}

// GetUserByID 根据ID获取用户
func (s *SystemUserService) GetUserByID(id uint) (*SystemUserResponse, error) {
	// 获取数据库连接
	database := db.GetDB()

	// 查询用户
	var user model.SystemUser
	if err := database.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return nil, errors.New("获取用户信息失败")
	}

	return s.toUserResponse(&user), nil
}

// CreateUser 创建用户（P1-2 修复：合并双套创建逻辑，统一走 system_user.go）
// 规则：
//   - 必须校验用户名、密码强度、邮箱格式（避免从 controller 旁路）
//   - username/email 全局唯一
//   - role 仅限 admin/user
//   - status 默认 1（启用）
//   - 不允许 system_init 之外的入口直接创建 admin（admin 必须通过初始化流程）
//     但保留 admin 角色作为合法值，便于 system_init 流程
func (s *SystemUserService) CreateUser(req *CreateUserRequest) (*SystemUserResponse, error) {
	// 1. 校验用户名
	if err := validateUsername(req.Username); err != nil {
		return nil, err
	}
	// 2. 校验密码强度（仅当提供了密码时）
	if req.Password != "" {
		if err := validatePassword(req.Password); err != nil {
			return nil, err
		}
	}
	// 3. 校验邮箱
	if req.Email != "" {
		if err := validateEmail(req.Email); err != nil {
			return nil, err
		}
	}
	// 4. 角色白名单
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		return nil, errors.New("角色非法，仅支持 admin/user")
	}

	database := db.GetDB()

	// 5. 检查用户名是否已存在
	var count int64
	if err := database.Model(&model.SystemUser{}).Where("username = ?", req.Username).Count(&count).Error; err != nil {
		logger.Error(err, "检查用户名失败")
		return nil, errors.New("创建用户失败")
	}
	if count > 0 {
		return nil, errors.New("用户名已存在")
	}

	// 6. 检查邮箱是否已存在
	if req.Email != "" {
		if err := database.Model(&model.SystemUser{}).Where("email = ?", req.Email).Count(&count).Error; err != nil {
			logger.Error(err, "检查邮箱失败")
			return nil, errors.New("创建用户失败")
		}
		if count > 0 {
			return nil, errors.New("邮箱已被使用")
		}
	}

	// 7. 创建用户
	user := model.SystemUser{
		Username:           req.Username,
		Password:           req.Password, // BeforeCreate 钩子会自动加密
		Email:              req.Email,
		RealName:           req.RealName,
		Role:               req.Role,
		Status:             req.Status,
		MustChangePassword: req.MustChangePassword,
	}
	if user.Status == 0 {
		user.Status = 1 // 默认启用
	}

	if err := database.Create(&user).Error; err != nil {
		logger.Error(err, "创建用户失败")
		return nil, errors.New("创建用户失败")
	}

	return s.toUserResponse(&user), nil
}

// UpdateUser 更新用户
func (s *SystemUserService) UpdateUser(id uint, req *UpdateUserRequest) (*SystemUserResponse, error) {
	// 获取数据库连接
	database := db.GetDB()

	// 查询用户
	var user model.SystemUser
	if err := database.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return nil, errors.New("更新用户失败")
	}

	// 更新字段
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Phone != "" {
		user.Phone = req.Phone
	}
	if req.RealName != "" {
		user.RealName = req.RealName
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Status != 0 {
		user.Status = req.Status
	}

	// 保存用户
	if err := database.Save(&user).Error; err != nil {
		logger.Error(err, "保存用户失败")
		return nil, errors.New("更新用户失败")
	}

	return s.toUserResponse(&user), nil
}

// DeleteUser 删除用户
func (s *SystemUserService) DeleteUser(id uint) error {
	// 获取数据库连接
	database := db.GetDB()

	// 检查用户是否存在
	var user model.SystemUser
	if err := database.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return errors.New("删除用户失败")
	}

	// 删除用户
	if err := database.Delete(&user).Error; err != nil {
		logger.Error(err, "删除用户失败")
		return errors.New("删除用户失败")
	}

	return nil
}

// ResetPassword 重置密码
func (s *SystemUserService) ResetPassword(id uint, newPassword string) error {
	// 获取数据库连接
	database := db.GetDB()

	// 查询用户
	var user model.SystemUser
	if err := database.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return errors.New("重置密码失败")
	}

	// 更新密码
	user.Password = newPassword
	if err := user.HashPassword(); err != nil {
		logger.Error(err, "密码加密失败")
		return errors.New("重置密码失败")
	}

	// 保存用户
	if err := database.Save(&user).Error; err != nil {
		logger.Error(err, "保存用户失败")
		return errors.New("重置密码失败")
	}

	return nil
}

// toUserResponse 转换为用户响应
func (s *SystemUserService) toUserResponse(user *model.SystemUser) *SystemUserResponse {
	return &SystemUserResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Email:              user.Email,
		Phone:              user.Phone,
		RealName:           user.RealName,
		Role:               user.Role,
		Status:             user.Status,
		LastLogin:          user.LastLogin,
		LastLoginAt:        user.LastLogin,
		MustChangePassword: user.MustChangePassword,
		CreatedAt:          user.CreatedAt,
		UpdatedAt:          user.UpdatedAt,
	}
}
