package service

import (
	"errors"
	"regexp"
	"strings"

	"context"
	"marketing/internal/pkg/utils/logger"
)

// SystemInitService 系统初始化服务
// 职责：
//   - 检查 system_users 表是否已有用户
//   - 创建第一个超管（强制首次改密）
//   - 校验密码强度 / 用户名规范
type SystemInitService struct {
	userService *SystemUserService
}

// NewSystemInitService 创建系统初始化服务
func NewSystemInitService() *SystemInitService {
	return &SystemInitService{
		userService: NewSystemUserService(),
	}
}

// CreateInitAdminParams 创建初始化超管参数
type CreateInitAdminParams struct {
	Username string
	Password string
	Email    string
	RealName string
}

// CreateInitAdmin 创建初始化超管
// 规则：
//   - 必须 system_users 表为空（任何用户都没有）
//   - 用户名长度 3-20，字母数字下划线
//   - 密码长度 ≥ 8，必须含大小写字母+数字
//   - 邮箱格式正确
//   - role='admin'，must_change_password=true
func (s *SystemInitService) CreateInitAdmin(ctx context.Context, p *CreateInitAdminParams) error {
	// 1. 系统已有任何用户 → 拒绝
	count, err := s.userService.CountUsers(ctx)
	if err != nil {
		logger.Error(err, "SystemInitService 检查用户总数失败")
		return errors.New("系统状态异常")
	}
	if count > 0 {
		return errors.New("超管已存在，无法重复创建")
	}

	// 2. 校验用户名
	if err := validateUsername(p.Username); err != nil {
		return err
	}

	// 3. 校验密码强度
	if err := validatePassword(p.Password); err != nil {
		return err
	}

	// 4. 校验邮箱
	if err := validateEmail(p.Email); err != nil {
		return err
	}

	// 5. 创建超管
	_, err = s.userService.CreateUser(ctx, &CreateUserRequest{
		Username:           p.Username,
		Password:           p.Password,
		Email:              p.Email,
		RealName:           p.RealName,
		Role:               "admin",
		Status:             1,
		MustChangePassword: true,
	})
	if err != nil {
		logger.Error(err, "SystemInitService 创建超管失败")
		return errors.New("创建超管失败: " + err.Error())
	}
	logger.Info("SystemInitService 创建超管成功: " + p.Username)
	return nil
}

// validateUsername 校验用户名：3-20 字符，字母数字下划线
var usernameRegex = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

func validateUsername(u string) error {
	if strings.TrimSpace(u) == "" {
		return errors.New("用户名不能为空")
	}
	if !usernameRegex.MatchString(u) {
		return errors.New("用户名必须为 3-20 位字母数字下划线")
	}
	return nil
}

// validatePassword 校验密码强度：至少 8 位，含大小写字母 + 数字
var (
	hasLower = regexp.MustCompile(`[a-z]`)
	hasUpper = regexp.MustCompile(`[A-Z]`)
	hasDigit = regexp.MustCompile(`[0-9]`)
)

func validatePassword(p string) error {
	if len(p) < 8 {
		return errors.New("密码长度至少 8 位")
	}
	if !hasLower.MatchString(p) {
		return errors.New("密码必须包含小写字母")
	}
	if !hasUpper.MatchString(p) {
		return errors.New("密码必须包含大写字母")
	}
	if !hasDigit.MatchString(p) {
		return errors.New("密码必须包含数字")
	}
	return nil
}

// validateEmail 简单邮箱格式校验
var emailRegex = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

func validateEmail(e string) error {
	if e == "" {
		return errors.New("邮箱不能为空")
	}
	if !emailRegex.MatchString(e) {
		return errors.New("邮箱格式不正确")
	}
	return nil
}
