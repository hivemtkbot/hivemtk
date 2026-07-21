package middleware

import (
	"github.com/gin-gonic/gin"
)

// ContextMiddleware 设置上下文信息的中间件
func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP
		clientIP := c.ClientIP()

		// 获取User-Agent
		userAgent := c.GetHeader("User-Agent")

		// 将IP和User-Agent设置到context中
		c.Set("ip", clientIP)
		c.Set("user_agent", userAgent)

		c.Next()
	}
}
