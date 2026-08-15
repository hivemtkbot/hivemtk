package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupSSORoutes 企业 SSO 登录路由（公开，无需登录态）
//
// 路由清单（与 service.SSOService.ListProviders 的 LoginURL 对齐）：
//   - GET /api/sso/providers        列出已启用登录方式
//   - GET /api/sso/login/:provider  发起登录（302 到 IdP）
//   - GET /api/sso/callback/:provider 处理 IdP 回调（签发本地 JWT）
func setupSSORoutes(public *gin.RouterGroup, gormDB *gorm.DB) {
	ssoCtrl := controller.NewSSOController(gormDB)
	public.GET("/sso/providers", ssoCtrl.ListProviders)
	public.GET("/sso/login/:provider", ssoCtrl.Login)
	public.GET("/sso/callback/:provider", ssoCtrl.Callback)
}

