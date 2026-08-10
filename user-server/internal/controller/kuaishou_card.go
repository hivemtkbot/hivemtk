package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// KuaishouCardController 快手卡片控制器
type KuaishouCardController struct {
	service service.KuaishouCardService
}

// NewKuaishouCardController 创建快手卡片控制器实例
func NewKuaishouCardController(service service.KuaishouCardService) *KuaishouCardController {
	return &KuaishouCardController{
		service: service,
	}
}

// Create 创建快手卡片
func (c *KuaishouCardController) Create(ctx *gin.Context) {
	var req dto.KuaishouCardCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Create(ctx, &req)
	if HandleDBError(ctx, err, "创建快手卡片") {
		return
	}

	response.Success(ctx, card, response.ErrSuccess)
}

// Update 更新快手卡片
func (c *KuaishouCardController) Update(ctx *gin.Context) {
	var req dto.KuaishouCardUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 从 URI 参数获取 ID 并验证与 JSON 中的 ID 一致
	idStr := ctx.Param("id")
	idFromURI, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	// 验证 JSON 中的 ID 与 URI 中的 ID 一致
	if req.ID == 0 {
		req.ID = uint(idFromURI)
	} else if req.ID != uint(idFromURI) {
		response.Error(ctx, http.StatusBadRequest, response.ErrIDMismatch, "JSON 中的 ID 与 URI 参数中的 ID 不匹配")
		return
	}

	card, err := c.service.Update(ctx, &req)
	if HandleDBError(ctx, err, "更新快手卡片") {
		return
	}

	response.Success(ctx, card, response.ErrUpdateSuccess)
}

// Delete 删除快手卡片
func (c *KuaishouCardController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	err = c.service.Delete(ctx, uint(id))
	if HandleDBError(ctx, err, "删除快手卡片") {
		return
	}

	response.Success(ctx, nil, response.ErrDeleteSuccess)
}

// GetByID 根据ID获取快手卡片
func (c *KuaishouCardController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	card, err := c.service.GetByID(ctx, uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.ErrResourceNotFound, err.Error())
		return
	}

	response.Success(ctx, card, response.ErrGetSuccess)
}

// GetList 获取快手卡片列表
func (c *KuaishouCardController) GetList(ctx *gin.Context) {
	var req dto.KuaishouCardListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 设置默认值
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	list, err := c.service.GetList(ctx, &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrGetListFailed, err.Error())
		return
	}

	response.Success(ctx, list, response.ErrGetSuccess)
}

// ViewCard 浏览卡片
func (c *KuaishouCardController) ViewCard(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	err = c.service.ViewCard(ctx, uint(id))
	if HandleDBError(ctx, err, "浏览快手卡片") {
		return
	}

	response.Success(ctx, nil, response.ErrSuccess)
}

// LikeCard 点赞卡片
func (c *KuaishouCardController) LikeCard(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	err = c.service.LikeCard(ctx, uint(id))
	if HandleDBError(ctx, err, "点赞快手卡片") {
		return
	}

	response.Success(ctx, nil, response.ErrSuccess)
}

// ShareCard 分享卡片
func (c *KuaishouCardController) ShareCard(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	platform := ctx.Query("platform")
	if platform == "" {
		platform = "unknown"
	}

	card, err := c.service.ShareCard(ctx, uint(id), platform)
	if HandleDBError(ctx, err, "分享快手卡片") {
		return
	}

	response.Success(ctx, card, response.ErrSuccess)
}

// GenerateShortLink 生成短链接
func (c *KuaishouCardController) GenerateShortLink(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID", err.Error())
		return
	}

	// 获取卡片模型
	card, err := c.service.GetCardModelByID(ctx, uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.ErrResourceNotFound, err.Error())
		return
	}

	// 生成短链接
	err = c.service.GenerateShortLink(ctx, card)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrBusinessError, err.Error())
		return
	}

	// 重新获取卡片信息
	updatedCard, err := c.service.GetByID(ctx, uint(id))
	if HandleDBError(ctx, err, "获取快手卡片") {
		return
	}

	response.Success(ctx, updatedCard, response.ErrSuccess)
}

// SharePage 渲染卡片分享页面（统一卡片聊天页，含联系客服按钮）
func (c *KuaishouCardController) SharePage(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.String(http.StatusBadRequest, "无效的卡片ID")
		return
	}
	renderCardChatPage(ctx, c.service.GenerateCardChatPage, uint(id), buildBaseURL(ctx))
}
