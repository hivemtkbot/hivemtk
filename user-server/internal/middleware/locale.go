package middleware

import (
	"hivemtk-user/internal/pkg/i18n"

	"github.com/gin-gonic/gin"
)

// LocaleMiddleware 解析请求语言并注入 gin 上下文，供业务层返回本地化提示。
// 优先级：X-Locale 头 > Accept-Language 头 > 默认中文。
//
// 说明：仅作用于「Go 业务层返回给前端的文案」。LLM 调用（模型提示词/生成内容）
// 不在此处理 —— 大模型自带多语言能力，前端把用户语言透传给对话接口即可。
func LocaleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		loc := i18n.ZH
		if h := c.GetHeader("X-Locale"); h != "" {
			loc = i18n.Parse(h)
		} else if h := c.GetHeader("Accept-Language"); h != "" {
			loc = i18n.ParseAcceptLanguage(h)
		}
		c.Set("locale", loc)
		c.Next()
	}
}

