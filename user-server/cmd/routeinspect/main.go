package main

import (
	"encoding/json"
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

	// 打印全部 /api 路由到 stdout（供测试脚手架解析）
	routes := r.Routes()
	type rp struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	out := []rp{}
	for _, route := range routes {
		if len(route.Path) >= 4 && route.Path[:4] == "/api" {
			out = append(out, rp{Method: route.Method, Path: route.Path})
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	fmt.Fprintf(os.Stderr, "API 路由总数: %d\n", len(out))
}
