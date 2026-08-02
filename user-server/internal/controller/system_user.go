package controller

// system_user.go 系统用户控制器（人员管理）
//
// 五层架构归属：L2 控制层
// 路由：/api/system/users/*（仅 admin 可见，由 router/system_user_routes.go 注册）
//
// 阶段 4 范围：
//   - GetUsers / GetByID / Create / Update / Delete
//   - 全部受 RequireAdminMiddleware 保护（路由层）
//   - 业务校验失败（service 返回的 *service.ErrInvalidInput 包装）→ 400
//
// 注意：
//   - 旧的 SystemUserController（GetUsers/GetUser/CreateUser/UpdateUser/DeleteUser/ResetPassword）
//     仍位于 controller/auth.go，供 setupUserRoutes（/api/user/*、/api/users/*）使用
//   - 本文件独立承载"人员管理"模块的 5 个新端点，避免与旧 controller 互相干扰

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// SystemUserAdminController 人员管理控制器（阶段 4 新增）
type SystemUserAdminController struct {
	svc *service.SystemUserService
}

// NewSystemUserAdminController 构造
func NewSystemUserAdminController() *SystemUserAdminController {
	return &SystemUserAdminController{
		svc: service.NewSystemUserService(),
	}
}

// extractActorID 从 gin.Context 取 actor user_id（由 JWTAuthMiddleware 写入）
//
// 缺失或类型错误返回 401。
func extractActorID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("user_id")
	if !ok || v == nil {
		response.Error(c, http.StatusUnauthorized, "未授权，请先登录")
		return 0, false
	}
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val < 0 {
			response.Error(c, http.StatusUnauthorized, "user_id 非法")
			return 0, false
		}
		return uint(val), true
	case int64:
		if val < 0 {
			response.Error(c, http.StatusUnauthorized, "user_id 非法")
			return 0, false
		}
		return uint(val), true
	case float64:
		if val < 0 {
			response.Error(c, http.StatusUnauthorized, "user_id 非法")
			return 0, false
		}
		return uint(val), true
	default:
		response.Error(c, http.StatusUnauthorized, "user_id 类型错误")
		return 0, false
	}
}

// parseSysUserIDParam 解析路径参数 :id（避免与 base_card_controller.parseIDParam 冲突）
func parseSysUserIDParam(c *gin.Context, paramName string) (uint, bool) {
	raw := c.Param(paramName)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "无效的 ID")
		return 0, false
	}
	return uint(id), true
}

// writeServiceError 统一 service 错误响应：
//   - service.ErrInvalidInput → 400
//   - 其它（系统级 / DB） → 500
func writeServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, service.ErrInvalidInput) {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.ErrorFromDB(c, err, err.Error())
}

// GetUsers GET /api/system/users
//
// Query：keyword / role / page / size
func (c *SystemUserAdminController) GetUsers(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "20"))
	req := &service.GetUsersRequest{
		Keyword: ctx.Query("keyword"),
		Role:    ctx.Query("role"),
		Page:    page,
		Size:    size,
	}
	resp, err := c.svc.GetUsersAdmin(ctx.Request.Context(), req)
	if err != nil {
		writeServiceError(ctx, err)
		return
	}
	// 分页参数兜底（与 service 层一致）
	p := req.Page
	if p < 1 {
		p = 1
	}
	s := req.Size
	if s <= 0 {
		s = 20
	}
	if s > 100 {
		s = 100
	}
	response.SuccessWithPage(ctx, resp.List, int64(p), int64(s), resp.Total)
}

// GetByID GET /api/system/users/:id
func (c *SystemUserAdminController) GetByID(ctx *gin.Context) {
	id, ok := parseSysUserIDParam(ctx, "id")
	if !ok {
		return
	}
	user, err := c.svc.GetByIDAdmin(ctx.Request.Context(), id)
	if err != nil {
		writeServiceError(ctx, err)
		return
	}
	response.Success(ctx, user, "获取账号详情成功")
}

// Create POST /api/system/users
func (c *SystemUserAdminController) Create(ctx *gin.Context) {
	actorID, ok := extractActorID(ctx)
	if !ok {
		return
	}
	var req service.CreateUserByAdminRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	user, err := c.svc.CreateByAdmin(ctx.Request.Context(), actorID, &req)
	if err != nil {
		writeServiceError(ctx, err)
		return
	}
	response.Success(ctx, user, "账号创建成功")
}

// Update PUT /api/system/users/:id
func (c *SystemUserAdminController) Update(ctx *gin.Context) {
	actorID, ok := extractActorID(ctx)
	if !ok {
		return
	}
	id, ok := parseSysUserIDParam(ctx, "id")
	if !ok {
		return
	}
	var req service.UpdateUserByAdminRequest
	if !response.BindJSON(ctx, &req) {
		return
	}
	user, err := c.svc.UpdateByAdmin(ctx.Request.Context(), actorID, id, &req)
	if err != nil {
		writeServiceError(ctx, err)
		return
	}
	response.Success(ctx, user, "账号更新成功")
}

// Delete DELETE /api/system/users/:id
func (c *SystemUserAdminController) Delete(ctx *gin.Context) {
	actorID, ok := extractActorID(ctx)
	if !ok {
		return
	}
	id, ok := parseSysUserIDParam(ctx, "id")
	if !ok {
		return
	}
	if err := c.svc.DeleteByAdmin(ctx.Request.Context(), actorID, id); err != nil {
		writeServiceError(ctx, err)
		return
	}
	response.Success(ctx, nil, "账号删除成功")
}
