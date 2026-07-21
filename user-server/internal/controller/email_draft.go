package controller

import (
	"errors"
	"marketing/internal/dto"
	"marketing/internal/email/service"
	"marketing/internal/pkg/utils/response"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailDraftController 草稿控制器
type EmailDraftController struct {
	svc *email.EmailDraftService
}

// NewEmailDraftController 创建草稿控制器实例
func NewEmailDraftController() *EmailDraftController {
	return &EmailDraftController{svc: email.NewEmailDraftService()}
}

// CreateEmailDraft 创建草稿
func (c *EmailDraftController) CreateEmailDraft(ctx *gin.Context) {
	var req dto.CreateEmailDraftRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	resp, err := c.svc.CreateEmailDraftDTO(req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "创建失败："+err.Error())
		return
	}
	response.Success(ctx, resp, "创建成功")
}

// GetEmailDraftList 获取草稿列表
func (c *EmailDraftController) GetEmailDraftList(ctx *gin.Context) {
	resp, err := c.svc.GetEmailDraftListDTO()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取列表失败："+err.Error())
		return
	}
	response.Success(ctx, resp, "获取成功")
}

// GetEmailDraftDetail 获取草稿详情
func (c *EmailDraftController) GetEmailDraftDetail(ctx *gin.Context) {
	var req struct {
		ID string `uri:"id" binding:"required"`
	}
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	draftID, err := uuid.Parse(req.ID)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID 格式")
		return
	}

	resp, err := c.svc.GetEmailDraftByIDDTO(draftID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, "草稿不存在")
			return
		}
		response.Error(ctx, http.StatusInternalServerError, "获取详情失败："+err.Error())
		return
	}
	response.Success(ctx, resp, "获取成功")
}

// UpdateEmailDraft 更新草稿
func (c *EmailDraftController) UpdateEmailDraft(ctx *gin.Context) {
	var req dto.UpdateEmailDraftRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	if _, err := uuid.Parse(req.ID); err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID 格式")
		return
	}

	if err := c.svc.UpdateEmailDraftDTO(req); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "更新失败："+err.Error())
		return
	}
	response.Success(ctx, nil, "更新成功")
}

// DeleteEmailDraft 删除草稿
func (c *EmailDraftController) DeleteEmailDraft(ctx *gin.Context) {
	var req struct {
		ID string `uri:"id" binding:"required"`
	}
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	draftID, err := uuid.Parse(req.ID)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID 格式")
		return
	}

	if err = c.svc.DeleteEmailDraft(draftID); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "删除失败："+err.Error())
		return
	}
	response.Success(ctx, nil, "删除成功")
}
