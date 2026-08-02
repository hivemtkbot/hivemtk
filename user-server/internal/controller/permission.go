package controller

// permission.go 授权管理控制器
//
// 五层架构归属：L2 控制层
// 路由：/api/system/permissions/*（由 router/permission_routes.go 注册）
//
// 阶段 6 范围：
//   - SetEnabled    PUT  /api/system/permissions/:id/enabled
//   - ResetPassword PUT  /api/system/permissions/:id/password
//   - ListAuditLogs GET  /api/system/permissions/audit-logs
//
// 全部受 RequireAdminMiddleware 保护（路由层）。
// 业务校验失败（service 返回的 *ErrInvalidInput 包装）→ 400。

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// PermissionController 授权管理控制器
type PermissionController struct {
	svc *service.AuthorizationService
}

// NewPermissionController 构造
func NewPermissionController() *PermissionController {
	return &PermissionController{svc: service.NewAuthorizationService()}
}

// SetEnabled PUT /api/system/permissions/:id/enabled
//
// Body: { "enabled": true|false }
func (ctrl *PermissionController) SetEnabled(c *gin.Context) {
	id, ok := parseSysUserIDParam(c, "id")
	if !ok {
		return
	}
	actorID, ok := extractActorID(c)
	if !ok {
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !response.BindJSON(c, &req) {
		return
	}

	if err := ctrl.svc.SetEnabled(c.Request.Context(), actorID, id, req.Enabled); err != nil {
		writePermissionServiceError(c, err)
		return
	}

	action := "启用"
	if !req.Enabled {
		action = "禁用"
	}
	response.Success(c, nil, "账号"+action+"成功")
}

// ResetPassword PUT /api/system/permissions/:id/password
//
// Body: { "password": "NewPass123" }
func (ctrl *PermissionController) ResetPassword(c *gin.Context) {
	id, ok := parseSysUserIDParam(c, "id")
	if !ok {
		return
	}
	actorID, ok := extractActorID(c)
	if !ok {
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if !response.BindJSON(c, &req) {
		return
	}
	if req.Password == "" {
		response.Error(c, http.StatusBadRequest, "新密码不能为空")
		return
	}

	if err := ctrl.svc.ResetPassword(c.Request.Context(), actorID, id, req.Password); err != nil {
		writePermissionServiceError(c, err)
		return
	}
	response.Success(c, nil, "密码重置成功")
}

// ListAuditLogs GET /api/system/permissions/audit-logs
//
// Query: user_id / action / page / page_size
func (ctrl *PermissionController) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	action := c.Query("action")

	req := &service.ListAuditLogsRequest{
		Action:   action,
		Page:     page,
		PageSize: size,
	}
	if userID > 0 {
		req.UserID = uint(userID)
	}

	resp, err := ctrl.svc.ListAuditLogs(c.Request.Context(), req)
	if err != nil {
		writePermissionServiceError(c, err)
		return
	}

	// 分页参数兜底
	p := req.Page
	if p < 1 {
		p = 1
	}
	s := req.PageSize
	if s <= 0 {
		s = 20
	}
	if s > 100 {
		s = 100
	}
	response.SuccessWithPage(c, resp.List, int64(p), int64(s), resp.Total)
}

// writePermissionServiceError 授权管理 service 错误响应：
//   - service.ErrInvalidInput → 400
//   - 其它（系统级 / DB）→ 500
func writePermissionServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, service.ErrInvalidInput) {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.ErrorFromDB(c, err, err.Error())
}
