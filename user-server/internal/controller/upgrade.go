package controller

import (
	"marketing/internal/migration"
	"marketing/internal/migration/migrations"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UpgradeController 升级控制器
type UpgradeController struct {
	migrationService *migration.MigrationService
}

// NewUpgradeController 创建升级控制器实例
func NewUpgradeController() *UpgradeController {
	registry := migration.NewMigrationRegistry()

	return &UpgradeController{
		migrationService: migration.NewMigrationService(registry, db.GetDB(), migrations.RegisterMigrations),
	}
}

// GetUpgradeTask 获取升级任务详情
func (c *UpgradeController) GetUpgradeTask(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的升级任务 ID")
		return
	}

	task, err := c.migrationService.GetUpgradeTask(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, task, "获取成功")
}

// GetUpgradeHistory 获取升级历史
func (c *UpgradeController) GetUpgradeHistory(ctx *gin.Context) {
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	tasks, total, err := c.migrationService.GetUpgradeHistory(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      tasks,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetMigrationRecords 获取迁移记录
func (c *UpgradeController) GetMigrationRecords(ctx *gin.Context) {
	records, err := c.migrationService.GetMigrationRecords()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, records, "获取成功")
}

// GetCurrentVersion 获取当前版本
func (c *UpgradeController) GetCurrentVersion(ctx *gin.Context) {
	version, err := c.migrationService.GetCurrentVersion()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{"version": version}, "获取成功")
}

// CreateUpgradeTask 创建升级任务
func (c *UpgradeController) CreateUpgradeTask(ctx *gin.Context) {
	var req struct {
		ToVersion string `json:"to_version" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	// 获取当前版本
	currentVersion, err := c.migrationService.GetCurrentVersion()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// 创建升级任务
	task, err := c.migrationService.ExecuteUpgrade(currentVersion, req.ToVersion)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, task, "升级任务已创建")
}

// Rollback 回滚到指定版本
func (c *UpgradeController) Rollback(ctx *gin.Context) {
	var req struct {
		TargetVersion string `json:"target_version" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	if err := c.migrationService.Rollback(req.TargetVersion); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, nil, "回滚成功")
}

// GetAvailableUpgrades 获取可用升级列表
func (c *UpgradeController) GetAvailableUpgrades(ctx *gin.Context) {
	// 获取当前版本
	currentVersion, err := c.migrationService.GetCurrentVersion()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// 获取待执行的迁移
	pendingMigrations, err := c.migrationService.GetPendingMigrations()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	// 构造可用升级列表
	type AvailableUpgrade struct {
		Version     string `json:"version"`
		Name        string `json:"name"`
		Description string `json:"description"`
		IsNext      bool   `json:"is_next"`
	}

	var upgrades []AvailableUpgrade
	for i, m := range pendingMigrations {
		upgrades = append(upgrades, AvailableUpgrade{
			Version:     m.Version(),
			Name:        m.Name(),
			Description: m.Description(),
			IsNext:      i == 0,
		})
	}

	response.Success(ctx, gin.H{
		"current_version":    currentVersion,
		"available_upgrades": upgrades,
	}, "获取成功")
}
