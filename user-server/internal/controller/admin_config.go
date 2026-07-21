package controller

import (
	"marketing/internal/config"
	"marketing/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// AdminConfigController 管理员配置控制器
type AdminConfigController struct{}

// NewAdminConfigController 创建管理员配置控制器
func NewAdminConfigController() *AdminConfigController {
	return &AdminConfigController{}
}

// GetAdminConfig 获取管理员配置
func (c *AdminConfigController) GetAdminConfig(ctx *gin.Context) {
	adminConfig := config.GetAdminConfig()

	// 只返回非敏感信息给前端
	configInfo := map[string]any{
		"login": map[string]any{
			"show_default_credentials": adminConfig.Login.ShowDefaultCredentials,
			"default_credentials_hint": adminConfig.Login.DefaultCredentialsHint,
		},
		"auto_login": map[string]any{
			"enabled":           adminConfig.AutoLogin.Enabled,
			"use_default_admin": adminConfig.AutoLogin.UseDefaultAdmin,
		},
	}

	response.Success(ctx, configInfo, "获取管理员配置成功")
}

// GetDefaultCredentials 获取默认登录凭据（仅用于开发环境）
func (c *AdminConfigController) GetDefaultCredentials(ctx *gin.Context) {
	// 注意：这个功能应该只在开发环境中使用
	// 在生产环境中应该禁用此功能
	adminConfig := config.GetAdminConfig()

	if !adminConfig.AutoLogin.UseDefaultAdmin {
		response.Error(ctx, 403, "默认账户功能已禁用")
		return
	}

	credentials := map[string]string{
		"username": adminConfig.DefaultAdmin.Username,
		"password": adminConfig.DefaultAdmin.Password,
	}

	response.Success(ctx, credentials, "获取默认登录凭据成功")
}
