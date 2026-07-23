package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupWeComRoutes 企业微信管理路由
func setupWeComRoutes(auth *gin.RouterGroup) {
	wecomCtrl := controller.NewWeComController(service.NewWeComServiceWithDB(db.GetDB()))

	// 企业微信账号管理
	auth.POST("/wecom/accounts", wecomCtrl.CreateAccount)
	auth.GET("/wecom/accounts", wecomCtrl.GetAccountList)
	auth.GET("/wecom/accounts/:id", wecomCtrl.GetAccountByID)
	auth.PUT("/wecom/accounts/:id", wecomCtrl.UpdateAccount)
	auth.DELETE("/wecom/accounts/:id", wecomCtrl.DeleteAccount)

	// 企业微信客户管理
	auth.POST("/wecom/accounts/:id/sync-customers", wecomCtrl.SyncCustomers)
	auth.GET("/wecom/customers", wecomCtrl.GetCustomerList)

	// 企业微信客户群管理
	auth.POST("/wecom/accounts/:id/sync-groups", wecomCtrl.SyncGroups)
	auth.GET("/wecom/groups", wecomCtrl.GetGroupList)

	// 企业微信消息管理
	auth.POST("/wecom/accounts/:id/send-message", wecomCtrl.SendMessage)
	auth.GET("/wecom/messages", wecomCtrl.GetMessageList)

	// 企业微信标签管理
	auth.GET("/wecom/tags", wecomCtrl.GetTagList)
	auth.POST("/wecom/accounts/:id/sync-tags", wecomCtrl.SyncTags)
}
