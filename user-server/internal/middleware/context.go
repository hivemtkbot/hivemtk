package middleware

import (
	"github.com/gin-gonic/gin"
)

// ContextMiddleware 设置上下文信息的中间件
func ContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		userAgent := c.GetHeader("User-Agent")

		c.Set("ip", clientIP)
		c.Set("user_agent", userAgent)

		c.Next()
	}
}

