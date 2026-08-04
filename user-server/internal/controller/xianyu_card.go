package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// XianyuCardController 闲鱼卡片控制器
type XianyuCardController struct {
	service  service.XianyuCardService
	statsSvc service.XianyuCardStatsService
}

// NewXianyuCardController 创建闲鱼卡片控制器实例
// statsSvc 在构造时注入，避免 Controller 方法内直接访问数据库（架构违规）
func NewXianyuCardController(service service.XianyuCardService, statsSvc service.XianyuCardStatsService) *XianyuCardController {
	return &XianyuCardController{
		service:  service,
		statsSvc: statsSvc,
	}
}

// Create 创建闲鱼卡片
func (c *XianyuCardController) Create(ctx *gin.Context) {
	var req dto.XianyuCardCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Create(ctx, &req)
	if HandleDBError(ctx, err, "创建闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrSuccess)
}

// Update 更新闲鱼卡片
func (c *XianyuCardController) Update(ctx *gin.Context) {
	var req dto.XianyuCardUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	card, err := c.service.Update(ctx, &req)
	if HandleDBError(ctx, err, "更新闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrUpdateSuccess)
}

// Delete 删除闲鱼卡片
func (c *XianyuCardController) Delete(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	if HandleDBError(ctx, c.service.Delete(ctx, uint(id)), "删除闲鱼卡片") {
		return
	}

	response.Success(ctx, gin.H{"id": id}, response.ErrDeleteSuccess)
}

// GetByID 根据ID获取闲鱼卡片
func (c *XianyuCardController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	card, err := c.service.GetByID(ctx, uint(id))
	if HandleDBError(ctx, err, "获取闲鱼卡片") {
		return
	}

	response.Success(ctx, card, response.ErrGetSuccess)
}

// GetList 获取闲鱼卡片列表
func (c *XianyuCardController) GetList(ctx *gin.Context) {
	var req dto.XianyuCardListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidParams, err.Error())
		return
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 10
	}

	cards, err := c.service.GetList(ctx, &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, response.ErrGetListFailed, err.Error())
		return
	}

	response.Success(ctx, cards, response.ErrGetSuccess)
}

// GenerateShortLink 生成短链
func (c *XianyuCardController) GenerateShortLink(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}

	// 获取卡片信息
	card, err := c.service.GetByIDWithRefresh(ctx, uint(id))
	if HandleDBError(ctx, err, "获取闲鱼卡片") {
		return
	}

	response.Success(ctx, gin.H{
		"short_link_url": card.ShortLinkURL,
		"short_code":     card.ShortCode,
	}, response.ErrSuccess)
}

// ViewCard 记录浏览（GET方式）
func (c *XianyuCardController) ViewCard(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}
	ip := ctx.ClientIP()
	ua := ctx.GetHeader("User-Agent")
	ref := ctx.GetHeader("Referer")
	if HandleDBError(ctx, c.statsSvc.RecordView(ctx, uint(id), ip, ua, ref), "记录浏览") {
		return
	}
	response.Success(ctx, gin.H{"id": id}, response.ErrSuccess)
}

// PostRecordView 记录浏览（POST方式，模板页上报）
func (c *XianyuCardController) PostRecordView(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}
	var payload struct {
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		Referer   string `json:"referer"`
	}
	_ = ctx.ShouldBindJSON(&payload)
	ip := payload.IP
	if ip == "" {
		ip = ctx.ClientIP()
	}
	ua := payload.UserAgent
	if ua == "" {
		ua = ctx.GetHeader("User-Agent")
	}
	ref := payload.Referer
	if ref == "" {
		ref = ctx.GetHeader("Referer")
	}
	if HandleDBError(ctx, c.statsSvc.RecordView(ctx, uint(id), ip, ua, ref), "记录浏览") {
		return
	}
	response.Success(ctx, gin.H{"id": id}, response.ErrSuccess)
}

// PostRecordClick 记录点击（POST方式，模板页上报）
func (c *XianyuCardController) PostRecordClick(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}
	var payload struct {
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		Referer   string `json:"referer"`
	}
	_ = ctx.ShouldBindJSON(&payload)
	ip := payload.IP
	if ip == "" {
		ip = ctx.ClientIP()
	}
	ua := payload.UserAgent
	if ua == "" {
		ua = ctx.GetHeader("User-Agent")
	}
	ref := payload.Referer
	if ref == "" {
		ref = ctx.GetHeader("Referer")
	}
	if HandleDBError(ctx, c.statsSvc.RecordClick(ctx, uint(id), ip, ua, ref), "记录点击") {
		return
	}
	response.Success(ctx, gin.H{"id": id}, response.ErrSuccess)
}

// PostRecordShare 记录分享（POST方式，模板页上报）
func (c *XianyuCardController) PostRecordShare(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.ErrInvalidIDFormat, err.Error())
		return
	}
	var payload struct {
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		Referer   string `json:"referer"`
	}
	_ = ctx.ShouldBindJSON(&payload)
	ip := payload.IP
	if ip == "" {
		ip = ctx.ClientIP()
	}
	ua := payload.UserAgent
	if ua == "" {
		ua = ctx.GetHeader("User-Agent")
	}
	ref := payload.Referer
	if ref == "" {
		ref = ctx.GetHeader("Referer")
	}
	if HandleDBError(ctx, c.statsSvc.RecordShare(ctx, uint(id), ip, ua, ref), "记录分享") {
		return
	}
	response.Success(ctx, gin.H{"id": id}, response.ErrSuccess)
}

// SharePage 渲染卡片分享页面（统一卡片聊天页，含联系客服按钮）
func (c *XianyuCardController) SharePage(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.String(http.StatusBadRequest, "无效的卡片ID")
		return
	}
	renderCardChatPage(ctx, c.service.GenerateCardChatPage, uint(id), buildBaseURL(ctx))
}
