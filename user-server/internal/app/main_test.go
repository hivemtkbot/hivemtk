package app

import (
	"os"
	"testing"
)

// TestMain 测试包级环境预设：
// REACH_DISABLE_QUIET_HOURS=true 禁用触达 quiet hours 守卫（22:00-8:00 CST），
// 否则夜间跑 E2E 触达测试时所有发送被延迟入队导致断言失败（时间依赖 flaky）。
func TestMain(m *testing.M) {
	os.Setenv("REACH_DISABLE_QUIET_HOURS", "true")
	os.Exit(m.Run())
}
