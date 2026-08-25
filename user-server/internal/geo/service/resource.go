package service

import "strings"

// ResourceAgent GEO 相关 AI Agent
type ResourceAgent struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Rating      string   `json:"rating"`
	Features    []string `json:"features"`
}

// ResourceTool GEO 相关工具
type ResourceTool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Rating      string   `json:"rating"`
	Features    []string `json:"features"`
}

// ResourcePaper GEO 相关论文/指南
type ResourcePaper struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Date        string `json:"date"`
	Importance  string `json:"importance"`
}

// ResourceCommunity GEO 社区
type ResourceCommunity struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Rating      string `json:"rating"`
}

// ResourceSearchResult 搜索结果
type ResourceSearchResult struct {
	Type        string   `json:"type"`
	Name        string   `json:"name,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Rating      string   `json:"rating,omitempty"`
	Features    []string `json:"features,omitempty"`
	Date        string   `json:"date,omitempty"`
	Importance  string   `json:"importance,omitempty"`
}

// ResourceSummary 资源汇总
type ResourceSummary struct {
	Total       int `json:"total"`
	Agents      int `json:"agents"`
	Tools       int `json:"tools"`
	Papers      int `json:"papers"`
	Communities int `json:"communities"`
}

// ResourceService GEO 资源推荐服务（迁移自 AIGEOTOOLS resources/service.go）
type ResourceService struct{}

// NewResourceService 创建资源推荐服务
func NewResourceService() *ResourceService {
	return &ResourceService{}
}

// GetAgents 获取 AI Agent 列表
func (s *ResourceService) GetAgents(category string) []ResourceAgent {
	if category == "" {
		return defaultAgents()
	}
	var result []ResourceAgent
	for _, a := range defaultAgents() {
		if a.Category == category {
			result = append(result, a)
		}
	}
	return result
}

// GetTools 获取工具列表
func (s *ResourceService) GetTools(category string) []ResourceTool {
	if category == "" {
		return defaultTools()
	}
	var result []ResourceTool
	for _, t := range defaultTools() {
		if t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// GetPapers 获取论文/指南列表
func (s *ResourceService) GetPapers(category, importance string) []ResourcePaper {
	result := defaultPapers()
	if category != "" {
		filtered := make([]ResourcePaper, 0, len(result))
		for _, p := range result {
			if p.Category == category {
				filtered = append(filtered, p)
			}
		}
		result = filtered
	}
	if importance != "" {
		filtered := make([]ResourcePaper, 0, len(result))
		for _, p := range result {
			if p.Importance == importance {
				filtered = append(filtered, p)
			}
		}
		result = filtered
	}
	return result
}

// GetCommunities 获取社区列表
func (s *ResourceService) GetCommunities() []ResourceCommunity {
	return defaultCommunities()
}

// GetSummary 获取资源汇总
func (s *ResourceService) GetSummary() ResourceSummary {
	return ResourceSummary{
		Total:       len(defaultAgents()) + len(defaultTools()) + len(defaultPapers()) + len(defaultCommunities()),
		Agents:      len(defaultAgents()),
		Tools:       len(defaultTools()),
		Papers:      len(defaultPapers()),
		Communities: len(defaultCommunities()),
	}
}

// SearchResources 搜索资源
func (s *ResourceService) SearchResources(query, resourceType string) []ResourceSearchResult {
	q := strings.ToLower(query)
	var all []ResourceSearchResult

	if resourceType == "" || resourceType == "agents" {
		for _, a := range defaultAgents() {
			all = append(all, ResourceSearchResult{
				Type: "agent", Name: a.Name, Description: a.Description,
				URL: a.URL, Category: a.Category, Rating: a.Rating, Features: a.Features,
			})
		}
	}
	if resourceType == "" || resourceType == "tools" {
		for _, t := range defaultTools() {
			all = append(all, ResourceSearchResult{
				Type: "tool", Name: t.Name, Description: t.Description,
				URL: t.URL, Category: t.Category, Rating: t.Rating, Features: t.Features,
			})
		}
	}
	if resourceType == "" || resourceType == "papers" {
		for _, p := range defaultPapers() {
			all = append(all, ResourceSearchResult{
				Type: "paper", Title: p.Title, Description: p.Description,
				URL: p.URL, Category: p.Category, Date: p.Date, Importance: p.Importance,
			})
		}
	}
	if resourceType == "" || resourceType == "communities" {
		for _, c := range defaultCommunities() {
			all = append(all, ResourceSearchResult{
				Type: "community", Name: c.Name, Description: c.Description,
				URL: c.URL, Category: c.Category, Rating: c.Rating,
			})
		}
	}

	var results []ResourceSearchResult
	for _, r := range all {
		name := strings.ToLower(r.Name)
		if name == "" {
			name = strings.ToLower(r.Title)
		}
		desc := strings.ToLower(r.Description)
		cat := strings.ToLower(r.Category)
		feat := strings.ToLower(strings.Join(r.Features, " "))

		if strings.Contains(name, q) || strings.Contains(desc, q) ||
			strings.Contains(cat, q) || strings.Contains(feat, q) {
			results = append(results, r)
		}
	}
	return results
}

func defaultAgents() []ResourceAgent {
	return []ResourceAgent{
		{Name: "Perplexity AI", Description: "AI 搜索引擎，可用于验证 GEO 效果", URL: "https://www.perplexity.ai", Category: "AI 搜索", Rating: "⭐⭐⭐⭐⭐", Features: []string{"实时搜索", "引用来源", "多模型支持"}},
		{Name: "ChatGPT Search", Description: "OpenAI 的搜索功能，验证品牌在 AI 搜索中的表现", URL: "https://chat.openai.com", Category: "AI 搜索", Rating: "⭐⭐⭐⭐⭐", Features: []string{"GPT-4", "实时联网", "引用分析"}},
		{Name: "Google SGE", Description: "Google 搜索生成体验，了解 AI 搜索趋势", URL: "https://search.google", Category: "AI 搜索", Rating: "⭐⭐⭐⭐", Features: []string{"AI 摘要", "来源引用", "搜索结果"}},
		{Name: "Jasper AI", Description: "AI 内容创作平台，支持 SEO 优化内容生成", URL: "https://www.jasper.ai", Category: "内容生成", Rating: "⭐⭐⭐⭐", Features: []string{"模板丰富", "品牌声音", "SEO 优化"}},
		{Name: "Surfer SEO", Description: "SEO 内容优化工具，支持 SERP 分析", URL: "https://surferseo.com", Category: "SEO 工具", Rating: "⭐⭐⭐⭐", Features: []string{"内容评分", "关键词分析", "SERP 分析"}},
	}
}

func defaultTools() []ResourceTool {
	return []ResourceTool{
		{Name: "Google Search Console", Description: "监控网站在 Google 搜索中的表现", URL: "https://search.google.com/search-console", Category: "搜索引擎工具", Rating: "⭐⭐⭐⭐⭐", Features: []string{"搜索分析", "索引监控", "性能报告"}},
		{Name: "Bing Webmaster Tools", Description: "Bing 搜索引擎的网站管理工具", URL: "https://www.bing.com/webmasters", Category: "搜索引擎工具", Rating: "⭐⭐⭐⭐", Features: []string{"索引提交", "搜索分析", "URL 检查"}},
		{Name: "Schema.org Validator", Description: "验证 JSON-LD Schema 标记是否正确", URL: "https://validator.schema.org", Category: "结构化数据", Rating: "⭐⭐⭐⭐⭐", Features: []string{"Schema 验证", "结构化数据测试", "错误检测"}},
		{Name: "Google Rich Results Test", Description: "测试网页是否支持 Google 富媒体搜索结果", URL: "https://search.google.com/test/rich-results", Category: "结构化数据", Rating: "⭐⭐⭐⭐⭐", Features: []string{"富媒体测试", "预览效果", "错误诊断"}},
		{Name: "PageSpeed Insights", Description: "分析网页性能，Core Web Vitals 指标", URL: "https://pagespeed.web.dev", Category: "性能工具", Rating: "⭐⭐⭐⭐⭐", Features: []string{"性能分析", "优化建议", "移动端测试"}},
		{Name: "Ahrefs", Description: "SEO 工具套件，关键词研究和竞品分析", URL: "https://ahrefs.com", Category: "SEO 工具", Rating: "⭐⭐⭐⭐⭐", Features: []string{"关键词研究", "反向链接分析", "竞品分析"}},
		{Name: "SEMrush", Description: "数字营销工具，SEO 和内容营销分析", URL: "https://www.semrush.com", Category: "SEO 工具", Rating: "⭐⭐⭐⭐⭐", Features: []string{"关键词研究", "站点审计", "内容优化"}},
		{Name: "Clearscope", Description: "AI 内容优化工具，提升内容相关性", URL: "https://www.clearscope.io", Category: "内容优化", Rating: "⭐⭐⭐⭐", Features: []string{"内容评分", "关键词建议", "竞品分析"}},
	}
}

func defaultPapers() []ResourcePaper {
	return []ResourcePaper{
		{Title: "GEO: Generative Engine Optimization (arXiv)", Description: "GEO 原始研究论文，定义了生成式引擎优化的概念和方法", URL: "https://arxiv.org/abs/2311.09735", Category: "学术论文", Date: "2023", Importance: "高"},
		{Title: "Google E-E-A-T Guidelines", Description: "Google 官方 E-E-A-T 指南，GEO 核心原则", URL: "https://developers.google.com/search/docs/fundamentals/creating-helpful-content", Category: "官方指南", Date: "2024", Importance: "高"},
		{Title: "Google Search Quality Rater Guidelines", Description: "Google 搜索质量评估指南，详细的 E-E-A-T 标准", URL: "https://guidelines.raterhub.com", Category: "官方指南", Date: "2024", Importance: "高"},
		{Title: "Schema.org Documentation", Description: "Schema.org 结构化数据完整文档", URL: "https://schema.org", Category: "技术文档", Date: "持续更新", Importance: "高"},
		{Title: "Google Structured Data Guidelines", Description: "Google 结构化数据指南和最佳实践", URL: "https://developers.google.com/search/docs/appearance/structured-data", Category: "技术文档", Date: "2024", Importance: "高"},
		{Title: "AI Search Optimization Guide", Description: "AI 搜索引擎优化最佳实践指南", URL: "https://www.searchenginejournal.com/ai-search-optimization", Category: "最佳实践", Date: "2024", Importance: "中"},
		{Title: "LLM Prompt Engineering Guide", Description: "大语言模型提示工程完整指南", URL: "https://www.promptingguide.ai", Category: "技术指南", Date: "持续更新", Importance: "中"},
		{Title: "Content Quality Guidelines", Description: "高质量内容创作指南", URL: "https://developers.google.com/search/docs/fundamentals/creating-helpful-content", Category: "内容指南", Date: "2024", Importance: "中"},
	}
}

func defaultCommunities() []ResourceCommunity {
	return []ResourceCommunity{
		{Name: "r/SEO (Reddit)", Description: "Reddit SEO 社区，讨论 SEO 和 GEO 策略", URL: "https://www.reddit.com/r/SEO", Category: "论坛社区", Rating: "⭐⭐⭐⭐⭐"},
		{Name: "r/ChatGPT (Reddit)", Description: "ChatGPT 社区，讨论 AI 搜索和 GEO 应用", URL: "https://www.reddit.com/r/ChatGPT", Category: "论坛社区", Rating: "⭐⭐⭐⭐"},
		{Name: "SEO Twitter/X Community", Description: "SEO 和 GEO 从业者 Twitter 社区", URL: "https://twitter.com/search?q=SEO%20GEO", Category: "社交媒体", Rating: "⭐⭐⭐⭐"},
		{Name: "Google Search Central Community", Description: "Google 官方搜索社区", URL: "https://support.google.com/webmasters/community", Category: "官方社区", Rating: "⭐⭐⭐⭐⭐"},
		{Name: "Moz Community", Description: "Moz SEO 社区，丰富的 SEO 资源", URL: "https://moz.com/community", Category: "论坛社区", Rating: "⭐⭐⭐⭐"},
	}
}
