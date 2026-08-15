package controller


import (
	"strconv"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service/translation"
)

// I18nStatsController 多语言监控看板控制器
type I18nStatsController struct {
	svc *translation.I18nStatsService
}

// NewI18nStatsController 构造看板控制器
func NewI18nStatsController(svc *translation.I18nStatsService) *I18nStatsController {
	return &I18nStatsController{svc: svc}
}

// RegisterRoutes 注册路由（与 GlossaryController 风格一致）
func (ctrl *I18nStatsController) RegisterRoutes(router *gin.RouterGroup) {
	g := router.Group("/i18n/stats")
	{
		g.GET("", ctrl.GetStats)
		g.GET("/lang-dist", ctrl.GetLangDistribution)
		g.GET("/cache", ctrl.GetCacheHitRate)
		g.GET("/glossary", ctrl.GetGlossaryCoverage)
		g.GET("/quality", ctrl.GetQualityTrend)
		g.GET("/latency", ctrl.GetLatencyStats)
		g.GET("/fallback", ctrl.GetFallbackRate)
	}
}

// parseDays 解析 days 查询参数，缺省/非法时返回 def。
func parseDays(c *gin.Context, def int) int {
	raw := c.Query("days")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > 365 {
		n = 365
	}
	return n
}

// GetStats 总览统计（看板首页）
// GET /api/i18n/stats?days=7
func (ctrl *I18nStatsController) GetStats(c *gin.Context) {
	days := parseDays(c, 7)
	overview, err := ctrl.svc.GetStatsWithDays(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.Success(c, overview, "获取成功")
}

// GetLangDistribution 语言分布
// GET /api/i18n/stats/lang-dist?days=7
func (ctrl *I18nStatsController) GetLangDistribution(c *gin.Context) {
	days := parseDays(c, 7)
	rows, err := ctrl.svc.GetLangDistribution(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, rows, int64(len(rows)))
}

// GetCacheHitRate 缓存命中率
// GET /api/i18n/stats/cache?days=7
func (ctrl *I18nStatsController) GetCacheHitRate(c *gin.Context) {
	days := parseDays(c, 7)
	hitRate, err := ctrl.svc.GetCacheHitRate(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.Success(c, gin.H{
		"days":           days,
		"cache_hit_rate": hitRate,
	}, "获取成功")
}

// GetGlossaryCoverage 术语覆盖率
// GET /api/i18n/stats/glossary
func (ctrl *I18nStatsController) GetGlossaryCoverage(c *gin.Context) {
	rows, err := ctrl.svc.GetGlossaryCoverage(c.Request.Context())
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, rows, int64(len(rows)))
}

// GetQualityTrend 质量趋势
// GET /api/i18n/stats/quality?days=30
func (ctrl *I18nStatsController) GetQualityTrend(c *gin.Context) {
	days := parseDays(c, 30)
	rows, err := ctrl.svc.GetQualityTrend(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, rows, int64(len(rows)))
}

// GetLatencyStats 延迟统计
// GET /api/i18n/stats/latency?days=7
func (ctrl *I18nStatsController) GetLatencyStats(c *gin.Context) {
	days := parseDays(c, 7)
	rows, err := ctrl.svc.GetLatencyStats(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.SuccessWithList(c, rows, int64(len(rows)))
}

// GetFallbackRate 降级率
// GET /api/i18n/stats/fallback?days=7
func (ctrl *I18nStatsController) GetFallbackRate(c *gin.Context) {
	days := parseDays(c, 7)
	rate, err := ctrl.svc.GetFallbackRate(c.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.Success(c, gin.H{
		"days":          days,
		"fallback_rate": rate,
	}, "获取成功")
}

