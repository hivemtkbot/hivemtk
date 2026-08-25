package service

import (
	"fmt"
	"strings"
	"time"
)

// TechConfigService GEO 技术配置生成服务（迁移自 AIGEOTOOLS techconfig/techconfig.go）
type TechConfigService struct{}

// NewTechConfigService 创建技术配置生成服务
func NewTechConfigService() *TechConfigService {
	return &TechConfigService{}
}

// RobotsConfig robots.txt 生成配置
type RobotsConfig struct {
	SiteURL      string   `json:"site_url" binding:"required"`
	Disallow     []string `json:"disallow"`
	Allow        []string `json:"allow"`
	CrawlDelay   int      `json:"crawl_delay"`
	SitemapPaths []string `json:"sitemaps"`
}

// SitemapConfig sitemap.xml 生成配置
type SitemapConfig struct {
	SiteURL string   `json:"site_url" binding:"required"`
	URLs    []string `json:"urls"`
}

// GenerateRobots 生成 robots.txt
func (s *TechConfigService) GenerateRobots(cfg *RobotsConfig) string {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for _, p := range cfg.Allow {
		b.WriteString("Allow: " + p + "\n")
	}
	for _, p := range cfg.Disallow {
		b.WriteString("Disallow: " + p + "\n")
	}
	if cfg.CrawlDelay > 0 {
		b.WriteString(fmt.Sprintf("Crawl-delay: %d\n", cfg.CrawlDelay))
	}
	sm := cfg.SitemapPaths
	if len(sm) == 0 && cfg.SiteURL != "" {
		sm = []string{cfg.SiteURL + "/sitemap.xml"}
	}
	for _, sPath := range sm {
		if !strings.HasPrefix(sPath, "http") && cfg.SiteURL != "" {
			sPath = cfg.SiteURL + sPath
		}
		b.WriteString("Sitemap: " + sPath + "\n")
	}
	return b.String()
}

// GenerateSitemap 生成 sitemap.xml
func (s *TechConfigService) GenerateSitemap(cfg *SitemapConfig) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	now := time.Now().UTC().Format(time.DateOnly)
	for _, u := range cfg.URLs {
		full := u
		if !strings.HasPrefix(full, "http") && cfg.SiteURL != "" {
			full = cfg.SiteURL + u
		}
		b.WriteString("  <url>\n")
		b.WriteString("    <loc>" + full + "</loc>\n")
		b.WriteString("    <lastmod>" + now + "</lastmod>\n")
		b.WriteString("    <changefreq>weekly</changefreq>\n")
		b.WriteString("    <priority>0.8</priority>\n")
		b.WriteString("  </url>\n")
	}
	b.WriteString("</urlset>\n")
	return b.String()
}
