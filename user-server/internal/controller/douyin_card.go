package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// DouyinCardController 抖音卡片控制器
type DouyinCardController struct {
	service service.DouyinCardService
}

// NewDouyinCardController 创建抖音卡片控制器实例
func NewDouyinCardController(service service.DouyinCardService) *DouyinCardController {
	return &DouyinCardController{
		service: service,
	}
}

// Create 创建抖音卡片
func (c *DouyinCardController) Create(ctx *gin.Context) {
	var req dto.DouyinCardCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Create(ctx, &req)
	if HandleDBError(ctx, err, "创建抖音卡片") {
		return
	}

	response.Success(ctx, card, response.ErrCreateSuccess)
}

// Update 更新抖音卡片
func (c *DouyinCardController) Update(ctx *gin.Context) {
	var req dto.DouyinCardUpdateRequest
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
	if HandleDBError(ctx, err, "更新抖音卡片") {
		return
	}

	response.Success(ctx, card, response.ErrUpdateSuccess)
}

// Delete 删除抖音卡片
func (c *DouyinCardController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	if HandleDBError(ctx, c.service.Delete(ctx, uint(id)), "删除抖音卡片") {
		return
	}

	response.Success(ctx, nil, response.ErrDeleteSuccess)
}

// GetByID 根据 ID 获取抖音卡片
func (c *DouyinCardController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	card, err := c.service.GetByID(ctx, uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.ErrResourceNotFound, err.Error())
		return
	}

	response.Success(ctx, card, response.ErrGetSuccess)
}

// GetList 获取抖音卡片列表
func (c *DouyinCardController) GetList(ctx *gin.Context) {
	var req dto.DouyinCardListRequest
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

// GenerateShortLink 为抖音卡片生成短链
func (c *DouyinCardController) GenerateShortLink(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	// 获取卡片模型
	card, err := c.service.GetCardModelByID(ctx, uint(id))
	if HandleDBError(ctx, err, "获取抖音卡片") {
		return
	}

	// 生成短链
	if HandleDBError(ctx, c.service.GenerateShortLink(ctx, card), "生成短链") {
		return
	}

	// 重新获取卡片信息，确保获取最新数据
	cardResp, err := c.service.GetByIDWithRefresh(ctx, uint(id))
	if HandleDBError(ctx, err, "获取抖音卡片") {
		return
	}

	response.Success(ctx, cardResp, response.ErrSuccess)
}

// SharePage 渲染卡片分享页面（统一卡片聊天页，含联系客服按钮）
func (c *DouyinCardController) SharePage(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.String(http.StatusBadRequest, "无效的卡片ID")
		return
	}
	renderCardChatPage(ctx, c.service.GenerateCardChatPage, uint(id), buildBaseURL(ctx))
}
