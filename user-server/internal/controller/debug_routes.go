package controller

import (
	"github.com/gin-gonic/gin"
)

// RouteInfo 调试端点 /__debug__/routes 的路由条目
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// DebugRoutesHandler 返回 gin.Engine 注册的全部路由（去重、排除 OPTIONS）。
// 原 router.go 内联遍历逻辑已于 2026-09-02 抽到此处。
// 返回格式保持裸 JSON（不走 response.Success 包装），与原端点契约一致，
// 避免破坏可能消费此调试端点的外部工具。
//
// Router 层只需：
//
//	r.GET("/__debug__/routes", controller.DebugRoutesHandler(r))
func DebugRoutesHandler(engine *gin.Engine) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := engine.Routes()
		out := make([]RouteInfo, 0, len(raw))
		seen := make(map[string]bool, len(raw))
		for _, rt := range raw {
			if rt.Method == "OPTIONS" {
				continue
			}
			key := rt.Method + " " + rt.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, RouteInfo{Method: rt.Method, Path: rt.Path})
		}
		c.JSON(200, gin.H{"code": 0, "total": len(out), "routes": out})
	}
}
