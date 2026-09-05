package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupSSORoutes(public *gin.RouterGroup, gormDB *gorm.DB) {
	ssoCtrl := controller.NewSSOController(service.NewSSOService(gormDB))
	public.GET("/sso/providers", ssoCtrl.ListProviders)
	public.GET("/sso/login/:provider", ssoCtrl.Login)
	public.GET("/sso/callback/:provider", ssoCtrl.Callback)
}
