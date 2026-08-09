package controller

import (
	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// LeadMiningController 线索发掘配置控制器
type LeadMiningController struct {
	cfgRepo repository.LeadMiningConfigRepository
}

func NewLeadMiningController() *LeadMiningController {
	return &LeadMiningController{cfgRepo: repository.NewLeadMiningConfigRepository()}
}

// GetConfig 读取线索发掘配置
func (c *LeadMiningController) GetConfig(ctx *gin.Context) {
	cfg, err := c.cfgRepo.GetSingleton(ctx)
	if err != nil {
		response.Error(ctx, 500, err.Error())
		return
	}
	response.Success(ctx, cfg, "success")
}

// SaveConfig 保存线索发掘配置（保存后热更新运行中的缓存）
func (c *LeadMiningController) SaveConfig(ctx *gin.Context) {
	var cfg model.LeadMiningConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		response.Error(ctx, 400, err.Error())
		return
	}
	if err := c.cfgRepo.Save(ctx, &cfg); err != nil {
		response.Error(ctx, 500, err.Error())
		return
	}
	service.ReloadConfigCache()
	response.Success(ctx, cfg, "success")
}
