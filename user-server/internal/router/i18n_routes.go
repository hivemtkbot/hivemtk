package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketing/internal/controller"
	"marketing/internal/repository"
	i18nservice "marketing/internal/service/i18n"
)

// ============================================================================
// 多语言方案路由（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 注册：
//   1. 术语表管理路由（/api/glossaries/*）
//   2. 监控看板路由（/api/i18n/stats/*）
//
// 私域独立部署：无 merchant_id，B 端 JWT 鉴权
// ============================================================================

// setupI18nRoutes 注册多语言方案相关路由（术语表 CRUD + 校验预览 + 监控看板）
func setupI18nRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 1. 术语表管理（/api/glossaries/*）
	glossaryRepo := repository.NewGlossaryRepositoryWithDB(db)
	glossarySvc := i18nservice.NewGlossaryService(glossaryRepo, nil)
	glossaryCtrl := controller.NewGlossaryController(glossarySvc)
	glossaryCtrl.RegisterRoutes(auth)

	// 2. 监控看板（/api/i18n/stats/*）
	statsRepo := repository.NewI18nStatsRepositoryWithDB(db)
	statsSvc := i18nservice.NewI18nStatsService(statsRepo)
	statsCtrl := controller.NewI18nStatsController(statsSvc)
	statsCtrl.RegisterRoutes(auth)
}
