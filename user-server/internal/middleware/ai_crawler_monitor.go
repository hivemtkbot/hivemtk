package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

var aiCrawlerBots = []struct {
	prefix string
	engine string
}{
	{"GPTBot", "OpenAI"},
	{"ChatGPT-User", "OpenAI"},
	{"OAI-SearchBot", "OpenAI"},
	{"PerplexityBot", "Perplexity"},
	{"Perplexity-User", "Perplexity"},
	{"ClaudeBot", "Anthropic"},
	{"Claude-Web", "Anthropic"},
	{"anthropic-ai", "Anthropic"},
	{"CCBot", "CommonCrawl"},
	{"Google-Extended", "Google"},
	{"GoogleOther", "Google"},
	{"Bytespider", "ByteDance"},
	{"Amazonbot", "Amazon"},
	{"Applebot-Extended", "Apple"},
	{"cohere-ai", "Cohere"},
	{"Diffbot", "Diffbot"},
}

// DetectAICrawler 检测 UA 是否为已知 AI 爬虫，返回归一化引擎名（非爬虫返回空）
func DetectAICrawler(ua string) string {
	if ua == "" {
		return ""
	}
	for _, b := range aiCrawlerBots {
		if strings.Contains(ua, b.prefix) {
			return b.engine
		}
	}
	return ""
}

// AICrawlerMonitor 中间件：自动记录 AI 爬虫访问到 geo_crawler_visits 表。
// 非阻塞：记录失败不影响请求处理。
// 注入 recorder 由装配层提供（避免 middleware→repository 直连违反五层架构）。
func AICrawlerMonitor(recorder func(engine, path, ua, ip string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		engine := DetectAICrawler(c.GetHeader("User-Agent"))
		if engine != "" && recorder != nil {
			go recorder(engine, c.Request.URL.Path, c.GetHeader("User-Agent"), c.ClientIP())
		}
		c.Next()
	}
}
