package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/bcrypt"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/system/install"

	"gorm.io/gorm"
)

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token     string              `json:"token,omitempty"`
	User      *SystemUserResponse `json:"user,omitempty"`
	Expires   int64               `json:"expires,omitempty"`
	NeedMFA   bool                `json:"need_mfa,omitempty"`   // 是否需要 MFA 二次验证
	TempToken string              `json:"temp_token,omitempty"` // 临时令牌（MFA 验证用）
}

// SystemUserResponse 系统用户响应
type SystemUserResponse struct {
	ID          uint       `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	RealName    string     `json:"real_name"`
	Role        string     `json:"role"`
	Status      int        `json:"status"`
	LastLogin   *time.Time `json:"last_login"`
	LastLoginAt *time.Time `json:"last_login_at"` // 兼容前端驼峰命名
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// AuthService 认证服务
type AuthService struct {
	jwtUtils       *utils.JWTUtils
	systemUserRepo repository.SystemUserRepository
}

// NewAuthService 创建认证服务实例
func NewAuthService() *AuthService {
	return &AuthService{
		jwtUtils:       utils.NewJWTUtils(utils.DefaultJWTConfig),
		systemUserRepo: repository.NewSystemUserRepository(),
	}
}

// JwtUtils 获取 JWT 工具实例（用于测试）
func (s *AuthService) JwtUtils(ctx context.Context) *utils.JWTUtils {
	return s.jwtUtils
}

// Login 用户登录
//
// 严格规则（修复）：
//  1. 不再"系统无用户 → 自动注册为超管"——该机制绕过 InitGuard/LicenseGuard，
//     导致未绑 License 即可创建超管，摧毁安全模型。
//  2. 必须先有用户（由 InitSetup 创建）才能登录。
//  3. 用户名/密码错误一律返回"用户名或密码错误"（防枚举）。
//  4. 用户被禁用直接拒绝（明确反馈）。
//  5. 密码用 bcrypt 验证。
//  6. 阶段 1：team_users 已被合并到 system_users，仅查 system_users。
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {

	user, err := s.systemUserRepo.GetByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Error(err, "查询用户失败")
		return nil, errors.New("登录失败，请稍后重试")
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("用户名或密码错误")
	}

	if user.Status != 1 || !user.Enabled {
		return nil, errors.New("用户已被禁用")
	}

	if !CheckPassword(user, req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	return s.loginWithUser(ctx, user)
}

// loginWithUser 使用用户对象完成登录流程
func (s *AuthService) loginWithUser(ctx context.Context, user *model.SystemUser) (*LoginResponse, error) {
	// 修复：MFA 启用检查
	// 用户名密码验证 OK 后，若启用了 MFA，则返回 need_mfa + temp_token，等待二次验证
	mfaSvc := NewMFAService()
	mfaEnabled, err := mfaSvc.IsMFAEnabled(ctx, user.ID)
	if err != nil {
		logger.Errorf("MFA 状态查询失败: %v", err)
	}
	if mfaEnabled {
		tempToken, err := mfaSvc.IssueTempToken(ctx, user.ID, user.Username, user.Role)
		if err != nil {
			return nil, errors.New("登录失败，请稍后重试")
		}
		return &LoginResponse{
			NeedMFA:   true,
			TempToken: tempToken,
		}, nil
	}

	token, err := s.jwtUtils.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		logger.Error(err, "生成JWT令牌失败")
		return nil, errors.New("登录失败，请稍后重试")
	}

	// 更新最后登录时间
	now := time.Now()
	user.LastLogin = &now
	if err := s.systemUserRepo.Update(ctx, user); err != nil {
		logger.Error(err, "更新用户最后登录时间失败")
		// 不影响登录流程，只记录日志
	}

	// 构建响应
	response := &LoginResponse{
		Token:   token,
		User:    s.toUserResponse(ctx, user),
		Expires: time.Now().Add(time.Hour * time.Duration(utils.DefaultJWTConfig.ExpiresHours)).Unix(),
	}

	return response, nil
}

// RefreshToken 刷新令牌
func (s *AuthService) RefreshToken(ctx context.Context, tokenString string) (string, error) {
	// 刷新令牌
	newToken, err := s.jwtUtils.RefreshToken(tokenString)
	if err != nil {
		return "", err
	}

	return newToken, nil
}

// GetCurrentUser 获取当前用户信息
func (s *AuthService) GetCurrentUser(ctx context.Context, userID uint) (*SystemUserResponse, error) {
	// 查找用户
	user, err := s.systemUserRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return nil, errors.New("获取用户信息失败")
	}

	return s.toUserResponse(ctx, user), nil
}

// ChangePassword 修改密码
// 修复：强制走密码策略校验 + 记录历史
func (s *AuthService) ChangePassword(ctx context.Context, userID uint, req *ChangePasswordRequest) error {
	// 查找用户
	user, err := s.systemUserRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在")
		}
		logger.Error(err, "查询用户失败")
		return errors.New("修改密码失败")
	}

	// 验证旧密码
	if !CheckPassword(user, req.OldPassword) {
		return errors.New("原密码不正确")
	}

	// 修复：强制走密码策略校验
	policySvc := NewPasswordPolicyService()
	if err := policySvc.ValidatePassword(ctx, req.NewPassword, userID); err != nil {
		return err
	}

	// 更新密码
	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		logger.Error(err, "密码加密失败")
		return errors.New("修改密码失败")
	}
	user.Password = hashed

	// 保存用户
	if err := s.systemUserRepo.Update(ctx, user); err != nil {
		logger.Error(err, "保存用户失败")
		return errors.New("修改密码失败")
	}

	// 修复：记录密码历史
	if err := policySvc.RecordPasswordHistory(ctx, userID, req.NewPassword, model.PasswordSourceChangePassword); err != nil {
		logger.Errorf("记录密码历史失败（不影响改密流程）: %v", err)
	}

	return nil
}

// toUserResponse 转换为用户响应
func (s *AuthService) toUserResponse(ctx context.Context, user *model.SystemUser) *SystemUserResponse {
	return &SystemUserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Phone:       user.Phone,
		RealName:    user.RealName,
		Role:        user.Role,
		Status:      user.Status,
		LastLogin:   user.LastLogin,
		LastLoginAt: user.LastLogin,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

// ============== 包级辅助函数（五层架构：业务方法集中在 service 层） ==============

// CheckPassword 校验 SystemUser 密码
func CheckPassword(u *model.SystemUser, password string) bool {
	return model.CheckSystemUserPassword(u, password)
}

// HashPassword 对明文密码进行 bcrypt 哈希
func HashPassword(password string) (string, error) {
	return bcrypt.HashPassword(password)
}

// ============== 阶段 3：系统初始化 InitAdmin ==============

// InitAdmin 初始化系统首个超管（公开，无 JWT）
//
// 系统用户统一 plan v3.1 §3.2：
//   - 调用方必须在请求体中传入 username/password/email（不再读 config 默认值）
//   - 密码强度：至少 8 位，含大小写字母 + 数字
//   - username 唯一性、email 唯一性（防重复初始化由路由层 install.lock 闸负责，见 admin_routes.go）
//   - 创建后写 install.lock（AdminUsername + Initialized=true），作为"已初始化"标记
//
// 失败语义：
//   - 已初始化（install.lock 存在）→ 由路由层返回 403 禁止重复创建，不到达本方法
//   - 密码强度不达标 → 复用 service.validatePassword 返回的错误
//   - username/email 冲突 → 透传 repo 错误
func (s *AuthService) InitAdmin(ctx context.Context, username, password, email string) error {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if username == "" || password == "" || email == "" {
		return errors.New("username/password/email 均不能为空")
	}

	// 密码强度校验（与 system_init.go 一致：≥8 位 + 大小写 + 数字）
	if err := validatePassword(password); err != nil {
		return err
	}

	// 3. 邮箱格式校验
	if err := validateEmail(email); err != nil {
		return err
	}

	// 4. 用户名格式校验（3-20 位字母数字下划线）
	if err := validateUsername(username); err != nil {
		return err
	}

	// 5. 唯一性预检：username / email
	if exists, _ := s.systemUserRepo.UsernameExists(ctx, username, 0); exists {
		return errors.New("用户名已存在")
	}
	if exists, _ := s.systemUserRepo.EmailExists(ctx, email, 0); exists {
		return errors.New("邮箱已被使用")
	}

	// 6. 创建超管（BeforeCreate 钩子自动 bcrypt 加密 + DataScope 初始化）
	user := &model.SystemUser{
		Username: username,
		Password: password,
		Email:    email,
		Role:     model.SystemUserRoleAdmin,
		Status:   1,
		Enabled:  true,
	}
	if err := s.systemUserRepo.Create(ctx, user); err != nil {
		logger.Error(err, "InitAdmin 创建超管失败")
		return errors.New("创建超管失败: " + err.Error())
	}

	// 7. 同步 install.lock：标记 AdminUsername + Initialized=true
	// 开源版：直接走 install 包（不依赖中间件避免 import cycle）
	if err := install.MarkAdminInitialized(username); err != nil {
		// 标记失败不阻塞主流程（用户已创建成功），仅记日志
		logger.Error(err, "InitAdmin 写 install.lock 失败")
	}

	logger.Info("InitAdmin 超管创建成功: " + username)
	return nil
}
