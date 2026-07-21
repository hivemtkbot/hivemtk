package controller

import (
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PaymentConfigController 支付配置控制器
type PaymentConfigController struct {
	svc *service.PaymentConfigService
}

// NewPaymentConfigController 创建支付配置控制器实例
func NewPaymentConfigController() *PaymentConfigController {
	return &PaymentConfigController{svc: service.NewPaymentConfigService()}
}

// GetConfig 获取支付配置
func (c *PaymentConfigController) GetConfig(ctx *gin.Context) {
	config, err := c.svc.GetConfig()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取支付配置失败: "+err.Error())
		return
	}

	response.Success(ctx, config, "获取成功")
}

// SaveConfig 保存支付配置
func (c *PaymentConfigController) SaveConfig(ctx *gin.Context) {
	var req service.PaymentConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	saved, err := c.svc.SaveConfig(&req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "保存支付配置失败: "+err.Error())
		return
	}

	response.Success(ctx, saved, "保存成功")
}
