// Package main 性能压测入口
// 独立部署版本：单租户，user-server 默认监听 :8080
//
// 使用方法：
//
//	go run ./cmd/perf                          # 跑全部场景
//	go run ./cmd/perf -scene=login             # 跑指定场景
//	go run ./cmd/perf -base=http://localhost:8080
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"marketing/tests/perf/perflib"
)

var (
	baseURL = flag.String("base", "http://localhost:8080", "user-server base URL")
	scene   = flag.String("scene", "all", "测试场景: all|login|customer|message|knowledge|cdp")
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
		// 防止压垮数据库
		time.Sleep(500 * time.Millisecond)
	}
}

// loginScene 登录接口压测
func loginScene() perflib.Config {
	return perflib.Config{
		Name:        "login",
		URL:         *baseURL + "/api/auth/login",
		Method:      "POST",
		Concurrency: 20,
		Total:       500,
		Body: map[string]string{
			"username": "admin",
			"password": "admin123",
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
