package controller

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// LeadMiningController 线索发掘配置控制器
//
// 五层架构 L3：controller 只解析入参/调 service/序列化响应，
// 不直接引用 repository / model（配置读写统一走 service 门面）。
type LeadMiningController struct{}

func NewLeadMiningController() *LeadMiningController {
	return &LeadMiningController{}
}

// GetConfig 读取线索发掘配置
func (c *LeadMiningController) GetConfig(ctx *gin.Context) {
	cfg, err := service.GetLeadMiningConfig(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, 500, err.Error())
		return
	}
	response.Success(ctx, cfg, "success")
}

// SaveConfig 保存线索发掘配置（保存后热更新运行中的缓存）
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
