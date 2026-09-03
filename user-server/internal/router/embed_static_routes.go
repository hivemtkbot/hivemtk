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
// 私域部署优化：
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
	userWebDist := os.Getenv("USER_WEB_DIST")
	if userWebDist == "" {
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

	if userWebDist != "" {
		if _, err := os.Stat(userWebDist); err == nil {
			r.Static("/assets", filepath.Join(userWebDist, "assets"))
			r.StaticFile("/favicon.ico", filepath.Join(userWebDist, "favicon.ico"))
			r.StaticFile("/logo.png", filepath.Join(userWebDist, "logo.png"))

			r.GET("/chat/embed/*path", func(c *gin.Context) {
				path := c.Param("path")
				if path != "" && !strings.HasPrefix(path, "#") {
					c.Redirect(http.StatusFound, "/#/chat/embed/"+strings.TrimPrefix(path, "/"))
					return
				}
				serveSPA(c, userWebDist)
			})
			r.GET("/chat/embed", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/#/chat/embed/default")
			})

			r.GET("/", func(c *gin.Context) {
				serveSPA(c, userWebDist)
			})

			r.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/api/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				// 存储文件访问路由（由 r.Static("/files", ...) 注册，此处只负责文件不存在时的正确 404）
				if strings.HasPrefix(path, "/files/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "file not found", "path": path})
					return
				}
				if strings.HasPrefix(path, "/assets/") ||
					strings.HasPrefix(path, "/embed/") ||
					strings.HasPrefix(path, "/static/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				serveSPA(c, userWebDist)
			})

			r.GET("/index.html", func(c *gin.Context) {
				serveSPA(c, userWebDist)
			})

			_ = userWebDist
		} else {
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

