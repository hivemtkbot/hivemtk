package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupConfigParamRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	svc := service.GlobalConfigParam()
	ctrl := controller.NewConfigParamController(svc)

	auth.GET("/manage/config-params", ctrl.List)
	auth.GET("/manage/config-params/audit-logs", ctrl.AuditLogs)
	auth.PUT("/manage/config-params/:group/:key", ctrl.Update)
	auth.POST("/manage/config-params/:group/:key/reset", ctrl.ResetToDefault)
	auth.POST("/manage/config-params/:group/reset", ctrl.BulkResetGroup)
}
