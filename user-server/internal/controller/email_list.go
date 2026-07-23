package controller

import (
	"context"
	"bytes"
	"image"
	"image/png"
	"marketing/internal/dto"
	"marketing/internal/email/service"
	"marketing/internal/pkg/utils/response"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EmailListController 列表控制器
type EmailListController struct {
	svc *email.EmailListService
}

// NewEmailListController 创建列表控制器实例
func NewEmailListController() *EmailListController {
	return &EmailListController{svc: email.NewEmailListService()}
}

// CreateEmailList 创建列表
func (c *EmailListController) CreateEmailList(ctx *gin.Context) {
	var req dto.CreateEmailListRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	total, err := c.svc.CreateEmailList(context.Background(), req.Subject, req.Content, strings.Join(req.Attachments, ","))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, map[string]int64{
		"total": total,
	}, "success")
}

// GetEmailListList 获取列表列表
func (c *EmailListController) GetEmailListList(ctx *gin.Context) {
	var req dto.GetEmailListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	resp, err := c.svc.GetEmailListListDTO(context.Background(), req.Page, req.PageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, resp, "success")
}

// GetEmailListDetail 获取列表详情
func (c *EmailListController) GetEmailListDetail(ctx *gin.Context) {
	var req struct {
		ID string `uri:"id" binding:"required"`
	}
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	listID, err := uuid.Parse(req.ID)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := c.svc.GetEmailListByIDDTO(context.Background(), listID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, resp, "success")
}

// UpdateEmailList 更新列表
func (c *EmailListController) UpdateEmailList(ctx *gin.Context) {
	var req dto.UpdateEmailListRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := uuid.Parse(req.ID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.svc.UpdateEmailListDTO(context.Background(), req); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "success")
}

// DeleteEmailList 删除列表
func (c *EmailListController) DeleteEmailList(ctx *gin.Context) {
	var req struct {
		ID string `uri:"id" binding:"required"`
	}
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	listID, err := uuid.Parse(req.ID)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err = c.svc.DeleteEmailList(context.Background(), listID); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "success")
}

func (c *EmailListController) TraceEmail(ctx *gin.Context) {
	// 创建1x1透明图像
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))

	// 编码为PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		ctx.AbortWithStatus(500)
		return
	}

	// 返回PNG响应
	var req struct {
		TraceID string `uri:"trace_id" binding:"required"`
	}
	if err := ctx.ShouldBindUri(&req); err != nil {
		ctx.Data(200, "image/png", buf.Bytes())
		return
	}

	// 更新状态  打开状态  时间 统计次数
	traceID, err := uuid.Parse(req.TraceID)
	if err != nil {
		ctx.Data(200, "image/png", buf.Bytes())
		return
	}

	err = c.svc.UpdateEmailListReadInfo(context.Background(), traceID)
	if err != nil {
		ctx.Data(200, "image/png", buf.Bytes())
		return
	}

	ctx.Data(200, "image/png", buf.Bytes())
}
