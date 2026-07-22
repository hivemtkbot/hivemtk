// Package controller 提供 AssetBundle（资产包）的 HTTP 路由。
//
// 方向9：资产包模式
// 文档依据：docs/企业级架构优化/资产包模式.md
//
// 设计原则：
//  1. 严格按五层架构：Controller 仅做 HTTP 适配，业务放 Service
//  2. 入参绑定 DTO，出参 JSON
//  3. 不直接持有 db / repository（全部由 service 屏蔽）
package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/service"
)

// AssetBundleController 资产包 HTTP 控制器
type AssetBundleController struct {
	svc *service.AssetBundleService
}

// NewAssetBundleController 构造控制器
func NewAssetBundleController(svc *service.AssetBundleService) *AssetBundleController {
	return &AssetBundleController{svc: svc}
}

// Register 注册路由
//
// 路由：
//
//	POST   /api/asset-bundle              创建
//	PUT    /api/asset-bundle/:id          更新
//	GET    /api/asset-bundle/:id          查询
//	GET    /api/asset-bundle/by-aid/:aid  按 AssetID 查询
//	POST   /api/asset-bundle/list         分页
//	POST   /api/asset-bundle/:id/publish  启用
//	POST   /api/asset-bundle/:id/archive  归档
//	DELETE /api/asset-bundle/:id          软删
//	POST   /api/asset-bundle/weave        Weave 织布
//	POST   /api/asset-bundle/merchant-save 商户表单保存
//	POST   /api/asset-bundle/merchant-parse/:aid 商户表单解析
func (c *AssetBundleController) Register(rg *gin.RouterGroup) {
	g := rg.Group("/asset-bundle")
	g.POST("", c.Create)
	g.PUT("/:id", c.Update)
	g.GET("/:id", c.Get)
	g.GET("/by-aid/:aid", c.GetByAssetID)
	g.POST("/list", c.List)
	g.POST("/:id/publish", c.Publish)
	g.POST("/:id/archive", c.Archive)
	g.DELETE("/:id", c.Delete)
	g.POST("/weave", c.Weave)
	g.POST("/merchant-save", c.MerchantSave)
	g.POST("/merchant-parse/:aid", c.MerchantParse)
}

// Create 创建
func (c *AssetBundleController) Create(ctx *gin.Context) {
	var req dto.AssetBundleCreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bundle := &model.AssetBundle{
		AssetID:     req.AssetID,
		Title:       req.Title,
		Description: req.Description,
		Author:      req.Author,
		Version:     req.Version,
		Scope:       req.Scope,
		Industry:    req.Industry,
		Language:    req.Language,
		Tags:        req.Tags,
		Messages:    req.Messages,
	}
	if err := c.svc.CreateBundle(ctx.Request.Context(), bundle); err != nil {
		logger.Errorf("[asset-bundle] create failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": bundle})
}

// Update 更新
func (c *AssetBundleController) Update(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req dto.AssetBundleUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ID = id
	bundle := &model.AssetBundle{
		ID:          req.ID,
		Title:       req.Title,
		Description: req.Description,
		Author:      req.Author,
		Version:     req.Version,
		Scope:       req.Scope,
		Industry:    req.Industry,
		Language:    req.Language,
		Tags:        req.Tags,
		Messages:    req.Messages,
		Status:      req.Status,
	}
	if err := c.svc.UpdateBundle(ctx.Request.Context(), bundle); err != nil {
		logger.Errorf("[asset-bundle] update failed: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": bundle})
}

// Get 按 ID 查
func (c *AssetBundleController) Get(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bundle, err := c.svc.GetBundle(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": bundle})
}

// GetByAssetID 按业务键查
func (c *AssetBundleController) GetByAssetID(ctx *gin.Context) {
	aid := ctx.Param("aid")
	bundle, err := c.svc.GetBundleByAssetID(ctx.Request.Context(), aid)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": bundle})
}

// List 分页
func (c *AssetBundleController) List(ctx *gin.Context) {
	var req dto.AssetBundleListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		// query 绑定失败时尝试 JSON 绑定
		if err2 := ctx.ShouldBindJSON(&req); err2 != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	filter := repository.AssetBundleFilter{
		Keyword:  req.Keyword,
		Author:   req.Author,
		Industry: req.Industry,
		Language: req.Language,
		Scope:    req.Scope,
		Status:   req.Status,
		Tags:     req.Tags,
		Page:     req.Page,
		Size:     req.Size,
	}
	list, total, err := c.svc.ListBundles(ctx.Request.Context(), filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": dto.AssetBundleListResponse{
		List: list, Total: total, Page: filter.Page, Size: filter.Size,
	}})
}

// Publish 启用
func (c *AssetBundleController) Publish(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := c.svc.PublishBundle(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// Archive 归档
func (c *AssetBundleController) Archive(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := c.svc.ArchiveBundle(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// Delete 软删
func (c *AssetBundleController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := c.svc.DeleteBundle(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// Weave 织布算法
func (c *AssetBundleController) Weave(ctx *gin.Context) {
	var req dto.WeaveRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// DTO → Service 层 WeaveInput 转换
	in := service.WeaveInput{
		UserQuery:    req.UserQuery,
		ChatHistory:  req.ChatHistory,
		MerchantVars: req.MerchantVars,
	}
	for _, d := range req.RAGDocs {
		in.RAGDocs = append(in.RAGDocs, service.RAGDocument{
			ID: d.ID, Title: d.Title, Content: d.Content, Score: d.Score, Source: d.Source,
		})
	}
	if req.Options != nil {
		in.Options = service.WeaveOptions{
			RAGPosition:         service.RAGInsertPosition(req.Options.RAGPosition),
			MaxHistoryMessages:  req.Options.MaxHistoryMessages,
			StripFewShotJSON:    req.Options.StripFewShotJSON,
			IncludeMerchantVars: req.Options.IncludeMerchantVars,
		}
	}
	msgs, err := c.svc.WeaveForRequest(ctx.Request.Context(), req.AssetID, req.UserQuery, in)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 统计
	stats := dto.WeaveStats{
		AssetMessages:    len(in.Asset.Messages),
		RAGMessages:      0,
		HistoryMessages:  len(in.ChatHistory),
		FinalTotal:       len(msgs),
		StrippedFewShots: 0,
	}
	if len(req.RAGDocs) > 0 {
		stats.RAGMessages = 1
	}
	ctx.JSON(http.StatusOK, gin.H{"data": dto.WeaveResponse{
		Messages:     msgs,
		ResultLength: len(msgs),
		Stats:        stats,
	}})
}

// MerchantSave 商户低代码表单保存（把表单反向翻译成标准 messages 数组）
func (c *AssetBundleController) MerchantSave(ctx *gin.Context) {
	var req dto.MerchantFormSaveRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bundle, err := service.BuildBundleFromMerchantForm(req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 如果已存在则更新，否则创建
	existing, _ := c.svc.GetBundleByAssetID(ctx.Request.Context(), req.AssetID)
	if existing != nil {
		bundle.ID = existing.ID
		if err := c.svc.UpdateBundle(ctx.Request.Context(), bundle); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := c.svc.CreateBundle(ctx.Request.Context(), bundle); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"data": bundle})
}

// MerchantParse 解析 messages 数组为商户表单（前端回显用）
func (c *AssetBundleController) MerchantParse(ctx *gin.Context) {
	aid := ctx.Param("aid")
	bundle, err := c.svc.GetBundleByAssetID(ctx.Request.Context(), aid)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	resp := service.ParseBundleToMerchantForm(bundle)
	ctx.JSON(http.StatusOK, gin.H{"data": resp})
}
