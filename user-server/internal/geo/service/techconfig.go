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

// LLMsTxtConfig llms.txt 生成配置（v3 竞品对齐 A4）
// llms.txt 是面向 AI 引擎的站点知识索引约定（类比 robots.txt 之于搜索引擎），
// 帮助 LLM 快速定位品牌权威文档，提升被引用概率。
type LLMsTxtConfig struct {
	SiteURL   string            `json:"site_url" binding:"required"`
	Brand     string            `json:"brand" binding:"required"`
	Overview  string            `json:"overview"`
	Documents []LLMsDocEntry    `json:"documents"`
	Policies  map[string]string `json:"policies,omitempty"`
}

type LLMsDocEntry struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// GenerateLLMsTxt 输出 Markdown 格式的 llms.txt 内容
func (s *TechConfigService) GenerateLLMsTxt(cfg *LLMsTxtConfig) string {
	var b strings.Builder
	b.WriteString("# " + cfg.Brand + "\n\n")
	if cfg.Overview != "" {
		b.WriteString("> " + cfg.Overview + "\n\n")
	}
	if len(cfg.Documents) > 0 {
		b.WriteString("## Docs\n\n")
		for _, d := range cfg.Documents {
			line := "- [" + d.Title + "](" + d.URL + ")"
			if d.Description != "" {
				line += ": " + d.Description
			}
			b.WriteString(line + "\n")
		}
	}
	if len(cfg.Policies) > 0 {
		b.WriteString("\n## Policies\n\n")
		for k, v := range cfg.Policies {
			b.WriteString("- [" + k + "](" + v + ")\n")
		}
	}
	return b.String()
}
