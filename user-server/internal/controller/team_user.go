package controller

import (
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// TeamUserController 团队用户控制器
type TeamUserController struct {
	userService       *service.TeamUserService
	systemUserService *service.SystemUserService
}

// NewTeamUserController 创建团队用户控制器实例
func NewTeamUserController() *TeamUserController {
	return &TeamUserController{
		userService:       service.NewTeamUserService(),
		systemUserService: service.NewSystemUserService(),
	}
}

// GetList 获取用户列表
func (c *TeamUserController) GetList(ctx *gin.Context) {
	// 获取商户ID

	// 获取分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	// 获取用户列表
	result, err := c.userService.GetList(ctx, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, result, "获取用户列表成功")
}

// GetByID 获取用户详情
func (c *TeamUserController) GetByID(ctx *gin.Context) {
	// 获取商户ID

	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取用户信息
	user, err := c.userService.GetByID(ctx, uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, user, "获取用户信息成功")
}

// Create 创建用户
// P1-7 修复：Service 层权限断言（取 role 从 context 透传到 Service）
func (c *TeamUserController) Create(ctx *gin.Context) {
	// 获取操作者ID
	userID, _ := ctx.Get("user_id")
	operatorID, _ := userID.(uint)
	// P1-7：从 context 提取 operatorRole 透传到 Service 用于权限断言
	operatorRole, _ := ctx.Get("role")
	operatorRoleStr, _ := operatorRole.(string)

	// 解析请求
	var req service.CreateTeamUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 创建用户（含 Service 层权限断言）
	user, err := c.userService.Create(ctx, &req, operatorID, operatorRoleStr, ctx.ClientIP())
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, user, "创建用户成功")
}

// Update 更新用户
// P1-7 修复：Service 层权限断言
func (c *TeamUserController) Update(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取操作者ID
	userID, _ := ctx.Get("user_id")
	operatorID, _ := userID.(uint)
	// P1-7：透传 role
	operatorRole, _ := ctx.Get("role")
	operatorRoleStr, _ := operatorRole.(string)

	// 解析请求
	var req service.UpdateTeamUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 更新用户（含 Service 层权限断言）
	user, err := c.userService.Update(ctx, uint(id), &req, operatorID, operatorRoleStr, ctx.ClientIP())
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, user, "更新用户成功")
}

// Delete 删除用户
// P1-7 修复：Service 层权限断言
func (c *TeamUserController) Delete(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取操作者ID
	userID, _ := ctx.Get("user_id")
	operatorID, _ := userID.(uint)
	// P1-7：透传 role
	operatorRole, _ := ctx.Get("role")
	operatorRoleStr, _ := operatorRole.(string)

	// 删除用户（含 Service 层权限断言）
	if err := c.userService.Delete(ctx, uint(id), operatorID, operatorRoleStr, ctx.ClientIP()); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除用户成功")
}

// ChangePassword 修改密码
func (c *TeamUserController) ChangePassword(ctx *gin.Context) {
	// 获取商户ID

	// 获取用户ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, _ := userID.(uint)

	// 解析请求
	var req service.TeamChangePasswordRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 修改密码
	if err := c.userService.ChangePassword(ctx, uid, &req, ctx.ClientIP()); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "修改密码成功")
}

// ResetPassword 重置密码
// P1-7 修复：Service 层权限断言
func (c *TeamUserController) ResetPassword(ctx *gin.Context) {
	// 获取用户ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户ID")
		return
	}

	// 获取操作者ID
	userID, _ := ctx.Get("user_id")
	operatorID, _ := userID.(uint)
	// P1-7：透传 role
	operatorRole, _ := ctx.Get("role")
	operatorRoleStr, _ := operatorRole.(string)

	// 解析请求
	var req struct {
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 重置密码（含 Service 层权限断言）
	if err := c.userService.ResetPassword(ctx, uint(id), req.Password, operatorID, operatorRoleStr, ctx.ClientIP()); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "重置密码成功")
}

// Login 登录
func (c *TeamUserController) Login(ctx *gin.Context) {
	// 获取商户ID

	// 解析请求
	var req service.TeamUserLoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 登录
	result, err := c.userService.Login(ctx, &req, ctx.ClientIP())
	if err != nil {
		response.Error(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	response.Success(ctx, result, "登录成功")
}

// GetCurrentuser 获取当前用户信息
// 修复：优先从 system_users 表查找（admin/平台超管），找不到再回退到 team_users（业务用户）
func (c *TeamUserController) GetCurrentUser(ctx *gin.Context) {
	// 获取商户ID

	// 获取用户ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		// 尝试从 float64 转换
		if f, ok := userID.(float64); ok {
			uid = uint(f)
		}
	}

	// 优先查 system_users（admin/超管），找不到再回退到 team_users
	user, err := c.systemUserService.GetUserByID(uid)
	if err != nil || user == nil {
		// 回退到 team_users
		teamUser, terr := c.userService.GetByID(ctx, uid)
		if HandleDBError(ctx, terr, "获取用户信息") {
			return
		}
		response.Success(ctx, teamUser, "获取用户信息成功")
		return
	}

	response.Success(ctx, user, "获取用户信息成功")
}

// TeamRoleController 团队角色控制器
type TeamRoleController struct {
	roleService *service.TeamRoleService
}

// NewTeamRoleController 创建团队角色控制器实例
func NewTeamRoleController() *TeamRoleController {
	return &TeamRoleController{
		roleService: service.NewTeamRoleService(),
	}
}

// GetList 获取角色列表
func (c *TeamRoleController) GetList(ctx *gin.Context) {
	// 独立部署：单租户，无需 merchantID
	roles, err := c.roleService.GetList(ctx)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, roles, "获取角色列表成功")
}

// Create 创建角色
func (c *TeamRoleController) Create(ctx *gin.Context) {
	// 获取商户ID

	// 解析请求
	var req service.CreateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 创建角色
	role, err := c.roleService.Create(ctx, &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, role, "创建角色成功")
}

// Update 更新角色
func (c *TeamRoleController) Update(ctx *gin.Context) {
	// 获取商户ID

	// 获取角色ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的角色ID")
		return
	}

	// 解析请求
	var req service.UpdateRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 更新角色
	role, err := c.roleService.Update(ctx, uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, role, "更新角色成功")
}

// Delete 删除角色
func (c *TeamRoleController) Delete(ctx *gin.Context) {
	// 获取商户ID

	// 获取角色ID
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的角色ID")
		return
	}

	// 删除角色
	if err := c.roleService.Delete(ctx, uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除角色成功")
}

// GetPermissions 获取所有权限
func (c *TeamRoleController) GetPermissions(ctx *gin.Context) {
	permissions := c.roleService.GetPermissions()
	response.Success(ctx, permissions, "获取权限列表成功")
}
