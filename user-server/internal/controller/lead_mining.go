package controller

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

type LeadMiningController struct{}

func NewLeadMiningController() *LeadMiningController {
	return &LeadMiningController{}
}

func (c *LeadMiningController) GetConfig(ctx *gin.Context) {
	cfg, err := service.GetLeadMiningConfig(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, 500, err.Error())
		return
	}
	response.Success(ctx, cfg, "success")
}

func (c *LeadMiningController) GetStatus(ctx *gin.Context) {
	status := service.GetStatus()
	response.Success(ctx, status, "success")
}

func (c *LeadMiningController) SaveConfig(ctx *gin.Context) {
	var cfg dto.LeadMiningConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		response.Error(ctx, 400, err.Error())
		return
	}
	if err := service.SaveLeadMiningConfig(ctx.Request.Context(), &cfg); err != nil {
		response.Error(ctx, 500, err.Error())
		return
	}
	response.Success(ctx, cfg, "success")
}
