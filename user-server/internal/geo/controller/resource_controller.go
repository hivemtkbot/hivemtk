package controller

import (
	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// ResourceController GEO 资源推荐 / 技术配置 / 质量指标控制器。
type ResourceController struct {
	resourceSvc   *service.ResourceService
	techconfigSvc *service.TechConfigService
	metricsSvc    *service.MetricsService
}

// NewResourceController 构造资源控制器
func NewResourceController() *ResourceController {
	return &ResourceController{
		resourceSvc:   service.NewResourceService(),
		techconfigSvc: service.NewTechConfigService(),
		metricsSvc:    service.NewMetricsService(),
	}
}

// === 资源推荐 ===

// GetAgents 获取 AI Agent 列表
// GET /geo/resources/agents?category=
func (c *ResourceController) GetAgents(ctx *gin.Context) {
	response.Success(ctx, c.resourceSvc.GetAgents(ctx.Query("category")), "获取成功")
}

// GetTools 获取工具列表
// GET /geo/resources/tools?category=
func (c *ResourceController) GetTools(ctx *gin.Context) {
	response.Success(ctx, c.resourceSvc.GetTools(ctx.Query("category")), "获取成功")
}

// GetPapers 获取论文/指南列表
// GET /geo/resources/papers?category=&importance=
func (c *ResourceController) GetPapers(ctx *gin.Context) {
	response.Success(ctx, c.resourceSvc.GetPapers(ctx.Query("category"), ctx.Query("importance")), "获取成功")
}

// GetCommunities 获取社区列表
// GET /geo/resources/communities
func (c *ResourceController) GetCommunities(ctx *gin.Context) {
	response.Success(ctx, c.resourceSvc.GetCommunities(), "获取成功")
}

// GetResourceSummary 获取资源汇总
// GET /geo/resources/summary
func (c *ResourceController) GetResourceSummary(ctx *gin.Context) {
	response.Success(ctx, c.resourceSvc.GetSummary(), "获取成功")
}

// SearchResources 搜索资源
// GET /geo/resources/search?q=&type=
func (c *ResourceController) SearchResources(ctx *gin.Context) {
	q := ctx.Query("q")
	if q == "" {
		response.Error(ctx, 400, "q 参数必填")
		return
	}
	response.Success(ctx, c.resourceSvc.SearchResources(q, ctx.Query("type")), "搜索完成")
}

// === 技术配置 ===

// GenerateRobots 生成 robots.txt
// POST /geo/techconfig/robots
func (c *ResourceController) GenerateRobots(ctx *gin.Context) {
	var cfg service.RobotsConfig
	if !response.BindJSON(ctx, &cfg) {
		return
	}
	response.Success(ctx, gin.H{"content": c.techconfigSvc.GenerateRobots(&cfg)}, "生成成功")
}

// GenerateSitemap 生成 sitemap.xml
// POST /geo/techconfig/sitemap
func (c *ResourceController) GenerateSitemap(ctx *gin.Context) {
	var cfg service.SitemapConfig
	if !response.BindJSON(ctx, &cfg) {
		return
	}
	response.Success(ctx, gin.H{"content": c.techconfigSvc.GenerateSitemap(&cfg)}, "生成成功")
}

// === 质量指标 ===

// AnalyzeMetrics 分析内容质量指标
// POST /geo/metrics/analyze
func (c *ResourceController) AnalyzeMetrics(ctx *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
		Keyword string `json:"keyword"`
		Brand   string `json:"brand"`
	}
	if !response.BindJSON(ctx, &req) {
		return
	}
	response.Success(ctx, c.metricsSvc.Analyze(req.Content, req.Keyword, req.Brand), "分析完成")
}
