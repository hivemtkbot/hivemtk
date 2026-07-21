// Package service 的测试统一入口（TestMain）
//
// 职责：
//  1. 在所有 service 包测试运行前设置环境变量，避免任何子测试意外发起真实网络出站
//  2. IS_TEST_MODE=1   —— 与 cmd/api/main.go 行为一致（test 模式）
//  3. WECOM_DISABLE_OUTBOUND=1 —— 强制禁用企微真实出站
//  4. HTTP_BASE_URL_TEST=...   —— 允许单独测试覆盖（不常用）
//
// 任何 service 子包的新增测试都会自动应用此设置，不需要每个 _test.go 重复声明。
package service

import (
	"os"
	"testing"
)

// TestMain service 包测试统一入口
func TestMain(m *testing.M) {
	// 强制设置 IS_TEST_MODE，使 wecom_integration / dispatcher 等模块走"无真实出站"分支
	if os.Getenv("IS_TEST_MODE") == "" {
		_ = os.Setenv("IS_TEST_MODE", "1")
	}
	// 强制禁用企微真实出站（除非测试明确要求开启）
	if os.Getenv("WECOM_DISABLE_OUTBOUND") == "" {
		_ = os.Setenv("WECOM_DISABLE_OUTBOUND", "1")
	}
	os.Exit(m.Run())
}
