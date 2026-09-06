// Package main 提供 syncscripts 命令行工具（T-5 双体系桥接手动触发入口）
//
// 职责:
//   - 读取管理端话术模板库（script_templates）
//   - 幂等 upsert 到运行时引擎话术库（script_library），按 (category, title) 去重
//
// 使用:
//
//	cd hivemtk/user-server
//	go run ./cmd/syncscripts
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/db"
)

func main() {
	flag.Parse()

	db.InitDB()

	svc := service.NewScriptTemplateSyncService()
	stats, err := svc.SyncToLibrary(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 同步失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ template → library 同步完成: scanned=%d created=%d updated=%d skipped=%d\n",
		stats.Scanned, stats.Created, stats.Updated, stats.Skipped)
}
