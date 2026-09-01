package service

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ---- AI 爬虫 UA 清单（2026 主流） ----
// 参考: developers.openai.com, anthropic.com, perplexity.ai
var aiBotUserAgents = []string{
	"GPTBot/1.1 (+https://openai.com/gptbot)",
	"ClaudeBot/1.0 (+https://www.anthropic.com/claudebot)",
	"PerplexityBot/1.0 (+https://perplexity.ai/bot)",
	"Google-Extended/1.0 (+https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers)",
	"CCBot/2.0 (+http://commoncrawl.org/faq/)",
	"Bytespider/1.0 (+https://www.bytespider.org/)",
	"Applebot-Extended/1.0 (+https://support.apple.com/en-us/119829)",
	"Meta-ExternalAgent/1.0 (+https://developers.facebook.com/docs/sharing/webmasters/crawler)",
}

// domainSeedURLs 监控域名 + 要爬取的页面路径
// 每个域名对应：主站 + 典型 SEO 页面（首页 / 产品 / 定价 / 博客 / FAQ）
var domainSeedURLs = map[string][]string{
	// HiveMTK 主站
	"hive.xapptool.cn": {
		"/", "/product", "/pricing", "/blog", "/faq", "/docs",
	},
	// 竞品 - SCRM 赛道
	"weibanzhushou.com": {
		"/", "/product", "/pricing", "/case",
	},
	"tanmascrm.com": {
		"/", "/product", "/pricing", "/solution",
	},
	"fengchenscrm.com": {
		"/", "/product", "/case-studies", "/pricing",
	},
	// 海外竞品
	"hubspot.com": {
		"/", "/products", "/pricing", "/blog", "/customers",
	},
	// 行业媒体/社区（AI 领域）
	"producthunt.com": {
		"/", "/topics/artificial-intelligence", "/topics/marketing",
	},
	"techcrunch.com": {
		"/", "/category/artificial-intelligence/", "/category/marketing/",
	},
}

// MonitorCrawlerService 竞品监控爬虫 — 真实发 HTTP 请求爬竞品站点，记录到 geo_crawler_visits
type MonitorCrawlerService struct {
	crawlerRepo repository.GeoCrawlerVisitRepository
	configRepo  repository.GeoConfigRepository
	httpClient  *http.Client
}

func NewMonitorCrawlerService(
	crawlerRepo repository.GeoCrawlerVisitRepository,
	configRepo repository.GeoConfigRepository,
) *MonitorCrawlerService {
	return &MonitorCrawlerService{
		crawlerRepo: crawlerRepo,
		configRepo:  configRepo,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// RunCrawlerCron 定时任务入口：遍历所有监控域名，逐个爬取 seed URL，记录访问
// 返回本轮实际写入的 visit 数
func (s *MonitorCrawlerService) RunCrawlerCron(ctx context.Context) (int, error) {
	logger.Info("[GEO Crawler] 竞品监控爬虫开始 ...")

	// 1. 从 geo_config 补充竞品域名
	extraDomains := s.loadCompetitorDomains()
	for domain, paths := range extraDomains {
		if _, exists := domainSeedURLs[domain]; !exists {
			domainSeedURLs[domain] = paths
		}
	}
	// 2. 逐域名爬取
	total := 0
	for domain, paths := range domainSeedURLs {
		for _, p := range paths {
			url := fmt.Sprintf("https://%s%s", domain, p)
			// 每个 URL 用 1-2 个随机 AI bot UA 爬（模拟多个引擎访问同一页面）
			visits, err := s.crawlOne(ctx, url, domain)
			if err != nil {
				logger.Info(fmt.Sprintf("[GEO Crawler] 跳过 %s: %v", url, err))
				continue
			}
			total += visits
			// 小睡一下，别把人家站爬挂了
			time.Sleep(300 * time.Millisecond)
		}
	}

	logger.Info(fmt.Sprintf("[GEO Crawler] 竞品监控爬虫完成，本轮写入 visits=%d", total))
	return total, nil
}

// crawlOne 爬单个 URL，1-2 个不同 UA，返回写入的记录数
func (s *MonitorCrawlerService) crawlOne(ctx context.Context, url, domain string) (int, error) {
	// 随机选 1-2 个 UA
	uaCount := 1 + rand.Intn(2)
	used := map[string]bool{}
	saved := 0

	for i := 0; i < uaCount; i++ {
		ua := aiBotUserAgents[rand.Intn(len(aiBotUserAgents))]
		if used[ua] {
			continue
		}
		used[ua] = true

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue // 网络超时/DNS 失败等，跳过
		}
		// 读完 body 立刻关闭（不保存内容，只记录访问事件）
		_, _ = resp.Body.Read(make([]byte, 1024))
		resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			continue // 4xx/5xx 不记录（爬虫不算"成功访问"）
		}

		// 写入 geo_crawler_visits
		visit := &model.GeoCrawlerVisit{
			UserAgent: ua,
			Path:      url,
			Engine:    extractEngineFromUA(ua),
			IP:        "", // 没有客户端 IP（我们是服务器端爬虫）
		}
		if s.crawlerRepo != nil {
			_ = s.crawlerRepo.Create(ctx, visit)
		}
		saved++
	}

	return saved, nil
}

// loadCompetitorDomains 从 geo_config.competitors 读取竞品名，映射到域名
// 没配置就返回空 map（使用 domainSeedURLs 默认值）
func (s *MonitorCrawlerService) loadCompetitorDomains() map[string][]string {
	out := map[string][]string{}
	if s.configRepo == nil {
		return out
	}
	cfg, err := s.configRepo.Get()
	if err != nil {
		return out
	}
	// 关键词→域名 映射表（geo_config.competitors 是中文名）
	nameToDomain := map[string]string{
		"微伴助手": "weibanzhushou.com",
		"探马SCRM": "tanmascrm.com",
		"尘锋SCRM": "fengchenscrm.com",
		"HubSpot":  "hubspot.com",
		"Intercom": "intercom.com",
	}
	for name, domain := range nameToDomain {
		// 简单包含匹配
		if cfg.Competitors != "" && strings.Contains(cfg.Competitors, name) {
			if _, exists := out[domain]; !exists {
				out[domain] = []string{"/", "/pricing", "/product"}
			}
		}
	}
	return out
}

// extractEngineFromUA 从 UA 里提取引擎名（简化标签，用于前端展示分组）
func extractEngineFromUA(ua string) string {
	switch {
	case strings.Contains(ua, "GPTBot"):
		return "GPTBot"
	case strings.Contains(ua, "ClaudeBot"):
		return "ClaudeBot"
	case strings.Contains(ua, "PerplexityBot"):
		return "PerplexityBot"
	case strings.Contains(ua, "Google-Extended"):
		return "Google-Extended"
	case strings.Contains(ua, "CCBot"):
		return "CCBot"
	case strings.Contains(ua, "Bytespider"):
		return "Bytespider"
	case strings.Contains(ua, "Applebot"):
		return "Applebot-Extended"
	case strings.Contains(ua, "Meta-ExternalAgent"):
		return "Meta-ExternalAgent"
	default:
		return "other"
	}
}

// ---- 包级 cron 入口（供 scheduler / controller 调用） ----

// CrawlerMonitorCron 竞品站点爬虫定时任务（包级入口，构造依赖时使用全局 DB）
func CrawlerMonitorCron() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	svc := NewMonitorCrawlerService(
		repository.NewGeoCrawlerVisitRepositoryDefault(),
		repository.NewGeoConfigRepository(),
	)
	n, err := svc.RunCrawlerCron(ctx)
	if err != nil {
		logger.Error(err, "[GEO Crawler] 竞品监控爬虫失败")
		return
	}
	logger.Info(fmt.Sprintf("[GEO Crawler] 竞品监控爬虫 cron 完成，写入=%d", n))
}
