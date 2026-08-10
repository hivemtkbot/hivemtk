package main

import (
	"fmt"
	"os"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/router"

	"github.com/gin-gonic/gin"
)

var _ = config.GetAppConfig
var _ = platform.InitSync

func main() {
	// 加载配置（Viper 自动从 config.yaml 读取）
	_ = config.GetAppConfig

	// 初始化 DB
	db.InitDB()

	// 初始化授权检查器：兜底 base URL 派生自 config.DefaultPlatformBaseURL
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8205 | platform-server
	// 注意：routeinspect 仅打印路由，不需要真实连接，URL 字段可填任意可达占位
	middleware.InitLicenseChecker(config.DefaultPlatformBaseURL, "")

	gin.SetMode(gin.DebugMode)
	r := gin.New()
	router.Setup(r, db.GetDB())

	// 打印所有已注册的 /api/chat-channels 路由
	fmt.Fprintln(os.Stderr, "=== 已注册的 /api/chat-channels 路由 ===")
	chatCount := 0
	for _, route := range r.Routes() {
		if len(route.Path) >= 14 && route.Path[:14] == "/api/chat-chan" {
			fmt.Fprintf(os.Stderr, "%-7s %s\n", route.Method, route.Path)
			chatCount++
		}
	}
	fmt.Fprintf(os.Stderr, "共 %d 条 chat-channels 路由\n", chatCount)

	// 测试访问
	fmt.Fprintln(os.Stderr, "\n=== 测试 /api/chat-channels 路由 ===")
	for _, route := range r.Routes() {
		if route.Path == "/api/chat-channels" {
			fmt.Fprintf(os.Stderr, "FOUND: %s %s\n", route.Method, route.Path)
		}
	}

	// 打印所有 /api 路由数量
	total := 0
	for _, route := range r.Routes() {
		if len(route.Path) >= 4 && route.Path[:4] == "/api" {
			total++
		}
	}
	fmt.Fprintf(os.Stderr, "API 路由总数: %d\n", total)
}
