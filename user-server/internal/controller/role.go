package controller

// role.go 角色管理控制器
//
// 五层架构归属：L2 控制层
// 路由：/api/system/roles/*（由 router/role_routes.go 注册）
//
// 阶段 5 范围：
//   - ListRoles  GET  /api/system/roles
//   - GetRole    GET  /api/system/roles/:code
//   - ListMembers GET /api/system/roles/:code/members
//
// 全部受 RequireAdminMiddleware 保护（路由层）。
// 业务校验失败（service 返回的 *ErrInvalidInput 包装）→ 400。

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// RoleController 角色管理控制器
type RoleController struct {
	svc *service.RoleService
}

// NewRoleController 构造
func NewRoleController() *RoleController {
	return &RoleController{svc: service.NewRoleService()}
}

// ListRoles GET /api/system/roles
//
// 列出 3 档系统角色 + 成员数。
func (ctrl *RoleController) ListRoles(c *gin.Context) {
	roles, err := ctrl.svc.ListRoles(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, roles, "获取角色列表成功")
}

// GetRole GET /api/system/roles/:code
//
// 单个角色详情（带成员数）。
func (ctrl *RoleController) GetRole(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.Error(c, http.StatusBadRequest, "角色 code 不能为空")
		return
	}
	role, err := ctrl.svc.GetRole(c.Request.Context(), code)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, role, "获取角色详情成功")
}

// ListMembers GET /api/system/roles/:code/members
//
// 角色下成员列表（分页）。
// Query: page / size
func (ctrl *RoleController) ListMembers(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.Error(c, http.StatusBadRequest, "角色 code 不能为空")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	members, total, err := ctrl.svc.ListMembersByRole(c.Request.Context(), code, page, size)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	// 分页参数兜底（与 service 层一致）
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	response.SuccessWithPage(c, members, int64(page), int64(size), total)
}
