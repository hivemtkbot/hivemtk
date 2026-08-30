package controller

import (
	"encoding/json"
	"fmt"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ManageDataExportController 管理端 GDPR 数据导出控制器
type ManageDataExportController struct {
	svc *service.DataExportService
}

// NewManageDataExportController 构造
func NewManageDataExportController() *ManageDataExportController {
	return &ManageDataExportController{svc: service.NewDataExportService()}
}

// Export GET /api/manage/data-export/:customer_id
// data 用 json.RawMessage 嵌入原始导出 JSON
func (c *ManageDataExportController) Export(ctx *gin.Context) {
	customerID := ctx.Param("customer_id")
	if customerID == "" {
		response.Error(ctx, 400, "customer_id 不能为空")
		return
	}
	data, err := c.svc.ExportJSON(ctx.Request.Context(), customerID)
	if HandleServiceError(ctx, err) {
		return
	}
	ctx.JSON(200, gin.H{
		"code":    0,
		"message": "ok",
		"data":    json.RawMessage(data),
		"_meta": gin.H{
			"customer_id": customerID,
			"size_bytes":  len(data),
			"generated_at": fmt.Sprintf("data_export:%s", customerID),
		},
	})
}
