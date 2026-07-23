package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/system/install"

	"gorm.io/gorm"
)

// LoginRequest 登录请求
type LoginRequest struct {
	Username	string	`json:"username" binding:"required"`
	Password	string	`json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token		string			`json:"token,omitempty"`
	User		*SystemUserResponse	`json:"user,omitempty"`
	Expires		int64			`json:"expires,omitempty"`
	NeedMFA		bool			`json:"need_mfa,omitempty"`	// 是否需要 MFA 二次验证
	TempToken	string			`json:"temp_token,omitempty"`	// 临时令牌（MFA 验证用）
}

// SystemUserResponse 系统用户响应
type SystemUserResponse struct {
	ID			uint		`json:"id"`
	Username		string		`json:"username"`
	Email			string		`json:"email"`
	Phone			string		`json:"phone"`
	RealName		string		`json:"real_name"`
	Role			string		`json:"role"`
	Status			int		`json:"status"`
	LastLogin		*time.Time	`json:"last_login"`
	LastLoginAt		*time.Time	`json:"last_login_at"`	// 兼容前端驼峰命名
	MustChangePassword	bool		`json:"must_change_password"`
	CreatedAt		time.Time	`json:"created_at"`
	UpdatedAt		time.Time	`json:"updated_at"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword	string	`json:"old_password" binding:"required"`
	NewPassword	string	`json:"new_password" binding:"required"`
}

// AuthService 认证服务
type AuthService struct {
	jwtUtils	*utils.JWTUtils
	systemUserRepo	repository.SystemUserRepository
	teamUserRepo	repository.TeamUserRepository
}

// NewAuthService 创建认证服务实例
func NewAuthService() *AuthService {
	return &AuthService{
		jwtUtils:	utils.NewJWTUtils(utils.DefaultJWTConfig),
		systemUserRepo:	repository.NewSystemUserRepository(),
		teamUserRepo:	repository.NewTeamUserRepository(),
	}
}

// JwtUtils 获取 JWT 工具实例（用于测试）
func (s *AuthService) JwtUtils(ctx context.Context) *utils.JWTUtils {
	return s.jwtUtils
}

// Login 用户登录
//
// 严格规则（修复 P0-3）：
//  1. 不再"系统无用户 → 自动注册为超管"——该机制绕过 InitGuard/LicenseGuard，
//     导致未绑 License 即可创建超管，摧毁安全模型。
//  2. 必须先有用户（由 InitSetup 创建）才能登录。
//  3. 用户名/密码错误一律返回"用户名或密码错误"（防枚举）。
//  4. 用户被禁用直接拒绝（明确反馈）。
//  5. 密码用 bcrypt 验证。
//  6. 先查询 system_users 表，找不到再查询 team_users 表（支持团队用户登录）。
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {

	user, err := s.systemUserRepo.GetByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Error(err, "查询用户失败")
		return nil, errors.New("登录失败，请稍后重试")
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		teamUser, err := s.teamUserRepo.GetByUsername(ctx, req.Username)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("用户名或密码错误")
			}
			logger.Error(err, "查询团队用户失败")
			return nil, errors.New("登录失败，请稍后重试")
		}

		if teamUser.Status != model.TeamUserStatusActive {
			return nil, errors.New("用户已被禁用")
		}

		if bcrypt.CheckPassword(teamUser.Password, req.Password) != nil {
			return nil, errors.New("用户名或密码错误")
		}

		// 检查是否启用 MFA（P1-1 缺口修复：登录后第二步 TOTP 验证）
		mfaSvc := NewMFAService()
		mfaEnabled, err := mfaSvc.IsMFAEnabled(ctx, teamUser.ID)
		if err != nil {
			logger.Errorf("TeamUser MFA 状态查询失败: %v", err)
		}
		if mfaEnabled {
			tempToken, err := mfaSvc.IssueTempToken(ctx, teamUser.ID, teamUser.Username, teamUser.Role)
			if err != nil {
				return nil, errors.New("登录失败，请稍后重试")
			}
			return &LoginResponse{
				NeedMFA:	true,
				TempToken:	tempToken,
			}, nil
		}

		now := time.Now()
		teamUser.LastLoginAt = &now
		if err := s.teamUserRepo.Update(ctx, teamUser); err != nil {
			logger.Errorf("更新团队用户最后登录时间失败: %v", err)
		}

		token, err := s.jwtUtils.GenerateToken(teamUser.ID, teamUser.Username, teamUser.Role)
		if err != nil {
			logger.Error(err, "生成JWT令牌失败")
			return nil, errors.New("登录失败，请稍后重试")
		}

		return &LoginResponse{
			Token:	token,
			User: &SystemUserResponse{
				ID:		teamUser.ID,
				Username:	teamUser.Username,
				Role:		teamUser.Role,
				Status:		int(teamUser.Status),
				CreatedAt:	teamUser.CreatedAt,
				UpdatedAt:	teamUser.UpdatedAt,
			},
			Expires:	time.Now().Add(time.Hour * time.Duration(utils.DefaultJWTConfig.ExpiresHours)).Unix(),
		}, nil
	}

	if user.Status != 1 {
		return nil, errors.New("用户已被禁用")
	}

	if !CheckPassword(user, req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	return s.loginWithUser(ctx, user)
}

// loginWithUser 使用用户对象完成登录流程
func (s *AuthService) loginWithUser(ctx context.Context, user *model.SystemUser) (*LoginResponse, error) {
	// P1-1 修复：MFA 启用检查
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
			NeedMFA:	true,
			TempToken:	tempToken,
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
		Token:		token,
		User:		s.toUserResponse(ctx, user),
		Expires:	time.Now().Add(time.Hour * time.Duration(utils.DefaultJWTConfig.ExpiresHours)).Unix(),
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
// P1-3 修复：强制走密码策略校验 + 记录历史
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

	// P1-3 修复：强制走密码策略校验
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

	// P1-3 修复：记录密码历史
	if err := policySvc.RecordPasswordHistory(ctx, userID, req.NewPassword, model.PasswordSourceChangePassword); err != nil {
		logger.Errorf("记录密码历史失败（不影响改密流程）: %v", err)
	}

	return nil
}

// toUserResponse 转换为用户响应
func (s *AuthService) toUserResponse(ctx context.Context, user *model.SystemUser) *SystemUserResponse {
	return &SystemUserResponse{
		ID:		user.ID,
		Username:	user.Username,
		Email:		user.Email,
		Phone:		user.Phone,
		RealName:	user.RealName,
		Role:		user.Role,
		Status:		user.Status,
		LastLogin:	user.LastLogin,
		LastLoginAt:	user.LastLogin,
		CreatedAt:	user.CreatedAt,
		UpdatedAt:	user.UpdatedAt,
	}
}

// InitChangePassword 初始化流程的首次强制改密
// 规则：
//   - 必须存在 username 匹配、且 must_change_password=true 的系统用户
//   - 不需要旧密码（初始化阶段特殊通道）
//   - 改密成功后清除 must_change_password 标志
//   - 改密成功后调用 LicenseChecker.MarkAdminInitialized()，使 install.lock 进入 INITIALIZED
//   - 改密失败不影响 install.lock 状态
func (s *AuthService) InitChangePassword(ctx context.Context, username, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("用户名不能为空")
	}

	// 1. 密码强度校验（与初始化时一致）
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	// 2. 查找用户（必须存在 + must_change_password=true）
	user, err := s.systemUserRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("用户不存在或无需强制改密")
		}
		logger.Error(err, "InitChangePassword 查询用户失败")
		return errors.New("改密失败，请稍后重试")
	}

	if !user.MustChangePassword {
		return errors.New("此账号无需强制改密，请使用普通改密流程")
	}

	// 3. 更新密码
	hashed, err := HashPassword(newPassword)
	if err != nil {
		logger.Error(err, "InitChangePassword 密码加密失败")
		return errors.New("改密失败，请稍后重试")
	}
	user.Password = hashed
	user.MustChangePassword = false
	// 保证 role=admin（防御性：万一被错改）
	if user.Role != "admin" {
		user.Role = "admin"
	}

	if err := s.systemUserRepo.Update(ctx, user); err != nil {
		logger.Error(err, "InitChangePassword 保存用户失败")
		return errors.New("改密失败，请稍后重试")
	}

	// 4. 标记 install.lock 为 AdminInitialized=true（关键修复 P0-2）
	// 开源版：直接走 install 包（不依赖中间件避免 import cycle）
	if err := install.MarkAdminInitializedStandalone(); err != nil {
		// 不影响主流程，但必须记录
		logger.Error(err, "InitChangePassword 标记 install.lock 失败")
	}

	logger.Info("InitChangePassword 首次改密完成: " + username)
	return nil
}

// ============== P0-4 平台超管忘记密码流程 ==============

// forgotTokenStore 全局：保存 reset_token → {username, expire_at}
// 单实例部署够用；多实例需改用 Redis（架构升级时再迁移）
var (
	forgotTokenStore	= make(map[string]forgotTokenEntry)
	forgotTokenStoreMutex	sync.RWMutex
)

type forgotTokenEntry struct {
	Username	string
	ExpiresAt	time.Time
}

// CreateForgotPasswordToken 创建"忘记密码"一次性 token
// 开源版：仅校验 username == install.lock.AdminUsername，移除公司名校验
// 私域部署：响应中直接返回 token（管理员自己操作）
// 公网部署：应改为通过 contact_email 发送，不直接返回
func (s *AuthService) CreateForgotPasswordToken(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("用户名不能为空")
	}

	// 1. 加载 install.lock
	lock, err := install.LoadInstallLockPublic()
	if err != nil {
		return "", errors.New("系统未安装或安装文件损坏")
	}
	if lock == nil {
		return "", errors.New("系统尚未初始化")
	}

	// 2. 校验 username == AdminUsername
	if lock.AdminUsername == "" {
		return "", errors.New("系统未创建超管，无法重置密码")
	}
	if !strings.EqualFold(lock.AdminUsername, username) {
		// 不暴露具体错误
		return "", errors.New("用户名不匹配")
	}

	// 3. 生成 64 字符 token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", errors.New("生成 token 失败")
	}
	token := hex.EncodeToString(tokenBytes)	// 64 字符

	// 5. 存入 store（5 分钟过期）
	entry := forgotTokenEntry{
		Username:	username,
		ExpiresAt:	time.Now().Add(5 * time.Minute),
	}
	forgotTokenStoreMutex.Lock()
	forgotTokenStore[token] = entry
	forgotTokenStoreMutex.Unlock()

	// 清理过期 token
	go s.cleanupExpiredForgotTokens(ctx)

	logger.Info("CreateForgotPasswordToken: 为 " + username + " 创建 reset_token")
	return token, nil
}

// ResetAdminPasswordWithToken 使用 reset_token 重置超管密码
func (s *AuthService) ResetAdminPasswordWithToken(ctx context.Context, username, token, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" || token == "" {
		return errors.New("参数错误")
	}
	if len(token) != 64 {
		return errors.New("token 格式不正确")
	}

	// 1. 校验密码强度
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	// 2. 查找并消费 token
	forgotTokenStoreMutex.Lock()
	entry, ok := forgotTokenStore[token]
	if ok {
		delete(forgotTokenStore, token)	// 一次性
	}
	forgotTokenStoreMutex.Unlock()

	if !ok {
		return errors.New("token 无效或已使用")
	}
	if time.Now().After(entry.ExpiresAt) {
		return errors.New("token 已过期")
	}
	if !strings.EqualFold(entry.Username, username) {
		return errors.New("token 与用户名不匹配")
	}

	// 3. 找到用户并重置密码
	user, err := s.systemUserRepo.GetByUsername(ctx, username)
	if err != nil {
		return errors.New("用户不存在")
	}
	hashed, err := HashPassword(newPassword)
	if err != nil {
		return errors.New("用户不存在")
	}
	user.Password = hashed
	// 强制下次登录再改一次
	user.MustChangePassword = true
	if err := s.systemUserRepo.Update(ctx, user); err != nil {
		return errors.New("保存失败，请稍后重试")
	}

	logger.Info("ResetAdminPasswordWithToken: 超管密码重置成功 username=" + username)
	return nil
}

// CleanupExpiredForgotTokens 清理过期的 forgot token（公开给 cron 或测试调用）
func (s *AuthService) cleanupExpiredForgotTokens(ctx context.Context) {
	forgotTokenStoreMutex.Lock()
	defer forgotTokenStoreMutex.Unlock()
	now := time.Now()
	for k, v := range forgotTokenStore {
		if now.After(v.ExpiresAt) {
			delete(forgotTokenStore, k)
		}
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
