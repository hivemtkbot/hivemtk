// sales_cockpit_controller.go 驾驶舱控制器（五层 L2）
package controller

import (
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// SalesCockpitController 驾驶舱控制器
type SalesCockpitController struct {
	svc *service.SalesCockpitService
}

// NewSalesCockpitController 构造
func NewSalesCockpitController() *SalesCockpitController {
	return &SalesCockpitController{svc: service.NewSalesCockpitService()}
}

// GetCockpit GET /api/ai/sales-cockpit
func (c *SalesCockpitController) GetCockpit(ctx *gin.Context) {
	data, err := c.svc.GetCockpit(ctx.Request.Context())
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, data, "ok")
}
