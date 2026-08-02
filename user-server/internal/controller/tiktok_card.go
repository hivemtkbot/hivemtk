package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

type TikTokCardController struct {
	svc service.TikTokCardService
}

// NewTikTokCardController 创建 TikTok 卡片控制器
//
// 五层架构合规：service 由 router 层注入（service.NewTikTokCardServiceWithDB），
// controller 不直接 import repository / db 包。
func NewTikTokCardController(svc service.TikTokCardService) *TikTokCardController {
	return &TikTokCardController{svc: svc}
}

func (ctrl *TikTokCardController) RegisterRoutes(router *gin.RouterGroup) {
	tiktok := router.Group("/tiktok-card")
	{
		tiktok.GET("/list", ctrl.List)
		tiktok.GET("/:id", ctrl.Get)
		tiktok.POST("", ctrl.Create)
		tiktok.PUT("/:id", ctrl.Update)
		tiktok.DELETE("/:id", ctrl.Delete)
		tiktok.POST("/generate-short-link", ctrl.GenerateShortLink)
		tiktok.GET("/stats/overall", ctrl.StatsOverall)
		tiktok.GET("/:id/stats", ctrl.Stats)
	}
}

func (ctrl *TikTokCardController) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	isActiveStr := c.Query("is_active")

	req := &dto.TikTokCardListRequest{
		Page:     page,
		PageSize: pageSize,
		Keyword:  keyword,
	}
	if isActiveStr != "" {
		v, err := strconv.ParseBool(isActiveStr)
		if err == nil {
			req.IsActive = &v
		}
	}

	list, err := ctrl.svc.GetList(c.Request.Context(), req)
	if err != nil {
		response.ErrorFromDB(c, err, "获取列表失败: "+err.Error())
		return
	}
	response.Success(c, list, "获取成功")
}

func (ctrl *TikTokCardController) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的卡片ID")
		return
	}

	card, err := ctrl.svc.GetByID(c.Request.Context(), uint(id))
	if err != nil {
		response.Error(c, http.StatusNotFound, "卡片不存在: "+err.Error())
		return
	}
	response.Success(c, card, "获取成功")
}

func (ctrl *TikTokCardController) Create(c *gin.Context) {
	var req dto.TikTokCardCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	card, err := ctrl.svc.Create(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFromDB(c, err, "创建失败: "+err.Error())
		return
	}
	response.Success(c, card, "创建成功")
}

func (ctrl *TikTokCardController) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的卡片ID")
		return
	}

	var req dto.TikTokCardUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	req.ID = uint(id)

	card, err := ctrl.svc.Update(c.Request.Context(), &req)
	if HandleDBError(c, err, "更新TikTok卡片") {
		return
	}
	response.Success(c, card, "更新成功")
}

func (ctrl *TikTokCardController) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的卡片ID")
		return
	}

	if HandleDBError(c, ctrl.svc.Delete(c.Request.Context(), uint(id)), "删除TikTok卡片") {
		return
	}
	response.Success(c, nil, "删除成功")
}

func (ctrl *TikTokCardController) GenerateShortLink(c *gin.Context) {
	var req struct {
		CardID uint `json:"cardId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	card, err := ctrl.svc.GenerateShortLink(c.Request.Context(), req.CardID)
	if HandleServiceError(c, err) {
		return
	}
	response.Success(c, gin.H{
		"short_link_url": card.ShortLinkURL,
		"short_code":     card.ShortCode,
		"card":           card,
	}, "生成成功")
}

func (ctrl *TikTokCardController) StatsOverall(c *gin.Context) {
	data, err := ctrl.svc.StatsOverall(c.Request.Context())
	if err != nil {
		response.ErrorFromDB(c, err, "获取统计失败: "+err.Error())
		return
	}
	response.Success(c, data, "获取成功")
}

func (ctrl *TikTokCardController) Stats(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的卡片ID")
		return
	}

	data, err := ctrl.svc.Stats(c.Request.Context(), uint(id))
	if HandleDBError(c, err, "获取TikTok卡片统计") {
		return
	}
	response.Success(c, data, "获取成功")
}
