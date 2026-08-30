// Package controller - GDPR DSAR 数据导出（G6）
package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DataExportController GDPR DSAR 数据导出 API
type DataExportController struct {
	svc *service.DataExportService
}

// NewDataExportController 创建实例
func NewDataExportController() *DataExportController {
	return &DataExportController{
		svc: service.NewDataExportService(),
	}
}

// Export GET /api/gdpr/export/:customer_id
// 返回 JSON 流（Content-Type: application/json，带缩进便于人工查看）
func (c *DataExportController) Export(ctx *gin.Context) {
	customerID := ctx.Param("customer_id")
	if customerID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "customer_id 不能为空",
		})
		return
	}

	data, err := c.svc.ExportJSON(ctx.Request.Context(), customerID)
	if err != nil {
		msg := err.Error()
		// 错误码映射: DSAR_001=参数错误, DSAR_002=客户不存在, 其余=内部错误
		switch {
		case strings.Contains(msg, "DSAR_001"):
			ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": msg})
		case strings.Contains(msg, "DSAR_002") || errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(msg, "不存在"):
			ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": msg})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": fmt.Sprintf("导出失败: %v", err)})
		}
		return
	}

	ctx.Header("Content-Type", "application/json; charset=utf-8")
	ctx.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="dsar_export_%s.json"`, customerID))
	ctx.Data(http.StatusOK, "application/json; charset=utf-8", data)

	// 防止 json 未使用警告
	_ = json.Marshal
}
