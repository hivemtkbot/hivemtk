package controller

import (
	"github.com/gin-gonic/gin"
)

// RouteInfo 调试端点 /__debug__/routes 的路由条目
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

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
