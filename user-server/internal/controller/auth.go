package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// AuthController 认证控制器
type AuthController struct {
	authService *service.AuthService
	mfaService  *service.MFAService
	riskService *service.LoginRiskService
}

// NewAuthController 创建认证控制器实例
func NewAuthController() *AuthController {
	return &AuthController{
		authService: service.NewAuthService(),
		mfaService:  service.NewMFAService(),
		riskService: service.NewLoginRiskService(),
	}
}

// Login 用户登录
func (c *AuthController) Login(ctx *gin.Context) {
	var req service.LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 即使在测试模式下，登录也必须验证用户名和密码
	// 测试模式只跳过后续的 JWT 中间件认证，不跳过登录验证
	resp, err := c.authService.Login(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	response.Success(ctx, resp, "登录成功")
}

// RefreshToken 刷新令牌
// 修复：
//   - 校验 Bearer 前缀
//   - token 为空直接拒绝
//   - 使用安全的 trim 前缀而非裸切片
//   - token 通过 gin Context 解析后调用 JWT 工具刷新
func (c *AuthController) RefreshToken(ctx *gin.Context) {
	// 从请求头获取 Authorization
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" {
		response.Error(ctx, http.StatusUnauthorized, "未提供认证令牌")
		return
	}

	// 严格校验 Bearer 前缀
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		response.Error(ctx, http.StatusUnauthorized, "Authorization 头格式错误，应为 Bearer <token>")
		return
	}
	token := strings.TrimSpace(authHeader[len(prefix):])
	if token == "" {
		response.Error(ctx, http.StatusUnauthorized, "认证令牌不能为空")
		return
	}

	// 刷新令牌
	newToken, err := c.authService.RefreshToken(context.Background(), token)
	if err != nil {
		response.Error(ctx, http.StatusUnauthorized, "刷新令牌失败", err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"token": newToken,
	}, "刷新令牌成功")
}

// GetCurrentUser 获取当前用户信息
func (c *AuthController) GetCurrentUser(ctx *gin.Context) {
	// 从上下文获取用户ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	// 转换为uint类型
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	// 获取用户信息
	user, err := c.authService.GetCurrentUser(context.Background(), uid)
	if err != nil {
		if HandleServiceError(ctx, err) {
			return
		}
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, user, "获取用户信息成功")
}

// ChangePassword 修改密码
func (c *AuthController) ChangePassword(ctx *gin.Context) {
	// 从上下文获取用户ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	// 转换为uint类型
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	var req service.ChangePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 修改密码
	if err := c.authService.ChangePassword(context.Background(), uid, &req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "修改密码成功")
}

// ============== MFA 多因素认证 ==============

// SetupMFA 设置 MFA：生成 TOTP 密钥并返回 otpauth URL
// 用户使用 Google Authenticator 扫描二维码
func (c *AuthController) SetupMFA(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	username, _ := ctx.Get("username")
	usernameStr, _ := username.(string)

	resp, err := c.mfaService.SetupMFA(context.Background(), uid, usernameStr)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, resp, "请使用 Google Authenticator 扫描二维码")
}

// ConfirmMFASetup 确认 MFA 设置：用户输入 6 位码验证，验证成功后启用 MFA
func (c *AuthController) ConfirmMFASetup(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	var req service.MFASetupVerifyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	if err := c.mfaService.ConfirmMFASetup(context.Background(), uid, req.Code); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"mfa_enabled": true,
		"message":     "MFA 启用成功，下次登录需输入验证码",
	}, "MFA 启用成功")
}

// DisableMFA 禁用 MFA
// 需要校验密码 + TOTP 码（双重保护）
func (c *AuthController) DisableMFA(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	var req service.MFADisableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	if err := c.mfaService.DisableMFA(context.Background(), uid, req.Password, req.Code); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"mfa_enabled": false,
		"message":     "MFA 已禁用",
	}, "MFA 已禁用")
}

// VerifyMFALogin MFA 登录验证（登录第二步）
// POST /api/auth/mfa/verify
// Body: { "temp_token": "...", "code": "123456" }
// 成功返回 JWT token
func (c *AuthController) VerifyMFALogin(ctx *gin.Context) {
	var req service.MFAVerifyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	userID, username, role, err := c.mfaService.VerifyMFALogin(context.Background(), req.TempToken, req.Code)
	if err != nil {
		response.Error(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	// 颁发正式 JWT
	jwtUtils := c.authService.JwtUtils(context.Background())
	token, err := jwtUtils.GenerateToken(userID, username, role)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "颁发令牌失败")
		return
	}

	// 标记 MFA 已验证（用于敏感操作中间件）
	middleware.MarkMFAVerified(userID)

	response.Success(ctx, gin.H{
		"token":   token,
		"expires": 86400,
		"user": gin.H{
			"id":       userID,
			"username": username,
			"role":     role,
		},
	}, "MFA 验证成功，登录完成")
}

// GetMFAStatus 查询当前用户的 MFA 启用状态
func (c *AuthController) GetMFAStatus(ctx *gin.Context) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusInternalServerError, "用户ID类型错误")
		return
	}

	enabled, err := c.mfaService.IsMFAEnabled(context.Background(), uid)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"mfa_enabled": enabled,
	}, "查询成功")
}

// ============== 异常登录预警 ==============

// ListLoginEvents 查询登录事件列表
// GET /api/auth/login-events?page=1&page_size=20
func (c *AuthController) ListLoginEvents(ctx *gin.Context) {
	userID, _ := ctx.Get("user_id")
	uid, _ := userID.(uint)

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	events, total, err := c.riskService.ListLoginEvents(context.Background(), uid, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      events,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "查询成功")
}

// ListSecurityAlerts 查询安全告警列表
// GET /api/auth/security-alerts?status=open&page=1&page_size=20
func (c *AuthController) ListSecurityAlerts(ctx *gin.Context) {
	userID, _ := ctx.Get("user_id")
	uid, _ := userID.(uint)

	status := ctx.Query("status")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	alerts, total, err := c.riskService.ListSecurityAlerts(context.Background(), uid, status, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      alerts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "查询成功")
}

// ResolveSecurityAlert 处理安全告警
// POST /api/auth/security-alerts/:id/resolve
func (c *AuthController) ResolveSecurityAlert(ctx *gin.Context) {
	alertIDStr := ctx.Param("id")
	alertID, err := strconv.ParseUint(alertIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的告警 ID")
		return
	}

	userID, _ := ctx.Get("user_id")
	uid, _ := userID.(uint)

	var req struct {
		Note string `json:"note"`
	}
	_ = ctx.ShouldBindJSON(&req)

	if err := c.riskService.ResolveSecurityAlert(context.Background(), uint(alertID), uid, req.Note); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "告警已处理")
}

// ============== 密码策略 ==============

// GetPasswordPolicy 查询当前密码策略
// GET /api/auth/password-policy
func (c *AuthController) GetPasswordPolicy(ctx *gin.Context) {
	policySvc := service.NewPasswordPolicyService()
	policy := policySvc.GetPolicy(context.Background())
	response.Success(ctx, policy, "查询成功")
}

// SavePasswordPolicy 更新密码策略（仅 admin）
// PUT /api/auth/password-policy
func (c *AuthController) SavePasswordPolicy(ctx *gin.Context) {
	role, _ := ctx.Get("role")
	roleStr, _ := role.(string)
	if roleStr != "admin" {
		response.Error(ctx, http.StatusForbidden, "仅管理员可修改密码策略")
		return
	}

	var policy service.PasswordPolicy
	if err := ctx.ShouldBindJSON(&policy); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	policySvc := service.NewPasswordPolicyService()
	if err := policySvc.SavePolicy(context.Background(), &policy); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, policy, "密码策略已更新")
}

// SystemUserController 系统用户控制器
type SystemUserController struct {
	userService *service.SystemUserService
}

// NewSystemUserController 创建系统用户控制器实例
func NewSystemUserController() *SystemUserController {
	return &SystemUserController{
		userService: service.NewSystemUserService(),
	}
}

// GetUsers 获取用户列表
func (c *SystemUserController) GetUsers(ctx *gin.Context) {
	// 获取分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 获取用户列表
	users, total, err := c.userService.GetUsers(context.Background(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取用户列表成功")
}

// GetUser 获取用户详情
func (c *SystemUserController) GetUser(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取用户信息
	user, err := c.userService.GetUserByID(context.Background(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, user, "获取用户信息成功")
}

// CreateUser 创建用户
func (c *SystemUserController) CreateUser(ctx *gin.Context) {
	var req service.CreateUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 创建用户
	user, err := c.userService.CreateUser(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, user, "创建用户成功")
}

// UpdateUser 更新用户
func (c *SystemUserController) UpdateUser(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	var req service.UpdateUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 更新用户
	user, err := c.userService.UpdateUser(context.Background(), uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, user, "更新用户成功")
}

// DeleteUser 删除用户
func (c *SystemUserController) DeleteUser(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 删除用户
	if err := c.userService.DeleteUser(context.Background(), uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除用户成功")
}

// ResetPassword 重置用户密码
func (c *SystemUserController) ResetPassword(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取新密码
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 重置密码
	if err := c.userService.ResetPassword(context.Background(), uint(id), req.Password); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "重置密码成功")
}

// InitAdmin 公开：初始化系统超管
// 路由：POST /api/system/init-admin（详见 admin_routes.go setupPublicRoutes）
// 流程：BindJSON → service.AuthService.InitAdmin → install.lock
//
// InitAdminRequest 复用 controller/system_init.go 已有的同 struct（避免重复定义）：
//   - Username: 3-20 位
//   - Password: ≥8 位
//   - Email:    可选，格式合法
//   - RealName / ContactPhone: 可选（开源版 init 上报用，本流程不消费）
func (c *AuthController) InitAdmin(ctx *gin.Context) {
	var req InitAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	if err := c.authService.InitAdmin(context.Background(), req.Username, req.Password, req.Email); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"username": req.Username,
		"message":  "超管创建成功，请使用此账号登录",
	}, "超管初始化完成")
}

// CreateDefaultAdmin 创建默认管理员账户（不需要认证，保留兼容旧前端）
//
// 系统用户统一 plan v3.1 §3.2：
//   - 函数名保留（避免破坏旧路由 / 测试 / 文档引用）
//   - 函数体重写：不再读取 config.GetAdminConfig().DefaultAdmin 的硬编码密码，
//     从请求体 InitAdminRequest 读取 username/password/email
//   - 委派给 service.AuthService.InitAdmin（统一的强密码 / 唯一性 / install.lock 流程）
//   - 路由 /api/system/create-default-admin 已从 public 中移除（见 admin_routes.go），
//     但函数保留以便 init_guard 白名单 / 历史测试 / 第三方集成仍可调用
func (c *SystemUserController) CreateDefaultAdmin(ctx *gin.Context) {
	var req InitAdminRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 委派给 AuthService（与 AuthController.InitAdmin 共享同一份业务逻辑）
	authSvc := service.NewAuthService()
	if err := authSvc.InitAdmin(context.Background(), req.Username, req.Password, req.Email); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"username": req.Username,
		"message":  "默认管理员创建成功（兼容旧路由）",
	}, "默认管理员创建成功")
}
