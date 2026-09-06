package controller

import (
	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// TechMetricsController GEO 技术配置生成 + 内容质量指标分析。
// （原 ResourceController 的 resourceSvc 硬编码静态数据已删除，
//
//	techconfig/metrics 两个真实子服务保留并独立成 controller）
type TechMetricsController struct {
	techconfigSvc *service.TechConfigService
	metricsSvc    *service.MetricsService
}

// NewTechMetricsController 构造技术配置+指标控制器
func NewTechMetricsController() *TechMetricsController {
	return &TechMetricsController{
		techconfigSvc: service.NewTechConfigService(),
		metricsSvc:    service.NewMetricsService(),
	}
}

// GenerateRobots 生成 robots.txt
// POST /geo/techconfig/robots
func (c *TechMetricsController) GenerateRobots(ctx *gin.Context) {
	var cfg service.RobotsConfig
	if !response.BindJSON(ctx, &cfg) {
		return
	}
	response.Success(ctx, gin.H{"content": c.techconfigSvc.GenerateRobots(&cfg)}, "生成成功")
}

// GenerateLLMsTxt 生成 llms.txt（AI 引擎知识索引，v3 竞品对齐 A4）
// POST /geo/techconfig/llms-txt
func (c *TechMetricsController) GenerateLLMsTxt(ctx *gin.Context) {
	var cfg service.LLMsTxtConfig
	if !response.BindJSON(ctx, &cfg) {
		return
	}
	response.Success(ctx, gin.H{"content": c.techconfigSvc.GenerateLLMsTxt(&cfg)}, "生成成功")
}

// GenerateSitemap 生成 sitemap.xml
// POST /geo/techconfig/sitemap
func (c *TechMetricsController) GenerateSitemap(ctx *gin.Context) {
	var cfg service.SitemapConfig
	if !response.BindJSON(ctx, &cfg) {
		return
	}
	response.Success(ctx, gin.H{"content": c.techconfigSvc.GenerateSitemap(&cfg)}, "生成成功")
}

// AnalyzeMetrics 分析内容质量指标
// POST /geo/metrics/analyze
func (c *TechMetricsController) AnalyzeMetrics(ctx *gin.Context) {
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
