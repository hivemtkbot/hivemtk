// Package main 性能压测入口
// 独立部署版本：user-server 兜底监听 :8204（单一源：config.DefaultUserServerBaseURL）
//
// 使用方法：
//
//	go run ./cmd/perf                          # 跑全部场景（默认 base 派生自 config.DefaultUserServerBaseURL）
//	go run ./cmd/perf -base=http://example.com # 覆盖 base URL
//	go run ./cmd/perf -scene=login             # 跑指定场景
//
// 安全护栏：
//   - 登录账号/密码必须通过命令行显式传入（PERF_USERNAME / PERF_PASSWORD）
//     禁止硬编码默认值（防 admin123 等弱口令风险）
//   - 未提供时要求显式传参并明确报错，不静默兜底
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/tests/perf/perflib"
)

var (
	baseURL = flag.String("base", config.DefaultUserServerBaseURL, "user-server base URL")
	scene   = flag.String("scene", "all", "测试场景: all|login|customer|message|knowledge|cdp")

	perfUsername = flag.String("username", os.Getenv("PERF_USERNAME"), "登录用户名（必须显式传入）")
	perfPassword = flag.String("password", os.Getenv("PERF_PASSWORD"), "登录密码（必须显式传入，禁止默认值）")
)

func main() {
	flag.Parse()

	runner := perflib.NewLoadRunner()
	ctx := context.Background()

	scenes := []perflib.Config{
		loginScene(),
		customerListScene(),
		messageListScene(),
		knowledgeQueryScene(),
		cdpEventScene(),
	}

	for _, s := range scenes {
		if *scene != "all" && s.Name != *scene {
			continue
		}
		fmt.Printf("\n🚀 正在执行场景: %s ...\n", s.Name)
		result, err := runner.Run(ctx, s)
		if err != nil {
			fmt.Printf("❌ 场景 %s 执行失败: %v\n", s.Name, err)
			continue
		}
		perflib.PrintResult(result)
		time.Sleep(500 * time.Millisecond)
	}
}

// loginScene 登录接口压测
// 强约束：账号/密码必须显式传入；未提供时直接 fatal，不静默兜底到 admin123
func loginScene() perflib.Config {
	if *perfUsername == "" || *perfPassword == "" {
		fmt.Fprintln(os.Stderr, "[FATAL] 登录压测场景必须显式传入 -username 与 -password（或 PERF_USERNAME / PERF_PASSWORD 环境变量）")
		fmt.Fprintln(os.Stderr, "[FATAL] 禁止硬编码默认值（如 admin123）—— 压测账号请使用专用测试账号，与生产超管隔离")
		os.Exit(1)
	}
	return perflib.Config{
		Name:        "login",
		URL:         *baseURL + "/api/auth/login",
		Method:      "POST",
		Concurrency: 20,
		Total:       500,
		Body: map[string]string{
			"username": *perfUsername,
			"password": *perfPassword,
		},
	}
}

// customerListScene 客户列表压测
func customerListScene() perflib.Config {
	return perflib.Config{
		Name:        "customer-list",
		URL:         *baseURL + "/api/customers?page=1&page_size=20",
		Method:      "GET",
		Headers:     map[string]string{"Authorization": "Bearer test-token"},
		Concurrency: 50,
		Total:       1000,
	}
}

// messageListScene 消息列表压测
func messageListScene() perflib.Config {
	return perflib.Config{
		Name:        "message-list",
		URL:         *baseURL + "/api/messages?page=1&page_size=50",
		Method:      "GET",
		Headers:     map[string]string{"Authorization": "Bearer test-token"},
		Concurrency: 50,
		Total:       1000,
	}
}

// knowledgeQueryScene 知识库查询压测
func knowledgeQueryScene() perflib.Config {
	return perflib.Config{
		Name:        "knowledge-query",
		URL:         *baseURL + "/api/knowledge/search?keyword=营销",
		Method:      "GET",
		Headers:     map[string]string{"Authorization": "Bearer test-token"},
		Concurrency: 30,
		Total:       500,
	}
}

// cdpEventScene CDP 事件追踪压测
func cdpEventScene() perflib.Config {
	return perflib.Config{
		Name:        "cdp-event",
		URL:         *baseURL + "/api/events/pageview",
		Method:      "POST",
		Concurrency: 100,
		Total:       2000,
		Body: map[string]any{
			"customer_id": "test-customer",
			"url":         "/products/1",
			"title":       "Test Page",
		},
	}
}

// suppress unused import warnings
var _ = os.Exit

