// dnc_controller.go DNC 全局退订控制器（R51：合规核心功能此前无 API 暴露——业务缺陷修复）
package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// DNCController 全局退订控制器
type DNCController struct {
	svc *service.DoNotContactService
}

// NewDNCController 构造
func NewDNCController() *DNCController {
	svc := service.NewDoNotContactService(nil)
	return &DNCController{svc: svc}
}

// List GET /api/dnc?one_id=（one_id 空=全部）
func (c *DNCController) List(ctx *gin.Context) {
	oneID := ctx.Query("one_id")
	var list []*dncRow
	g := db.GetDB()
	q := g.WithContext(ctx).Table("customer_do_not_contact")
	if oneID != "" {
		q = q.Where("one_id = ?", oneID)
	}
	if err := q.Order("created_at DESC").Limit(200).Scan(&list).Error; HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

type dncRow struct {
	ID        int64  `json:"id"`
	OneID     string `json:"one_id"`
	Channel   string `json:"channel"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// Block POST /api/dnc {one_id, channel?, source?} — channel 空=全局
func (c *DNCController) Block(ctx *gin.Context) {
	var req struct {
		OneID   string `json:"one_id" binding:"required"`
		Channel string `json:"channel"`
		Source  string `json:"source"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "one_id 必填")
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	if err := c.svc.Block(ctx.Request.Context(), req.OneID, req.Channel, req.Source); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"blocked": true, "channel": req.Channel}, "已加入全局退订")
}

// BlockByPhone POST /api/dnc/block-phone {phone, source?}
func (c *DNCController) BlockByPhone(ctx *gin.Context) {
	var req struct {
		Phone  string `json:"phone" binding:"required"`
		Source string `json:"source"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "phone 必填")
		return
	}
	if err := c.svc.BlockFromPhone(ctx.Request.Context(), req.Phone, orDefault(req.Source, "manual")); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"blocked": true}, "已按手机号加入退订")
}

// Unblock DELETE /api/dnc/:one_id?channel=
func (c *DNCController) Unblock(ctx *gin.Context) {
	oneID := ctx.Param("one_id")
	channel := ctx.DefaultQuery("channel", "")
	if err := c.svc.Unblock(ctx.Request.Context(), oneID, channel); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, gin.H{"unblocked": true}, "已解除退订")
}

// IsBlocked GET /api/dnc/:one_id/blocked?channel=
func (c *DNCController) IsBlocked(ctx *gin.Context) {
	oneID := ctx.Param("one_id")
	blocked := c.svc.IsBlocked(ctx.Request.Context(), oneID, ctx.DefaultQuery("channel", ""))
	response.Success(ctx, gin.H{"one_id": oneID, "blocked": blocked}, "ok")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

var _ = db.GetDB
var _ = repository.NewCustomerDoNotContactRepository
