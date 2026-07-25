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
// 注册术语表管理路由（/api/glossaries/*）
// 私域独立部署：无 merchant_id，B 端 JWT 鉴权
// ============================================================================

// setupI18nRoutes 注册多语言方案相关路由（术语表 CRUD + 校验预览）
func setupI18nRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	repo := repository.NewGlossaryRepositoryWithDB(db)
	svc := i18nservice.NewGlossaryService(repo, nil)
	ctrl := controller.NewGlossaryController(svc)
	ctrl.RegisterRoutes(auth)
}
