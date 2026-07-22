package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件。
// 私有部署默认全放开（反射 Origin 并允许凭证），与平台端保持一致；
// 若需收敛来源，可在此读取环境变量白名单进行校验。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Vary", "Origin")
		if hdr := c.Request.Header.Get("Access-Control-Request-Headers"); hdr != "" {
			c.Writer.Header().Set("Access-Control-Allow-Headers", hdr)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Timestamp, X-Signature, X-Merchant-Key, X-Trace-ID, X-Requested-With, X-Knowledge-Token")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
