package controller

// row_level_security_controller.go A 域 P1-4 数据行级权限控制器
//
// 五层架构归属: L3 业务编排（薄层 controller）
// 设计依据: docs/standards/MASTER_RULES.md「Controller 仅参数解析 / 调 service / 统一响应」
//          A 域 P1 缺口修复 (2026-07-21)
//
// 职责：暴露 team_user 数据范围（data_scope）的查询/修改接口
//
// 路由（由 router 层注册）：
//   GET  /api/team/users/:id/data-scope   查询用户的 data_scope
//   PUT  /api/team/users/:id/data-scope   修改用户的 data_scope（仅 admin）

import (
	"net/http"
	"strconv"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// RowLevelSecurityController 行级权限控制器
type RowLevelSecurityController struct {
	rowLevelSvc *service.RowLevelSecurityService
	teamUserSvc *service.TeamUserService
}

// NewRowLevelSecurityController 创建行级权限控制器
func NewRowLevelSecurityController() *RowLevelSecurityController {
	return &RowLevelSecurityController{
		rowLevelSvc: service.NewRowLevelSecurityService(),
		teamUserSvc: service.NewTeamUserService(),
	}
}

// DataScopeResponse data_scope 响应
type DataScopeResponse struct {
	UserID        uint   `json:"user_id"`
	DataScope     int    `json:"data_scope"`
	DataScopeName string `json:"data_scope_name"`
	DepartmentID  uint   `json:"department_id"`
	TeamID        uint   `json:"team_id"`
	CustomDeptIDs string `json:"custom_dept_ids,omitempty"`
	IsAdmin       bool   `json:"is_admin"`
}

// GetUserDataScope 查询用户的 data_scope
// @Summary 查询用户的行级权限
// @Description 返回 team_user.data_scope 及其相关字段（部门ID/团队ID/自定义白名单）
// @Tags A域-行级权限
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response{data=DataScopeResponse}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/team/users/{id}/data-scope [get]
func (c *RowLevelSecurityController) GetUserDataScope(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户 ID")
		return
	}

	user, err := c.teamUserSvc.GetByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "用户不存在")
		return
	}

	role, _ := ctx.Get("role")
	roleStr, _ := role.(string)
	isAdmin := roleStr == "admin"

	response.Success(ctx, DataScopeResponse{
		UserID:        user.ID,
		DataScope:     user.DataScope,
		DataScopeName: c.rowLevelSvc.BuildScopeDescription(user.DataScope),
		DepartmentID:  user.DepartmentID,
		TeamID:        user.TeamID,
		CustomDeptIDs: user.CustomDeptIDs,
		IsAdmin:       isAdmin,
	}, "查询成功")
}

// UpdateDataScopeRequest 修改 data_scope 请求
type UpdateDataScopeRequest struct {
	DataScope     int    `json:"data_scope" binding:"required"`
	DepartmentID  uint   `json:"department_id"`
	TeamID        uint   `json:"team_id"`
	CustomDeptIDs string `json:"custom_dept_ids"`
}

// UpdateUserDataScope 修改用户的 data_scope（仅 admin）
// @Summary 修改用户的行级权限
// @Description 仅 admin 可修改 team_user.data_scope，支持 1=全部 2=本部门 3=本人 4=自定义
// @Tags A域-行级权限
// @Accept json
// @Produce json
// @Param id path int true "用户 ID"
// @Param body body UpdateDataScopeRequest true "新数据范围"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/team/users/{id}/data-scope [put]
func (c *RowLevelSecurityController) UpdateUserDataScope(ctx *gin.Context) {
	role, _ := ctx.Get("role")
	roleStr, _ := role.(string)
	if roleStr != "admin" {
		response.Error(ctx, http.StatusForbidden, "仅管理员可修改数据范围")
		return
	}

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的用户 ID")
		return
	}

	var req UpdateDataScopeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	if !service.IsValidTeamDataScope(req.DataScope) {
		response.Error(ctx, http.StatusBadRequest, "data_scope 必须是 1/2/3/4 之一")
		return
	}

	// 取当前操作者上下文（service.Update 需 operatorID/operatorRole/operatorIP）
	operatorID, _ := ctx.Get("user_id")
	uid, _ := operatorID.(uint)

	dataScope := req.DataScope
	deptID := req.DepartmentID
	teamID := req.TeamID
	updateReq := &service.UpdateTeamUserRequest{
		DataScope:     &dataScope,
		DepartmentID:  &deptID,
		TeamID:        &teamID,
		CustomDeptIDs: req.CustomDeptIDs,
	}
	user, err := c.teamUserSvc.Update(uint(id), updateReq, uid, roleStr, ctx.ClientIP())
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, DataScopeResponse{
		UserID:        user.ID,
		DataScope:     user.DataScope,
		DataScopeName: c.rowLevelSvc.BuildScopeDescription(user.DataScope),
		DepartmentID:  user.DepartmentID,
		TeamID:        user.TeamID,
		CustomDeptIDs: user.CustomDeptIDs,
		IsAdmin:       roleStr == "admin",
	}, "数据范围已更新")
}
