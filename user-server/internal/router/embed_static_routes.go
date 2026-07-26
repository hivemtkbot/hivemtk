package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// setupEmbedStaticRoutes 静态文件路由（chat embed 页面 + embed SDK）
//
// P0-10 ADR-010 私域部署优化（2026-07-17）：
//   - /chat/embed/*  → user-web dist（Vue 聊天窗 SPA）
//   - /embed/*       → embed SDK 静态文件（marketing-chat-widget.iife.js）
//
// 部署方式：
//  1. 生产环境：把 user-web/dist 和 embed-sdk/dist 部署到同源
//  2. 开发环境：通过环境变量 USER_WEB_DIST / EMBED_SDK_DIST 指定
//
// 默认查找路径（相对项目根）：
//   - ../user-web/dist
//   - ../embed-sdk/dist
func setupEmbedStaticRoutes(r *gin.Engine) {
	// 1. 解析 user-web dist 路径
	userWebDist := os.Getenv("USER_WEB_DIST")
	if userWebDist == "" {
		// 默认相对路径（go run 时 cwd=user-server）
		candidates := []string{
			"../user-web/dist",
			"./user-web-dist",
			"./dist",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				userWebDist, _ = filepath.Abs(c)
				break
			}
		}
	}

	// 2. 解析 embed-sdk dist 路径
	embedDist := os.Getenv("EMBED_SDK_DIST")
	if embedDist == "" {
		candidates := []string{
			"../embed-sdk/dist",
			"./embed-sdk-dist",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				embedDist, _ = filepath.Abs(c)
				break
			}
		}
	}

	// 3. 注册 user-web dist 静态服务（聊天窗 SPA）
	if userWebDist != "" {
		if _, err := os.Stat(userWebDist); err == nil {
			// 静态资源（CSS / JS / 图片 / 字体）
			r.Static("/assets", filepath.Join(userWebDist, "assets"))
			r.StaticFile("/favicon.ico", filepath.Join(userWebDist, "favicon.ico"))
			r.StaticFile("/logo.png", filepath.Join(userWebDist, "logo.png"))

			// SPA 路由：所有 /chat/embed/* 都返回 index.html（Vue Router hash 模式）
			// user-web 路由使用 createWebHashHistory()，
			//   浏览器访问 /chat/embed/default 不会触发 hash 路由，
			//   必须用 /#/chat/embed/default 才能命中目标路由。
			//   自动加上 #/ 前缀让 URL 更友好。
			r.GET("/chat/embed/*path", func(c *gin.Context) {
				path := c.Param("path")
				if path != "" && !strings.HasPrefix(path, "#") {
					// 浏览器访问 /chat/embed/default → 重定向到 /#/chat/embed/default
					c.Redirect(http.StatusFound, "/#/chat/embed/"+strings.TrimPrefix(path, "/"))
					return
				}
				serveSPA(c, userWebDist)
			})
			r.GET("/chat/embed", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/#/chat/embed/default")
			})

			// 根路径：直接返回 SPA（让 Vue Router 根据 hash 路由）
			// 统一返回 index.html，由 Vue Router 处理路由：
			//   - /#                  → Layout → 跳 /unifiedInbox/list
			//   - /#/chat/embed/xxx   → ChatEmbed（公开）
			r.GET("/", func(c *gin.Context) {
				serveSPA(c, userWebDist)
			})

			// SPA 路由：所有非 API 路径都返回 index.html
			// 兜底 /unifiedInbox/list 等主应用路由，避免 404
			r.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				// API 路径不处理（返回 404）
				if strings.HasPrefix(path, "/api/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				// 静态资源路径不处理
				if strings.HasPrefix(path, "/assets/") ||
					strings.HasPrefix(path, "/embed/") ||
					strings.HasPrefix(path, "/static/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				serveSPA(c, userWebDist)
			})

			// hash 模式 SPA：根路径 /
			r.GET("/index.html", func(c *gin.Context) {
				serveSPA(c, userWebDist)
			})

			_ = userWebDist
		} else {
			// 没找到 dist，给一个清晰的提示
			r.GET("/chat/embed/*path", func(c *gin.Context) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "user-web dist not found",
					"message": "请先构建 user-web: cd ../user-web && npm run build，并把 dist 放到能被 user-server 找到的位置。可通过环境变量 USER_WEB_DIST 指定。",
					"path":    c.Param("path"),
				})
			})
		}
	} else {
		r.GET("/chat/embed/*path", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "user-web dist not configured",
			})
		})
	}

	// 4. 注册 embed-sdk 静态服务
	if embedDist != "" {
		if _, err := os.Stat(embedDist); err == nil {
			r.Static("/embed", embedDist)
		}
	}
}

// serveSPA 提供 SPA 入口：返回 index.html
func serveSPA(c *gin.Context, distDir string) {
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "index.html not found", "path": indexPath})
		return
	}
	c.File(indexPath)
}
