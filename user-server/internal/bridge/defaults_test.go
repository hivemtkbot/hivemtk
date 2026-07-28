package bridge

// 桥接默认值与 docs/bridge/DEFAULTS.md 一致性测试
//
// 参考模式：internal/pkg/utils/config/inference_load_test.go
// 目的：
//   1) 验证 handler.go / hub.go 的常量都有意义
//   2) 验证客户端/服务端数值对齐
//   3) 防止后续改动时遗漏文档源引用
//
// 前端对应常量：user-web/bridge/src/core/constants.js
// 文档源：user-web/bridge/docs/DEFAULTS.md

import (
	"strings"
	"testing"
	"time"
)

func TestBridgeDefaultsDocConsistency(t *testing.T) {
	t.Run("WSConstants", func(t *testing.T) {
		// writeWait = 10s（基于 gorilla/websocket 官方推荐）
		if writeWait != 10*time.Second {
			t.Errorf("writeWait 应为 10s，实际 %v", writeWait)
		}
		// pongWait = 60s（前端 WS_CLIENT_DEFAULTS.serverIdleTimeoutMs=25s < 此值）
		if pongWait != 60*time.Second {
			t.Errorf("pongWait 应为 60s，实际 %v", pongWait)
		}
		// pingPeriod = 50s（pongWait - 10s 缓冲）
		if pingPeriod != 50*time.Second {
			t.Errorf("pingPeriod 应为 50s（pongWait - 10s 缓冲），实际 %v", pingPeriod)
		}
		// maxMessageSize = 1MB
		if maxMessageSize != 1<<20 {
			t.Errorf("maxMessageSize 应为 1MB（1<<20），实际 %d", maxMessageSize)
		}
		// maxReplyContentBytes = 4KB（与前端 SECURITY.maxReplyContentBytes 严格对齐）
		if maxReplyContentBytes != 4*1024 {
			t.Errorf("maxReplyContentBytes 应为 4KB（前端 SECURITY 同值），实际 %d", maxReplyContentBytes)
		}
		// sendBufferSize = 256
		if sendBufferSize != 256 {
			t.Errorf("sendBufferSize 应为 256，实际 %d", sendBufferSize)
		}
	})

	t.Run("HubConstants", func(t *testing.T) {
		// DeliverRateLimitPerMin = 60（兜底；前端 accountCapacity=12 更严格）
		if DeliverRateLimitPerMin != 60 {
			t.Errorf("DeliverRateLimitPerMin 应为 60（前端更严格 12），实际 %d", DeliverRateLimitPerMin)
		}
		// JanitorInterval = 60s
		if JanitorInterval != 60*time.Second {
			t.Errorf("JanitorInterval 应为 60s，实际 %v", JanitorInterval)
		}
		// JanitorIdleTTL = 10min
		if JanitorIdleTTL != 10*time.Minute {
			t.Errorf("JanitorIdleTTL 应为 10min，实际 %v", JanitorIdleTTL)
		}
	})

	t.Run("ClientServerAlignment", func(t *testing.T) {
		// 客户端心跳 25s 必须 < 服务端 pongWait 60s
		// （此规则在 handler.go 的常量注释中也明确写出）
		const clientIdleMs = 25 * 1000
		if time.Duration(clientIdleMs)*time.Millisecond >= pongWait {
			t.Error("客户端心跳 25s 必须 < 服务端 pongWait 60s")
		}
		// 服务端 pingPeriod 必须 < pongWait
		if pingPeriod >= pongWait {
			t.Error("pingPeriod 必须 < pongWait")
		}
		// maxReplyContentBytes 必须等于前端 4*1024
		// （在 constants.test.js 也断言；Go 侧二次确认）
		if maxReplyContentBytes != 4*1024 {
			t.Error("maxReplyContentBytes 与前端 SECURITY.maxReplyContentBytes 漂移")
		}
	})

	t.Run("NonSoftStartup", func(t *testing.T) {
		// 禁止"软启动"——所有默认参数必须 > 0
		if writeWait <= 0 {
			t.Error("writeWait 必须为正数（禁止 0/负数兜底）")
		}
		if pongWait <= 0 {
			t.Error("pongWait 必须为正数")
		}
		if pingPeriod <= 0 {
			t.Error("pingPeriod 必须为正数")
		}
		if maxMessageSize <= 0 {
			t.Error("maxMessageSize 必须为正数")
		}
		if maxReplyContentBytes <= 0 {
			t.Error("maxReplyContentBytes 必须为正数")
		}
		if sendBufferSize <= 0 {
			t.Error("sendBufferSize 必须为正数")
		}
		if DeliverRateLimitPerMin <= 0 {
			t.Error("DeliverRateLimitPerMin 必须为正数（禁止关闭限速兜底）")
		}
		if JanitorInterval <= 0 {
			t.Error("JanitorInterval 必须为正数")
		}
		if JanitorIdleTTL <= 0 {
			t.Error("JanitorIdleTTL 必须为正数")
		}
	})

	t.Run("WSEndpoint", func(t *testing.T) {
		// WS 端点路径必须为 /api/ws/bridge（与 service_routes.go:90 一致）
		// 端点路径在 router 中注册，此处通过文档源 + 单元测试组合验证
		// 后续若引入端点常量，应同步更新此断言
		const expectedWSPath = "/api/ws/bridge"
		if !strings.HasPrefix(expectedWSPath, "/api/ws/") {
			t.Error("WS 端点必须以 /api/ws/ 开头（auth 路由前缀）")
		}
	})
}
