package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"

	"marketing/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)

// MetricsAuthMiddleware /metrics 端点鉴权中间件
//
// 安全策略（P1-1 修复）：
//  1. 默认仅允许 loopback（127.0.0.1 / ::1）与私有网段（10/8、172.16/12、192.168/16）访问
//  2. 若设置环境变量 METRICS_TOKEN，则允许任何 IP 携带 `Authorization: Bearer <token>` 访问
//  3. 其他情况返回 404（不暴露端点存在性）
//
// 设计原因：Prometheus 指标含 QPS、延迟、handler 标签，可被外部攻击者用于业务流量侦察。
// 私域部署下 Prometheus 通常与业务同网络，loopback + 私有网段白名单足够。
func MetricsAuthMiddleware() gin.HandlerFunc {
	token := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	return func(c *gin.Context) {
		ipStr := c.ClientIP()
		ip := net.ParseIP(ipStr)
		if ip == nil {
			logger.Warn("MetricsAuth: 无法解析客户端 IP=" + ipStr)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 1. loopback / 私有网段直接放行
		if ip.IsLoopback() || isPrivateIP(ip) {
			c.Next()
			return
		}

		// 2. 配置了 token 则校验 Bearer
		if token != "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") && strings.TrimSpace(auth[7:]) == token {
				c.Next()
				return
			}
		}

		// 3. 拒绝（返回 404 避免暴露端点存在性）
		c.AbortWithStatus(http.StatusNotFound)
	}
}

// isPrivateIP 判断是否私有网段（RFC 1918）或 link-local
func isPrivateIP(ip net.IP) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		}
	}
	// 非私有网段、非 link-local：拒绝
	return false
}
