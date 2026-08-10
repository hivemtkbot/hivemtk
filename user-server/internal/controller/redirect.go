package controller

import (
	"context"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// RedirectController 短链重定向控制器
//
// 私域部署（升级）：
//   - 抖音 / 快手 / 小红书 / 闲鱼 四个平台的卡片短链统一跳转到「卡片聊天页」
//   - 卡片聊天页包含卡片信息 + 联系客服按钮，点击按钮打开 /chat/embed/{platform}_card_{id}
//   - 不再直接 302 跳转到外部 redirect_url，避免跳出客服域
//
// 五层架构修复：所有 service 由 router 注入，controller 不再直接访问数据库
type RedirectController struct {
	shortLinkService       service.ShortLinkService
	douyinCardService      service.DouyinCardService
	kuaishouCardService    service.KuaishouCardService
	xiaohongshuCardService service.XiaohongshuCardService
	xianyuCardService      service.XianyuCardService
}

// NewRedirectController 创建短链重定向控制器（service 由 router 注入）
func NewRedirectController(
	shortLinkService service.ShortLinkService,
	douyinCardService service.DouyinCardService,
	kuaishouCardService service.KuaishouCardService,
	xiaohongshuCardService service.XiaohongshuCardService,
	xianyuCardService service.XianyuCardService,
) *RedirectController {
	return &RedirectController{
		shortLinkService:       shortLinkService,
		douyinCardService:      douyinCardService,
		kuaishouCardService:    kuaishouCardService,
		xiaohongshuCardService: xiaohongshuCardService,
		xianyuCardService:      xianyuCardService,
	}
}

// RedirectShortLink 重定向短链
func (ctrl *RedirectController) RedirectShortLink(ctx *gin.Context) {
	code := ctx.Param("code")
	if code == "" {
		ctx.String(http.StatusBadRequest, "无效的短链")
		return
	}

	// 根据短码获取短链
	shortLink, err := ctrl.shortLinkService.GetByShortCode(context.Background(), code)
	if err != nil {
		ctx.String(http.StatusNotFound, "短链不存在")
		return
	}

	// 记录访问（best-effort：不阻断重定向主流程，但记录错误以便排查）
	if _, err := ctrl.shortLinkService.AccessShortLink(context.Background(), &dto.AccessShortLinkRequest{
		ShortCode: code,
		UserAgent: ctx.GetHeader("User-Agent"),
		IP:        ctx.ClientIP(),
		Referer:   ctx.GetHeader("Referer"),
	}); err != nil {
		logger.Errorf("[RedirectShortLink] 记录短链访问失败（code=%s）: %v", code, err)
	}

	// 构造站点根地址，用于生成绝对 URL（embed chat 链接可被外部直接打开）
	baseURL := buildBaseURL(ctx)

	// 按平台分发：根据 OriginalURL 的内部路径模式识别平台
	originalURL := shortLink.OriginalURL

	// 抖音卡片：/douyin/card/{id}
	if id, ok := extractCardID(originalURL, "/douyin/card/"); ok {
		renderCardChatPage(ctx, ctrl.douyinCardService.GenerateCardChatPage, id, baseURL)
		return
	}

	// 快手卡片：/kuaishou/card/{id}
	if id, ok := extractCardID(originalURL, "/kuaishou/card/"); ok {
		renderCardChatPage(ctx, ctrl.kuaishouCardService.GenerateCardChatPage, id, baseURL)
		return
	}

	// 小红书卡片：/xiaohongshu/card/{id}
	if id, ok := extractCardID(originalURL, "/xiaohongshu/card/"); ok {
		renderCardChatPage(ctx, ctrl.xiaohongshuCardService.GenerateCardChatPage, id, baseURL)
		return
	}

	// 闲鱼卡片：/xianyu/card/{id}
	if id, ok := extractCardID(originalURL, "/xianyu/card/"); ok {
		renderCardChatPage(ctx, ctrl.xianyuCardService.GenerateCardChatPage, id, baseURL)
		return
	}

	// 兼容旧数据：闲鱼卡片短链此前直接使用 card.RedirectURL 作为 OriginalURL
	// 这里无法判断平台，只能按外链处理（302 跳转）
	target := originalURL
	if u, err := url.Parse(target); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		ctx.String(http.StatusBadRequest, "非法的跳转地址")
		return
	}
	ctx.Redirect(http.StatusMovedPermanently, target)
}

// cardChatPageGenerator 卡片聊天页生成函数签名（四平台统一）
type cardChatPageGenerator func(ctx context.Context, id uint, baseURL string) (string, error)

// renderCardChatPage 调用平台 service 生成卡片聊天页并写入响应
func renderCardChatPage(ctx *gin.Context, gen cardChatPageGenerator, id uint, baseURL string) {
	html, err := gen(ctx.Request.Context(), id, baseURL)
	if err != nil {
		// 卡片不存在或渲染失败时降级为提示页
		ctx.Header("Content-Type", "text/html; charset=utf-8")
		ctx.String(http.StatusNotFound, fallbackCardHTML())
		return
	}
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.String(http.StatusOK, html)
}

// extractCardID 从 originalURL 中提取卡片 ID
// pathPrefix 例如 "/douyin/card/"
func extractCardID(originalURL, pathPrefix string) (uint, bool) {
	if !strings.Contains(originalURL, pathPrefix) {
		return 0, false
	}
	idStr := strings.Replace(originalURL, pathPrefix, "", 1)
	// 兼容尾部斜杠或查询串
	if idx := strings.IndexAny(idStr, "?#/"); idx >= 0 {
		idStr = idStr[:idx]
	}
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// buildBaseURL 从请求上下文构造站点根地址
// 优先使用 X-Forwarded-Proto，其次使用 r.URL.Scheme，最后默认 https
func buildBaseURL(ctx *gin.Context) string {
	scheme := ctx.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if ctx.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := ctx.GetHeader("Host")
	if host == "" {
		host = ctx.Request.Host
	}
	if host == "" {
		return "" // 兜底：使用相对路径
	}
	return scheme + "://" + host
}

// fallbackCardHTML 卡片不存在时的兜底页面
func fallbackCardHTML() string {
	return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>卡片不存在</title>
<style>body{font-family:-apple-system,sans-serif;background:#f5f5f7;color:#6e6e73;text-align:center;padding:60px 20px;}
h1{font-size:18px;margin-bottom:8px;color:#1d1d1f;}p{font-size:14px;}</style></head>
<body><h1>卡片不存在或已下线</h1><p>请联系客服获取最新链接</p></body></html>`
}
