// card_cross_publish.go 名片跨平台同步发布（R39 /api/cards/cross-publish）
//
// 语义：一份卡片素材（标题/描述/图/跳转）同步创建到多个平台卡片体系；
// 单平台失败不阻断其余平台，逐平台返回结果（部分成功语义）。
package controller

import (
	"errors"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// CardCrossPublishController 跨平台发布控制器
type CardCrossPublishController struct {
	douyin     service.DouyinCardService
	kuaishou   service.KuaishouCardService
	xiaohongshu service.XiaohongshuCardService
	xianyu     service.XianyuCardService
}

// NewCardCrossPublishController 构造（DI 在路由装配完成）
func NewCardCrossPublishController(douyin service.DouyinCardService, kuaishou service.KuaishouCardService, xiaohongshu service.XiaohongshuCardService, xianyu service.XianyuCardService) *CardCrossPublishController {
	return &CardCrossPublishController{douyin: douyin, kuaishou: kuaishou, xiaohongshu: xiaohongshu, xianyu: xianyu}
}

// CrossPublish POST /api/cards/cross-publish
// body: { platforms: ["douyin","kuaishou",...], data: {title, description, image_url, redirect_url, tags, is_active} }
func (c *CardCrossPublishController) CrossPublish(ctx *gin.Context) {
	var req struct {
		Platforms []string `json:"platforms"`
		Platform  string   `json:"platform"` // 单平台快捷字段
		Data      struct {
			Title       string `json:"title" binding:"required"`
			Description string `json:"description"`
			ImageURL    string `json:"image_url"`
			RedirectURL string `json:"redirect_url"`
			Tags        string `json:"tags"`
			IsActive    *bool  `json:"is_active"`
		} `json:"data" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "请求参数错误："+err.Error())
		return
	}
	if len(req.Platforms) == 0 && req.Platform != "" {
		req.Platforms = []string{req.Platform}
	}
	if len(req.Platforms) == 0 {
		req.Platforms = []string{"douyin", "kuaishou", "xiaohongshu", "xianyu"}
	}
	active := true
	if req.Data.IsActive != nil {
		active = *req.Data.IsActive
	}

	results := []map[string]any{}
	for _, p := range req.Platforms {
		entry := map[string]any{"platform": p}
		var err error
		switch p {
		case "douyin":
			card, e := c.douyin.Create(ctx.Request.Context(), &dto.DouyinCardCreateRequest{
				Title: req.Data.Title, Description: req.Data.Description,
				ImageURL: req.Data.ImageURL, RedirectURL: req.Data.RedirectURL,
				Tags: req.Data.Tags, IsActive: active,
			})
			if e == nil {
				entry["card_id"] = card.ID
			}
			err = e
		case "kuaishou":
			card, e := c.kuaishou.Create(ctx.Request.Context(), &dto.KuaishouCardCreateRequest{
				Title: req.Data.Title, Description: req.Data.Description,
				ImageURL: req.Data.ImageURL, RedirectURL: req.Data.RedirectURL,
				Tags: req.Data.Tags, IsActive: active,
			})
			if e == nil {
				entry["card_id"] = card.ID
			}
			err = e
		case "xiaohongshu":
			card, e := c.xiaohongshu.Create(ctx.Request.Context(), &dto.XiaohongshuCardCreateRequest{
				Title: req.Data.Title, Description: req.Data.Description,
				ImageURL: req.Data.ImageURL, RedirectURL: req.Data.RedirectURL,
				Tags: req.Data.Tags, IsActive: active,
			})
			if e == nil {
				entry["card_id"] = card.ID
			}
			err = e
		case "xianyu":
			card, e := c.xianyu.Create(ctx.Request.Context(), &dto.XianyuCardCreateRequest{
				Title: req.Data.Title, Description: req.Data.Description,
				ImageURL: req.Data.ImageURL, RedirectURL: req.Data.RedirectURL,
				Tags: req.Data.Tags, IsActive: active,
			})
			if e == nil {
				entry["card_id"] = card.ID
			}
			err = e
		default:
			err = errors.New("不支持的平台: " + p)
		}
		if err != nil {
			entry["success"] = false
			entry["error"] = err.Error()
		} else {
			entry["success"] = true
		}
		results = append(results, entry)
	}
	okCount := 0
	for _, r := range results {
		if r["success"] == true {
			okCount++
		}
	}
	response.Success(ctx, gin.H{"results": results, "succeeded": okCount, "total": len(results)}, "跨平台发布完成")
}
