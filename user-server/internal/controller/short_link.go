package controller

import (
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ShortLinkController 短链控制器
type ShortLinkController struct {
	shortLinkService service.ShortLinkService
}

// NewShortLinkController 创建短链控制器实例
func NewShortLinkController(shortLinkService service.ShortLinkService) *ShortLinkController {
	return &ShortLinkController{
		shortLinkService: shortLinkService,
	}
}

// Create 创建短链
func (c *ShortLinkController) Create(ctx *gin.Context) {
	var req dto.CreateShortLinkRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := c.shortLinkService.Create(&req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, resp, "创建成功")
}

// Update 更新短链
func (c *ShortLinkController) Update(ctx *gin.Context) {
	var req dto.UpdateShortLinkRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	// 从URI参数获取ID并验证与JSON中的ID一致
	idStr := ctx.Param("id")
	idFromURI, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID格式")
		return
	}

	// 验证JSON中的ID与URI中的ID一致
	if req.ID != uint(idFromURI) {
		response.Error(ctx, http.StatusBadRequest, "ID不一致: JSON中的ID与URI参数中的ID不匹配")
		return
	}

	resp, err := c.shortLinkService.Update(&req)
	if HandleDBError(ctx, err, "更新短链") {
		return
	}

	response.Success(ctx, resp, "更新成功")
}

// Delete 删除短链
func (c *ShortLinkController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	err = c.shortLinkService.Delete(uint(id))
	if HandleDBError(ctx, err, "删除短链") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetByID 根据ID获取短链
func (c *ShortLinkController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}

	resp, err := c.shortLinkService.GetByID(uint(id))
	if HandleDBError(ctx, err, "获取短链") {
		return
	}

	response.Success(ctx, resp, "获取成功")
}

// GetList 获取短链列表
func (c *ShortLinkController) GetList(ctx *gin.Context) {
	var req dto.ListShortLinkRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := c.shortLinkService.GetList(&req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, resp, "获取成功")
}

// AccessShortLink 访问短链
func (c *ShortLinkController) AccessShortLink(ctx *gin.Context) {
	var req dto.AccessShortLinkRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := c.shortLinkService.AccessShortLink(&req)
	if HandleDBError(ctx, err, "访问短链") {
		return
	}

	response.Success(ctx, resp, "访问成功")
}

// GenerateShortCode 生成短码
func (c *ShortLinkController) GenerateShortCode(ctx *gin.Context) {
	var req dto.GenerateShortCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	resp, err := c.shortLinkService.GenerateShortCode(&req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, resp, "生成成功")
}
