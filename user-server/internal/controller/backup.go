package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// BackupController 备份控制器
type BackupController struct {
	backupService *service.BackupService
}

// NewBackupController 创建备份控制器实例
func NewBackupController() *BackupController {
	return &BackupController{
		backupService: service.NewBackupService(),
	}
}

// isAdmin 校验当前用户是否为 admin（v3 审计 P2-30 修复）
func isAdmin(ctx *gin.Context) bool {
	role, exists := ctx.Get("role")
	if !exists {
		return false
	}
	roleStr, ok := role.(string)
	if !ok {
		return false
	}
	return roleStr == "admin"
}

// requireAdminAuth 统一守卫：先认证(401)后授权(403)。
// v3 审计 P2-30 + Round6 测试契约对齐：无 user_id → 401；有身份非 admin → 403。
func requireAdminAuth(ctx *gin.Context) (uint, bool) {
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return 0, false
	}
	uid, ok := userID.(uint)
	if !ok || uid == 0 {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return 0, false
	}
	if !isAdmin(ctx) {
		response.Error(ctx, http.StatusForbidden, "仅管理员可执行备份操作")
		return 0, false
	}
	return uid, true
}

// CreateBackup 创建备份
func (c *BackupController) CreateBackup(ctx *gin.Context) {
	uid, ok := requireAdminAuth(ctx)
	if !ok {
		return
	}
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	var req service.CreateBackupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	backup, err := c.backupService.CreateBackup(ctx.Request.Context(), uid, &req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, backup, "创建备份任务成功")
}

// GetBackupList 获取备份列表
func (c *BackupController) GetBackupList(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	if _, ok := userID.(uint); !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	backups, total, err := c.backupService.GetBackupList(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      backups,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetBackupByID 获取备份详情
func (c *BackupController) GetBackupByID(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	if _, ok := userID.(uint); !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的备份 ID")
		return
	}

	backup, err := c.backupService.GetBackupByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, backup, "获取成功")
}

// DeleteBackup 删除备份
func (c *BackupController) DeleteBackup(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	if _, ok := userID.(uint); !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的备份 ID")
		return
	}

	if HandleDBError(ctx, c.backupService.DeleteBackup(ctx.Request.Context(), uint(id)), "删除备份") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// RestoreController 恢复控制器
type RestoreController struct {
	restoreService *service.RestoreService
}

// NewRestoreController 创建恢复控制器实例
func NewRestoreController() *RestoreController {
	return &RestoreController{
		restoreService: service.NewRestoreService(),
	}
}

// RestoreBackup 恢复备份
func (c *RestoreController) RestoreBackup(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	uid, ok := userID.(uint)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	var req service.RestoreBackupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	record, err := c.restoreService.RestoreBackup(ctx.Request.Context(), uid, &req)
	if HandleDBError(ctx, err, "恢复备份") {
		return
	}

	response.Success(ctx, record, "创建恢复任务成功")
}

// GetRestoreList 获取恢复记录列表
func (c *RestoreController) GetRestoreList(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	if _, ok := userID.(uint); !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	records, total, err := c.restoreService.GetRestoreList(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetLastRestore 获取最近一次恢复记录
func (c *RestoreController) GetLastRestore(ctx *gin.Context) {

	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未找到用户信息")
		return
	}
	if _, ok := userID.(uint); !ok {
		response.Error(ctx, http.StatusUnauthorized, "无效的用户信息")
		return
	}

	record, err := c.restoreService.GetLastRestore(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, record, "获取成功")
}

